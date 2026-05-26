package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) TerraformDestroy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Running terraform destroy...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureProvisionedEnvironment(cfg); err != nil {
		return err
	}
	c.ProvisioningTfDir = c.tfDir()
	if err := os.MkdirAll(c.ProvisioningTfDir, 0700); err != nil {
		return fmt.Errorf("preparing terraform work dir: %w", err)
	}
	if cfg.ServerProvider == "" {
		return fmt.Errorf("server_provider missing from %s/config.json in infra repo. Cannot determine which provider to destroy", c.SelectedEnv)
	}
	normalizeConfigStrings(cfg, c.SelectedEnv)

	targets := c.collectDestroyTargets(cfg)

	fmt.Println(styles.AlertImportant.Render(fmt.Sprintf("You are about to destroy environment %q.", c.SelectedEnv)))
	printDestroyTargets(targets)
	fmt.Println(styles.AlertImportant.Render("All the resources listed above will be destroyed. Irreversible."))

	confirm, err := c.TUI.NewPrompt(fmt.Sprintf("Proceed with destroying %q?", c.SelectedEnv), false)
	if err != nil {
		return err
	}
	if !confirm {
		fmt.Println("Not destroying...")
		return nil
	}

	fmt.Printf("Type the environment name (%s) to confirm:\n", c.SelectedEnv)
	typed, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder(c.SelectedEnv),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return err
	}
	if strings.TrimSpace(typed) != c.SelectedEnv {
		fmt.Printf("Typed %q does not match %q. Not destroying.\n", typed, c.SelectedEnv)
		return nil
	}

	labelPrefix := destroyLabelPrefix(cfg)
	if cfg.ServerProvider == configs.D8XServerProviderAWS {
		if err := verifyTerraformState(c.ProvisioningTfDir, labelPrefix, c.SelectedEnv); err != nil {
			fmt.Println(styles.AlertImportant.Render(err.Error()))
			proceed, perr := c.TUI.NewPrompt("Continue anyway? (destroy will be a no-op; resources at the cloud provider may be orphaned)", false)
			if perr != nil {
				return perr
			}
			if !proceed {
				return fmt.Errorf("aborted: terraform state did not match env %q", c.SelectedEnv)
			}
		}
	}

	if err := c.ensureSSHKey(c.SelectedEnv); err != nil {
		return fmt.Errorf("ensuring SSH key for destroy: %w", err)
	}
	authorizedKey, err := getPublicKey(c.SshKeyPath)
	if err != nil {
		return fmt.Errorf("reading SSH public key for terraform: %w", err)
	}
	if strings.TrimSpace(authorizedKey) == "" {
		return fmt.Errorf("SSH public key at %s.pub is empty; terraform requires a non-empty authorized_keys value even for destroy", c.SshKeyPath)
	}

	if err := c.fetchTerraformInputs(cfg); err != nil {
		return err
	}

	tfInit := exec.Command("terraform", "init")
	tfInit.Dir = c.ProvisioningTfDir
	connectCMDToCurrentTerm(tfInit)
	if err := c.RunCmd(tfInit); err != nil {
		return fmt.Errorf("terraform init: %w", err)
	}

	var args []string = []string{
		"destroy", "-auto-approve",
	}

	var env []string = []string{}

	switch cfg.ServerProvider {
	case configs.D8XServerProviderAWS:
		if cfg.AWSConfig == nil {
			return fmt.Errorf("aws config is not defined")
		}
		awsCfg := *cfg.AWSConfig
		awsCfg.AccesKey = readEnvSecret(c.SelectedEnv, "AWS_ACCESS_KEY")
		awsCfg.SecretKey = readEnvSecret(c.SelectedEnv, "AWS_SECRET_KEY")
		if awsCfg.AccesKey == "" || awsCfg.SecretKey == "" {
			return fmt.Errorf("AWS credentials missing: set AWS_ACCESS_KEY_%s and AWS_SECRET_KEY_%s in Bitwarden", strings.ToUpper(c.SelectedEnv), strings.ToUpper(c.SelectedEnv))
		}
		awsConfigurer := &awsConfigurer{D8XAWSConfig: awsCfg, authorizedKey: authorizedKey}
		args = append(args, awsConfigurer.generateVariables()...)
		env = append(env, awsConfigurer.awsEnv()...)

	case configs.D8XServerProviderLinode:
		token := readEnvSecret(c.SelectedEnv, "LINODE_TOKEN")
		if token == "" {
			return fmt.Errorf("LINODE_TOKEN missing: set LINODE_TOKEN_%s in Bitwarden", strings.ToUpper(c.SelectedEnv))
		}
		env = append(env, fmt.Sprintf("LINODE_TOKEN=%s", token))

		username, email, aerr := linodeAccountInfo(token)
		if aerr != nil {
			return fmt.Errorf("verifying Linode account for token LINODE_TOKEN_%s: %w", strings.ToUpper(c.SelectedEnv), aerr)
		}
		fmt.Printf("\n%s Linode token LINODE_TOKEN_%s belongs to account: %s <%s>\n", arrow, strings.ToUpper(c.SelectedEnv), username, email)
		acctOk, perr := c.TUI.NewPrompt("Is this the correct Linode account for this env?", false)
		if perr != nil {
			return perr
		}
		if !acctOk {
			fmt.Println(styles.ItalicText.Render(fmt.Sprintf("Aborted. Fix LINODE_TOKEN_%s in Bitwarden so it points at the right account, then retry.", strings.ToUpper(c.SelectedEnv))))
			return nil
		}

		fmt.Printf("\n%s Discovering Linode resources for label prefix %q...\n", arrow, labelPrefix)
		discovered, derr := discoverLinodeResources(token, labelPrefix)
		if derr != nil {
			return fmt.Errorf("discovering Linode resources for %q: %w", labelPrefix, derr)
		}
		if len(discovered) == 0 {
			fmt.Println(styles.ItalicText.Render(fmt.Sprintf("No Linode resources found matching label prefix %q. Nothing to destroy.", labelPrefix)))
			return nil
		}

		fmt.Printf("\n%s Linode resources discovered for env %q (label prefix %q):\n", arrow, c.SelectedEnv, labelPrefix)
		for _, r := range discovered {
			fmt.Printf("  %s  id=%d  ip=%s  →  %s\n", r.label, r.id, r.ipv4, r.tfAddress)
		}

		hostsIPs := map[string]struct{}{}
		if mip, herr := c.HostsCfg.GetMangerPublicIp(); herr == nil && mip != "" {
			hostsIPs[mip] = struct{}{}
		}
		if wips, herr := c.HostsCfg.GetWorkerIps(); herr == nil {
			for _, w := range wips {
				hostsIPs[w] = struct{}{}
			}
		}
		if bip, herr := c.HostsCfg.GetBrokerPublicIp(); herr == nil && bip != "" {
			hostsIPs[bip] = struct{}{}
		}
		discoveredIPs := map[string]struct{}{}
		var unknownDiscovered []discoveredLinodeResource
		for _, r := range discovered {
			if r.ipv4 == "" {
				continue
			}
			discoveredIPs[r.ipv4] = struct{}{}
			if _, ok := hostsIPs[r.ipv4]; !ok {
				unknownDiscovered = append(unknownDiscovered, r)
			}
		}
		var missingFromDiscovery []string
		for ip := range hostsIPs {
			if _, ok := discoveredIPs[ip]; !ok {
				missingFromDiscovery = append(missingFromDiscovery, ip)
			}
		}
		if len(hostsIPs) == 0 {
			fmt.Println(styles.AlertImportant.Render(fmt.Sprintf("hosts.cfg for %q parses to zero IPs; cannot cross-check discovered Linodes against the expected set.", c.SelectedEnv)))
			fmt.Println("Type DESTROY-WITHOUT-HOSTS-CHECK (uppercase, exact) to proceed without that check:")
			typedSkip, serr := c.TUI.NewInput(
				components.TextInputOptPlaceholder("DESTROY-WITHOUT-HOSTS-CHECK"),
				components.TextInputOptDenyEmpty(),
			)
			if serr != nil {
				return serr
			}
			if strings.TrimSpace(typedSkip) != "DESTROY-WITHOUT-HOSTS-CHECK" {
				fmt.Println(styles.ItalicText.Render("Aborted; cross-check could not run."))
				return nil
			}
		} else if len(unknownDiscovered) > 0 || len(missingFromDiscovery) > 0 {
			fmt.Println(styles.AlertImportant.Render(fmt.Sprintf("Mismatch between discovered Linodes and %s/hosts.cfg:", c.SelectedEnv)))
			if len(unknownDiscovered) > 0 {
				fmt.Println("  Discovered but NOT in hosts.cfg (would be destroyed; was NOT provisioned for this env):")
				for _, r := range unknownDiscovered {
					fmt.Printf("    - %s  id=%d  ip=%s\n", r.label, r.id, r.ipv4)
				}
			}
			if len(missingFromDiscovery) > 0 {
				fmt.Println("  Listed in hosts.cfg but NOT discovered (already gone, or scaled down outside the CLI):")
				for _, ip := range missingFromDiscovery {
					fmt.Printf("    - %s\n", ip)
				}
			}
			expected := strconv.Itoa(len(unknownDiscovered))
			fmt.Printf("Type %s (the count of unknown Linodes) to acknowledge that many will be destroyed:\n", expected)
			typedAck, aerr := c.TUI.NewInput(
				components.TextInputOptPlaceholder(expected),
				components.TextInputOptDenyEmpty(),
			)
			if aerr != nil {
				return aerr
			}
			if strings.TrimSpace(typedAck) != expected {
				fmt.Println(styles.ItalicText.Render("Aborted; mismatch not acknowledged."))
				return nil
			}
		} else {
			fmt.Printf("  %s every discovered IP matches %s/hosts.cfg\n", ok, c.SelectedEnv)
		}

		fmt.Println()
		fmt.Println(styles.ItalicText.Render("Select EXACTLY which Linodes to destroy. Rows whose IP is in hosts.cfg are pre-checked. Toggle anything you do NOT want destroyed."))
		optByLabel := map[string]discoveredLinodeResource{}
		var optList []string
		var selOpts []components.SelectionOpts
		for _, r := range discovered {
			opt := fmt.Sprintf("%s  id=%d  ip=%s", r.label, r.id, r.ipv4)
			optList = append(optList, opt)
			optByLabel[opt] = r
			if _, ok := hostsIPs[r.ipv4]; ok {
				selOpts = append(selOpts, components.SelectionOptSelectedValue(opt))
			}
		}
		selected, serr := c.TUI.NewSelection(optList, selOpts...)
		if serr != nil {
			return serr
		}
		if len(selected) == 0 {
			fmt.Println(styles.ItalicText.Render("No resources selected. Aborted."))
			return nil
		}

		var picked []discoveredLinodeResource
		hasManager, hasBroker, numWorkers := 0, 0, 0
		for _, opt := range selected {
			r := optByLabel[opt]
			picked = append(picked, r)
			switch {
			case strings.HasPrefix(r.tfAddress, "linode_instance.manager"):
				hasManager = 1
			case strings.HasPrefix(r.tfAddress, "linode_instance.broker_server"):
				hasBroker = 1
			case strings.HasPrefix(r.tfAddress, "linode_instance.nodes"):
				numWorkers++
			}
		}
		workerIdx := 0
		for i := range picked {
			if strings.HasPrefix(picked[i].tfAddress, "linode_instance.nodes") {
				picked[i].tfAddress = fmt.Sprintf("linode_instance.nodes[%d]", workerIdx)
				workerIdx++
			}
		}

		fmt.Printf("\n%s Final set to destroy in env %q:\n", arrow, c.SelectedEnv)
		for _, r := range picked {
			fmt.Printf("  %s  id=%d  ip=%s  →  %s\n", r.label, r.id, r.ipv4, r.tfAddress)
		}
		fmt.Printf("  total: %d instance(s)  (%d manager, %d worker, %d broker)\n", len(picked), hasManager, numWorkers, hasBroker)

		proceed, perr := c.TUI.NewPrompt(fmt.Sprintf("Proceed to import the %d resource(s) above into state?", len(picked)), false)
		if perr != nil {
			return perr
		}
		if !proceed {
			fmt.Println(styles.ItalicText.Render("Aborted; no resources imported or destroyed."))
			return nil
		}

		fmt.Printf("Type the environment name (%s) to confirm import:\n", c.SelectedEnv)
		typedImport, terr := c.TUI.NewInput(
			components.TextInputOptPlaceholder(c.SelectedEnv),
			components.TextInputOptDenyEmpty(),
		)
		if terr != nil {
			return terr
		}
		if strings.TrimSpace(typedImport) != c.SelectedEnv {
			fmt.Printf("Typed %q does not match %q. Not destroying.\n", typedImport, c.SelectedEnv)
			return nil
		}

		statePath := filepath.Join(c.ProvisioningTfDir, "terraform.tfstate")
		_ = os.Remove(statePath)
		_ = os.Remove(statePath + ".backup")

		importVars := []string{
			"-var", fmt.Sprintf(`authorized_keys=["%s"]`, strings.TrimSpace(authorizedKey)),
			"-var", fmt.Sprintf("num_workers=%d", numWorkers),
			"-var", fmt.Sprintf("create_swarm=%t", hasManager == 1),
			"-var", fmt.Sprintf("create_broker_server=%t", hasBroker == 1),
		}
		for _, r := range picked {
			fmt.Println(styles.ItalicText.Render(fmt.Sprintf("  importing %s (id %d)...", r.tfAddress, r.id)))
			importArgs := append([]string{"import"}, importVars...)
			importArgs = append(importArgs, r.tfAddress, strconv.Itoa(r.id))
			importCmd := exec.Command("terraform", importArgs...)
			importCmd.Dir = c.ProvisioningTfDir
			importCmd.Env = append(os.Environ(), env...)
			connectCMDToCurrentTerm(importCmd)
			if err := c.RunCmd(importCmd); err != nil {
				return fmt.Errorf("importing %s (id %d): %w", r.tfAddress, r.id, err)
			}
		}
		args = append(args, importVars...)

		fmt.Println()
		fmt.Println(styles.AlertImportant.Render(fmt.Sprintf("Imports complete. About to run \"terraform destroy\" on %d resource(s) in env %q.", len(picked), c.SelectedEnv)))
		fmt.Println(styles.AlertImportant.Render("This is the final step. After this, the Linodes are gone."))
		fmt.Println("Type DESTROY (uppercase) to proceed:")
		typedFinal, ferr := c.TUI.NewInput(
			components.TextInputOptPlaceholder("DESTROY"),
			components.TextInputOptDenyEmpty(),
		)
		if ferr != nil {
			return ferr
		}
		if strings.TrimSpace(typedFinal) != "DESTROY" {
			fmt.Printf("Typed %q, expected exactly \"DESTROY\". Not destroying.\n", typedFinal)
			return nil
		}
	}

	cmd := exec.Command("terraform", args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = c.ProvisioningTfDir

	connectCMDToCurrentTerm(cmd)
	if err := c.RunCmd(cmd); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(styles.SuccessText.Render(fmt.Sprintf("Environment %q successfully destroyed:", c.SelectedEnv)))
	printDestroyTargets(targets)
	fmt.Println()

	doCleanup, err := c.TUI.NewPrompt(fmt.Sprintf("Proceed with bookkeeping cleanup (clear deployment flags in %s/config.json, remove hosts.cfg locally and from infra repo)?", c.SelectedEnv), true)
	if err != nil {
		return err
	}
	if !doCleanup {
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf("Cleanup skipped. %s/config.json and %s/hosts.cfg in the infra repo still reflect the pre-destroy state, and the local ./hosts.cfg is intact. Rerun \"d8x tf-destroy\" to clean them up later.", c.SelectedEnv, c.SelectedEnv)))
		return nil
	}

	cfg.ResetDeploymentStatus()
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return err
	}
	if err := c.PublishRemoteConfig(cfg); err != nil {
		fmt.Printf("%s warning: could not publish reset state to infra repo: %s\n", warning, err)
	}
	c.cleanupHostsAfterDestroy()
	return nil
}

