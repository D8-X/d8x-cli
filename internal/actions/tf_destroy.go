package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	if _, err := c.EnsureEnvironment(cfg); err != nil {
		return err
	}
	if cfg.ServerProvider == "" {
		return fmt.Errorf("server_provider missing from %s/config.json in infra repo. Cannot determine which provider to destroy", c.SelectedEnv)
	}

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

	if err := c.fetchTerraformInputs(cfg); err != nil {
		return err
	}

	tfInit := exec.Command("terraform", "init")
	tfInit.Dir = c.ProvisioningTfDir
	connectCMDToCurrentTerm(tfInit)
	if err := tfInit.Run(); err != nil {
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
		awsConfigurer := &awsConfigurer{D8XAWSConfig: awsCfg, authorizedKey: ""}
		args = append(args, awsConfigurer.generateVariables()...)

	case configs.D8XServerProviderLinode:
		args = append(args, "-var", `authorized_keys=[""]`)
		token := readEnvSecret(c.SelectedEnv, "LINODE_TOKEN")
		if token == "" {
			return fmt.Errorf("LINODE_TOKEN missing: set LINODE_TOKEN_%s in Bitwarden", strings.ToUpper(c.SelectedEnv))
		}
		env = append(env, fmt.Sprintf("LINODE_TOKEN=%s", token))
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
	if err := os.WriteFile(tfvarsPath, []byte(tfvars.Content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", tfvarsPath, err)
	}
	fmt.Printf("  %s wrote %s\n", ok, tfvarsPath)
	return nil
}

func (c *Container) cleanupHostsAfterDestroy() {
	if err := os.Remove(configs.DEFAULT_HOSTS_FILE); err == nil {
		fmt.Printf("%s removed local %s\n", ok, configs.DEFAULT_HOSTS_FILE)
	} else if !os.IsNotExist(err) {
		fmt.Printf("%s warning: could not remove local %s: %s\n", warning, configs.DEFAULT_HOSTS_FILE, err)
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
	if err := ghDeleteFile(token, remotePath, existing.SHA, "delete "+remotePath+" - d8x tf-destroy"); err != nil {
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
		fmt.Printf("  workers (%d):     %s\n", len(t.workerIPs), strings.Join(t.workerIPs, ", "))
	}
	if t.brokerIP != "" {
		fmt.Printf("  broker:          %s\n", t.brokerIP)
	}
	if len(t.deployed) > 0 {
		fmt.Printf("  deployed:        %s\n", strings.Join(t.deployed, ", "))
	}
}