func destroyLabelPrefix(cfg *configs.D8XConfig) string {
	switch cfg.ServerProvider {
	case configs.D8XServerProviderAWS:
		if cfg.AWSConfig != nil {
			return cfg.AWSConfig.LabelPrefix
		}
	case configs.D8XServerProviderLinode:
		if cfg.LinodeConfig != nil {
			return cfg.LinodeConfig.LabelPrefix
		}
	}
	return ""
}

func verifyTerraformState(dir, labelPrefix, env string) error {
	statePath := filepath.Join(dir, "terraform.tfstate")
	info, err := os.Stat(statePath)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("no terraform state at %s. Either the env was never provisioned from this directory, or local state was deleted. Re-running provision or migrating state from another machine is required to safely destroy", statePath)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", statePath, err)
	}
	var parsed struct {
		Resources []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("parsing %s: %w", statePath, err)
	}
	if len(parsed.Resources) == 0 {
		return fmt.Errorf("terraform state at %s has no tracked resources. Destroy would be a no-op", statePath)
	}
	if labelPrefix != "" && !bytes.Contains(data, []byte(labelPrefix)) {
		return fmt.Errorf("terraform state at %s does not reference label prefix %q from %s/config.json. The state file may belong to a different environment; refusing to destroy", statePath, labelPrefix, env)
	}
	return nil
}

func (c *Container) fetchTerraformInputs(cfg *configs.D8XConfig) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN missing, cannot fetch terraform configs from infra repo")
	}
	tfSubdir := "terraform/" + string(cfg.ServerProvider)
	fmt.Println(styles.ItalicText.Render("Fetching terraform configs from infra repo (" + tfSubdir + ")..."))
	if err := ghFetchDir(token, tfSubdir, c.ProvisioningTfDir); err != nil {
		return fmt.Errorf("fetching %s from infra repo: %w", tfSubdir, err)
	}
	tfvars, err := ghReadFile(token, c.SelectedEnv+"/terraform.tfvars")
	if err != nil {
		return fmt.Errorf("fetching %s/terraform.tfvars from infra repo: %w", c.SelectedEnv, err)
	}
	tfvarsPath := filepath.Join(c.ProvisioningTfDir, "env.auto.tfvars")
	if err := os.WriteFile(tfvarsPath, []byte(tfvars.Content), 0600); err != nil {
		return fmt.Errorf("writing %s: %w", tfvarsPath, err)
	}
	fmt.Printf("  %s wrote %s\n", ok, tfvarsPath)
	return nil
}

func (c *Container) cleanupHostsAfterDestroy() {
	hostsPath := c.hostsCfgPath()
	if err := os.Remove(hostsPath); err == nil {
		fmt.Printf("%s removed local %s\n", ok, hostsPath)
	} else if !os.IsNotExist(err) {
		fmt.Printf("%s warning: could not remove local %s: %s\n", warning, hostsPath, err)
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" || c.SelectedEnv == "" {
		return
	}
	remotePath := c.SelectedEnv + "/hosts.cfg"
	existing, err := ghReadFile(token, remotePath)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return
		}
		fmt.Printf("%s warning: could not look up %s on infra repo: %s\n", warning, remotePath, err)
		return
	}
	if err := ghDeleteFile(token, remotePath, existing.SHA, "delete "+remotePath); err != nil {
		fmt.Printf("%s warning: could not delete %s on infra repo: %s\n", warning, remotePath, err)
		return
	}
	fmt.Printf("%s removed %s from infra repo\n", ok, remotePath)
}

type destroyTargets struct {
	provider    string
	region      string
	labelPrefix string
	managerIP   string
	workerIPs   []string
	brokerIP    string
	deployed    []string
}

func (c *Container) collectDestroyTargets(cfg *configs.D8XConfig) destroyTargets {
	t := destroyTargets{provider: string(cfg.ServerProvider)}
	switch cfg.ServerProvider {
	case configs.D8XServerProviderAWS:
		if cfg.AWSConfig != nil {
			t.region = cfg.AWSConfig.Region
			t.labelPrefix = cfg.AWSConfig.LabelPrefix
		}
	case configs.D8XServerProviderLinode:
		if cfg.LinodeConfig != nil {
			t.region = cfg.LinodeConfig.Region
			t.labelPrefix = cfg.LinodeConfig.LabelPrefix
		}
	}
	if managerIp, err := c.HostsCfg.GetMangerPublicIp(); err == nil {
		t.managerIP = managerIp
	}
	if workerIps, err := c.HostsCfg.GetWorkerIps(); err == nil {
		t.workerIPs = workerIps
	}
	if brokerIp, err := c.HostsCfg.GetBrokerPublicIp(); err == nil {
		t.brokerIP = brokerIp
	}
	if cfg.SwarmDeployed {
		t.deployed = append(t.deployed, "swarm")
	}
	if cfg.SwarmNginxDeployed {
		t.deployed = append(t.deployed, "swarm-nginx")
	}
	if cfg.BrokerDeployed {
		t.deployed = append(t.deployed, "broker")
	}
	if cfg.BrokerNginxDeployed {
		t.deployed = append(t.deployed, "broker-nginx")
	}
	if cfg.MetricsDeployed {
		t.deployed = append(t.deployed, "metrics")
	}
	return t
}

func printDestroyTargets(t destroyTargets) {
	fmt.Printf("  provider:        %s\n", t.provider)
	if t.region != "" {
		fmt.Printf("  region:          %s\n", t.region)
	}
	if t.labelPrefix != "" {
		fmt.Printf("  label prefix:    %s\n", t.labelPrefix)
	}
	if t.managerIP != "" {
		fmt.Printf("  manager:         %s\n", t.managerIP)
	}
	if len(t.workerIPs) > 0 {
		fmt.Printf("  workers:         (%d) %s\n", len(t.workerIPs), strings.Join(t.workerIPs, ", "))
	}
	if t.brokerIP != "" {
		fmt.Printf("  broker:          %s\n", t.brokerIP)
	}
	if len(t.deployed) > 0 {
		fmt.Printf("  deployed:        %s\n", strings.Join(t.deployed, ", "))
	}
}

type discoveredLinodeResource struct {
	id        int
	label     string
	tfAddress string
	ipv4      string
}

func discoverLinodeResources(token, labelPrefix string) ([]discoveredLinodeResource, error) {
	if labelPrefix == "" {
		return nil, fmt.Errorf("label_prefix is empty in this env's config.json; refusing to enumerate Linode resources")
	}
	req, err := http.NewRequest("GET", "https://api.linode.com/v4/linode/instances?page_size=200", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	filter, _ := json.Marshal(map[string]map[string]string{
		"label": {"+contains": labelPrefix},
	})
	req.Header.Set("X-Filter", string(filter))
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Linode API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Data []struct {
			ID    int      `json:"id"`
			Label string   `json:"label"`
			IPv4  []string `json:"ipv4"`
		} `json:"data"`
		Pages int `json:"pages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Pages > 1 {
		return nil, fmt.Errorf("Linode API returned %d pages for label_prefix %q (only the first 200 results are read); aborting to avoid a partial destroy", payload.Pages, labelPrefix)
	}

	matchManager := regexp.MustCompile("^" + regexp.QuoteMeta(labelPrefix) + "-manager$")
	matchWorker := regexp.MustCompile("^" + regexp.QuoteMeta(labelPrefix) + `-worker-(\d+)$`)
	matchBroker := regexp.MustCompile("^" + regexp.QuoteMeta(labelPrefix) + "-broker-server$")

	var out []discoveredLinodeResource
	for _, inst := range payload.Data {
		ip := firstPublicIPv4(inst.IPv4)
		switch {
		case matchManager.MatchString(inst.Label):
			out = append(out, discoveredLinodeResource{id: inst.ID, label: inst.Label, tfAddress: "linode_instance.manager[0]", ipv4: ip})
		case matchBroker.MatchString(inst.Label):
			out = append(out, discoveredLinodeResource{id: inst.ID, label: inst.Label, tfAddress: "linode_instance.broker_server[0]", ipv4: ip})
		default:
			if m := matchWorker.FindStringSubmatch(inst.Label); m != nil {
				n, perr := strconv.Atoi(m[1])
				if perr != nil || n < 1 {
					continue
				}
				out = append(out, discoveredLinodeResource{id: inst.ID, label: inst.Label, tfAddress: fmt.Sprintf("linode_instance.nodes[%d]", n-1), ipv4: ip})
			}
		}
	}

	seen := map[string][]discoveredLinodeResource{}
	for _, r := range out {
		seen[r.tfAddress] = append(seen[r.tfAddress], r)
	}
	for addr, group := range seen {
		if len(group) > 1 {
			var detail []string
			for _, r := range group {
				detail = append(detail, fmt.Sprintf("%s (id %d, ip %s)", r.label, r.id, r.ipv4))
			}
			return nil, fmt.Errorf("multiple Linodes map to %s: %s. Two instances share the same label. Resolve by hand on Linode before retrying destroy", addr, strings.Join(detail, "; "))
		}
	}
	return out, nil
}

func stripStrayQuotes(s string) string {
	out := strings.TrimSpace(s)
	for strings.HasPrefix(out, `"`) || strings.HasSuffix(out, `"`) {
		out = strings.TrimPrefix(out, `"`)
		out = strings.TrimSuffix(out, `"`)
	}
	return out
}

func normalizeConfigStrings(cfg *configs.D8XConfig, env string) {
	warn := func(field, before, after string) {
		fmt.Printf("%s %s/config.json has corrupted %s (%q); using %q. Fix the infra repo so future runs don't need this workaround.\n", warning, env, field, before, after)
	}
	if cfg.LinodeConfig != nil {
		if v := stripStrayQuotes(cfg.LinodeConfig.LabelPrefix); v != cfg.LinodeConfig.LabelPrefix {
			warn("linode_config.label_prefix", cfg.LinodeConfig.LabelPrefix, v)
			cfg.LinodeConfig.LabelPrefix = v
		}
		if v := stripStrayQuotes(cfg.LinodeConfig.Region); v != cfg.LinodeConfig.Region {
			warn("linode_config.region", cfg.LinodeConfig.Region, v)
			cfg.LinodeConfig.Region = v
		}
		if v := stripStrayQuotes(cfg.LinodeConfig.BrokerServerSize); v != cfg.LinodeConfig.BrokerServerSize {
			warn("linode_config.broker_server_size", cfg.LinodeConfig.BrokerServerSize, v)
			cfg.LinodeConfig.BrokerServerSize = v
		}
	}
	if cfg.AWSConfig != nil {
		if v := stripStrayQuotes(cfg.AWSConfig.LabelPrefix); v != cfg.AWSConfig.LabelPrefix {
			warn("aws_config.label_prefix", cfg.AWSConfig.LabelPrefix, v)
			cfg.AWSConfig.LabelPrefix = v
		}
		if v := stripStrayQuotes(cfg.AWSConfig.Region); v != cfg.AWSConfig.Region {
			warn("aws_config.region", cfg.AWSConfig.Region, v)
			cfg.AWSConfig.Region = v
		}
	}
}

func linodeAccountInfo(token string) (string, string, error) {
	req, err := http.NewRequest("GET", "https://api.linode.com/v4/profile", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("Linode API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	return payload.Username, payload.Email, nil
}

func firstPublicIPv4(ips []string) string {
	for _, ip := range ips {
		if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "172.") {
			continue
		}
		return ip
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}
