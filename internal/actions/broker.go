package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)


const BROKER_KEY_VOL_NAME = "keyvol"


func (c *Container) EditBrokerEnvBytes(envContent []byte, managed map[string]string) ([]byte, error) {
	envFileLines := strings.Split(string(envContent), "\n")
	finalKeys := map[string]string{}
	for k, v := range managed {
		finalKeys[k] = v
	}
	for k, v := range c.gatherBrokerEnvOverridesFromBitwarden() {
		if _, isManaged := managed[k]; isManaged {
			fmt.Printf("%s Bitwarden override for %q ignored (managed by broker-deploy pipeline)\n", warning, k)
			continue
		}
		finalKeys[k] = v
	}
	prependEnvs := []string{}
	for key, value := range finalKeys {
		if value == "" {
			continue
		}
		envFound := false
		envVal := key + "=" + value
		fmt.Printf("Setting %s=%s\n", key, redactSecret(key, value))
		for lineIndex, line := range envFileLines {
			trimmed := strings.TrimLeft(line, " \t")
			if strings.HasPrefix(trimmed, key+"=") || strings.HasPrefix(trimmed, key+" =") {
				envFound = true
				envFileLines[lineIndex] = envVal
				break
			}
		}
		if !envFound {
			prependEnvs = append(prependEnvs, envVal)
		}
	}
	if len(prependEnvs) > 0 {
		envFileLines = append(prependEnvs, envFileLines...)
	}
	return []byte(strings.Join(envFileLines, "\n")), nil
}

func (c *Container) gatherBrokerEnvOverridesFromBitwarden() map[string]string {
	out := map[string]string{}
	if c.BitwardenFields == nil || c.SelectedEnv == "" {
		return out
	}
	envExample, err := configs.EmbededConfigs.ReadFile("embedded/broker-server/env.example")
	if err != nil {
		return out
	}
	suffix := "_" + strings.ToUpper(c.SelectedEnv)
	for key := range parseEnvBytes(envExample) {
		if v, ok := c.BitwardenFields[key+suffix]; ok && v != "" {
			out[key] = v
		}
	}
	return out
}

func (c *Container) LoadBrokerDeployConfigs() (rpc, chainConfig, dockerCompose []byte, err error) {
	rpc, err = c.loadInfraRepoFile("broker-server/rpc.json", "embedded/broker-server/rpc.json")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading broker-server/rpc.json: %w", err)
	}
	chainConfig, err = c.loadInfraRepoFile("broker-server/chainConfig.json", "embedded/broker-server/chainConfig.json")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading broker-server/chainConfig.json: %w", err)
	}
	dockerCompose, err = c.loadInfraRepoFile("broker-server/docker-compose.yml", "embedded/broker-server/docker-compose.yml")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading broker-server/docker-compose.yml: %w", err)
	}
	return rpc, chainConfig, dockerCompose, nil
}

func (c *Container) CopyBrokerDeployConfigs() error {
	rpc, chainCfg, compose, err := c.LoadBrokerDeployConfigs()
	if err != nil {
		return err
	}
	base := c.envWorkDir()
	for _, w := range []struct {
		path    string
		content []byte
	}{
		{filepath.Join(base, "broker-server/rpc.json"), rpc},
		{filepath.Join(base, "broker-server/chainConfig.json"), chainCfg},
		{filepath.Join(base, "broker-server/docker-compose.yml"), compose},
	} {
		if err := os.MkdirAll(filepath.Dir(w.path), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(w.path, w.content, 0644); err != nil {
			return err
		}
	}
	fmt.Println(styles.ItalicText.Render("Broker configs written to " + filepath.Join(base, "broker-server")))
	return nil
}

// BrokerDeploy collects information related to broker-server
// deploymend, copies the configurations files to remote broker host and deploys
// the docker-compose d8x-broker-server setup.
func (c *Container) BrokerDeploy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting broker server deployment configuration...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureProvisionedEnvironment(cfg); err != nil {
		return err
	}
	if err := c.RequireProvisionedHosts("broker-deploy", "broker"); err != nil {
		return err
	}

	if err := c.Input.CollectBrokerDeployInput(ctx); err != nil {
		return fmt.Errorf("collecting broker deploy input: %w", err)
	}

	// Refresh the cfg after input was collected
	cfg, err = c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	// Set broker deployed to true so distribute RPCs works
	cfg.BrokerDeployed = true

	bsd := brokerServerDeployment{}

	// Check for broker ip address
	brokerIpAddr, err := c.HostsCfg.GetBrokerPublicIp()
	if err != nil {
		fmt.Println(
			styles.ErrorText.Render("Broker server ip address was not found. Did you provision broker server?"),
		)
		return err
	}
	bsd.brokerServerIpAddr = brokerIpAddr

	rpcContent, chainConfigContent, composeContent, err := c.LoadBrokerDeployConfigs()
	if err != nil {
		return err
	}
	chainConfigContent, rpcContent, err = c.reviewBrokerConfigs(chainConfigContent, rpcContent)
	if err != nil {
		return err
	}

	fieldName := "BROKER_REDIS_PW_" + strings.ToUpper(c.SelectedEnv)
	var redisPw string
	if c.BitwardenFields != nil {
		if existing, ok := c.BitwardenFields[fieldName]; ok && existing != "" {
			redisPw = existing
			fmt.Printf("  %s Reusing %s from Bitwarden for broker redis password\n", styles.SuccessText.Render("✓"), fieldName)
		}
	}
	if redisPw == "" {
		var err error
		redisPw, err = generatePassword(16)
		if err != nil {
			return fmt.Errorf("generating redis password: %w", err)
		}
		fmt.Printf("  Broker Redis password (newly generated): %s\n", redisPw)
		if os.Getenv("BW_SESSION") != "" && c.SelectedEnv != "" {
			if err := saveAndReport(fieldName, redisPw); err != nil {
				keep, perr := c.TUI.NewPrompt(fmt.Sprintf("Bitwarden save for %s failed. Continue broker deploy with an unsaved password?", fieldName), false)
				if perr != nil {
					return perr
				}
				if !keep {
					return fmt.Errorf("aborted: broker redis password was not persisted to Bitwarden")
				}
			}
		}
	}

	pk := c.Input.brokerDeployInput.privateKey
	bsd.brokerFeeTBPS = c.Input.brokerDeployInput.feeTBPS

	brokerPrivateIp, err := c.HostsCfg.GetBrokerPrivateIp()
	if err != nil || brokerPrivateIp == "" {
		return fmt.Errorf("broker_private_ip not found in hosts.cfg")
	}

	envSuffix := strings.ToUpper(c.SelectedEnv)
	privyAppId := ""
	rateLimit := ""
	enforceMode := ""
	if c.BitwardenFields != nil {
		privyAppId = c.BitwardenFields["PRIVY_APP_ID_"+envSuffix]
		rateLimit = c.BitwardenFields["RATE_LIMIT_"+envSuffix]
		enforceMode = c.BitwardenFields["ENFORCE_MODE_"+envSuffix]
	}
	if privyAppId == "" {
		fmt.Printf("  %s PRIVY_APP_ID_%s not in Bitwarden; rpc-proxy will start with an empty value\n", notok, envSuffix)
	}
	if rateLimit == "" {
		rateLimit = "200"
		fmt.Printf("  %s RATE_LIMIT_%s not in Bitwarden; defaulting to %s and saving\n", notok, envSuffix, rateLimit)
		if err := saveAndReport("RATE_LIMIT_"+envSuffix, rateLimit); err != nil {
			return err
		}
	}
	if enforceMode == "" {
		enforceMode = "1"
		fmt.Printf("  %s ENFORCE_MODE_%s not in Bitwarden; defaulting to %s and saving\n", notok, envSuffix, enforceMode)
		if err := saveAndReport("ENFORCE_MODE_"+envSuffix, enforceMode); err != nil {
			return err
		}
	}

	envTemplate, err := configs.EmbededConfigs.ReadFile("embedded/broker-server/env.example")
	if err != nil {
		return fmt.Errorf("loading embedded broker env template: %w", err)
	}
	if infraEnv, ierr := c.loadOptionalInfraRepoFile("broker-server/.env"); ierr == nil && infraEnv != nil {
		envTemplate = infraEnv
	}
	managedBrokerEnv := map[string]string{
		"REDIS_PW":          redisPw,
		"BROKER_FEE_TBPS":   bsd.brokerFeeTBPS,
		"CHAIN_ID":          strconv.Itoa(int(cfg.ChainId)),
		"BROKER_PRIVATE_IP": brokerPrivateIp,
		"PRIVY_APP_ID":      privyAppId,
		"RATE_LIMIT":        rateLimit,
		"ENFORCE_MODE":      enforceMode,
	}
	brokerEnvBytes, err := c.EditBrokerEnvBytes(envTemplate, managedBrokerEnv)
	if err != nil {
		return fmt.Errorf("building broker .env: %w", err)
	}

	fmt.Println(styles.ItalicText.Render("Copying files to broker-server..."))
	sshClient, err := c.CreateSSHConn(
		bsd.brokerServerIpAddr,
		c.DefaultClusterUserName,
		c.SshKeyPath,
	)
	if err != nil {
		return fmt.Errorf("establishing ssh connection: %w", err)
	}
	defer sshClient.Close()
	if err := sshClient.CopyFilesOverSftp(
		conn.SftpCopySrcDest{Content: chainConfigContent, Dst: "./broker/chainConfig.json"},
		conn.SftpCopySrcDest{Content: rpcContent, Dst: "./broker/rpc.json"},
		conn.SftpCopySrcDest{Content: composeContent, Dst: "./broker/docker-compose.yml"},
		conn.SftpCopySrcDest{Content: brokerEnvBytes, Dst: "./broker/.env"},
	); err != nil {
		return err
	}

	fmt.Println(styles.ItalicText.Render("Preparing Docker volumes..."))
	out, err := c.brokerServerKeyVolSetup(sshClient, pk)
	if err != nil {
		fmt.Printf("%s\n\n%s", out, styles.ErrorText.Render("Something went wrong during broker-server volume deployment ^^^"))
		return err
	}

	fmt.Println(styles.ItalicText.Render("Starting docker compose on broker-server..."))
	out, err = sshClient.ExecCommand("cd ./broker && docker compose up -d")
	if err != nil {
		fmt.Printf("%s\n\n%s", out, styles.ErrorText.Render("Something went wrong during broker-server deployment ^^^"))
		return err
	}

	// Store broker server setup details except pk
	cfg.BrokerServerConfig = configs.D8XBrokerServerConfig{
		FeeTBPS:       bsd.brokerFeeTBPS,
		RedisPassword: redisPw,
	}
	cfg.BrokerDeployed = true
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return err
	}
	if err := c.PublishRemoteConfig(cfg); err != nil {
		fmt.Printf("  %s failed to sync remote config: %s\n", notok, err)
	}

	fmt.Println(styles.SuccessText.Render("Broker server deployment done!"))

	return nil
}

func (c *Container) BrokerServerNginxCertbotSetup(ctx *cli.Context) error {
	styles.PrintCommandTitle("Performing nginx and certbot setup for broker server...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	env, err := c.EnsureProvisionedEnvironment(cfg)
	if err != nil {
		return err
	}
	if err := c.RequireProvisionedHosts("broker-nginx", "broker"); err != nil {
		return err
	}

	if err := c.RequireBitwardenField("GITHUB_TOKEN"); err != nil {
		return err
	}
	token := os.Getenv("GITHUB_TOKEN")

	password, err := c.ResolvePassword(ctx)
	if err != nil {
		return err
	}

	brokerIpAddr, err := c.HostsCfg.GetBrokerPublicIp()
	if err != nil {
		return fmt.Errorf("broker server ip not found in hosts.cfg: %w", err)
	}

	brokerPrivateIp, err := c.HostsCfg.GetBrokerPrivateIp()
	if err != nil || brokerPrivateIp == "" {
		return fmt.Errorf("broker_private_ip not found in hosts.cfg")
	}

	sshConn, err := c.CreateSSHConn(brokerIpAddr, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return fmt.Errorf("SSH connection to broker: %w", err)
	}
	defer sshConn.Close()

	// Fetch broker nginx config from GitHub
	fmt.Println(styles.ItalicText.Render("Fetching broker nginx config from GitHub..."))
	brokerNginx, err := ghReadFile(token, env+"/broker-nginx.conf")
	if err != nil {
		return fmt.Errorf("reading broker-nginx.conf: %w", err)
	}

	if err := c.RequireBitwardenField("NGINX_API_KEY"); err != nil {
		return err
	}
	apiKey := os.Getenv("NGINX_API_KEY")
	brokerNginxContent := strings.ReplaceAll(brokerNginx.Content, "BROKER_PRIVATE_IP_HERE", brokerPrivateIp)
	brokerNginxContent = strings.ReplaceAll(brokerNginxContent, "API_KEY_HERE", apiKey)

	// Extract server_name for DNS instructions
	allNames := extractAllServerNames(brokerNginxContent)
	brokerServerName := ""
	if len(allNames) > 0 {
		brokerServerName = allNames[0]
	}

	fmt.Println(styles.AlertImportant.Render("Please ensure this DNS record exists:"))
	fmt.Printf("  Hostname: %s  Type: A  IP: %s\n", brokerServerName, brokerIpAddr)
	c.TUI.NewConfirmation("Press enter when done...")

	// Install certbot
	fmt.Println("Installing certbot...")
	sshExecSudo(sshConn, password, "apt-get remove -y certbot 2>/dev/null; true")
	sshExecSudo(sshConn, password, "snap install --classic certbot 2>/dev/null; true")
	sshExecSudo(sshConn, password, "ln -sf /snap/bin/certbot /usr/bin/certbot")

	// Deploy nginx config
	fmt.Println("Deploying broker nginx config...")
	sshExecSudo(sshConn, password, "rm -f /etc/nginx/sites-enabled/default")
	if err := sshWriteFileSudo(sshConn, password, "/etc/nginx/sites-enabled/broker", brokerNginxContent); err != nil {
		return err
	}
	fmt.Println("  /etc/nginx/sites-enabled/broker")

	if out, err := sshExecSudo(sshConn, password, "nginx -t 2>&1"); err != nil {
		return fmt.Errorf("nginx config test failed:\n%s", string(out))
	}
	if out, err := sshExecSudo(sshConn, password, "systemctl reload nginx"); err != nil {
		return fmt.Errorf("nginx reload failed:\n%s", string(out))
	}
	fmt.Println(styles.SuccessText.Render("Broker nginx deployed and reloaded."))

	cfg.Services[configs.D8XServiceBrokerServer] = configs.D8XService{
		Name:     configs.D8XServiceBrokerServer,
		HostName: brokerServerName,
	}
	cfg.BrokerNginxDeployed = true

	// Certbot
	setupCertbot, err := c.TUI.NewPrompt("Setup SSL certificate with certbot?", true)
	if err != nil {
		return err
	}
	if setupCertbot {
		emailForCertbot := cfg.CertbotEmail
		if emailForCertbot == "" {
			fmt.Println("Enter email for certbot:")
			emailForCertbot, err = c.TUI.NewInput(components.TextInputOptPlaceholder("admin@example.com"))
			if err != nil {
				return err
			}
			cfg.CertbotEmail = emailForCertbot
		}

		fmt.Printf("  Issuing cert for %s...\n", brokerServerName)
		certCmd := fmt.Sprintf("certbot --nginx -d %s --non-interactive --agree-tos -m %s 2>&1", shQuote(brokerServerName), shQuote(emailForCertbot))
		out, certErr := sshExecSudo(sshConn, password, certCmd)
		certIssued := certErr == nil
		if !certIssued {
			fmt.Printf("  %s certbot failed: %s\n", notok, strings.TrimSpace(string(out)))
		} else {
			fmt.Printf("  %s %s\n", ok, brokerServerName)
			sshExecSudo(sshConn, password, "systemctl enable snap.certbot.renew.timer && systemctl start snap.certbot.renew.timer")
			if val, ok := cfg.Services[configs.D8XServiceBrokerServer]; ok {
				val.UsesHTTPS = true
				cfg.Services[configs.D8XServiceBrokerServer] = val
			}
			cfg.BrokerCertbotDeployed = true
			fmt.Println(styles.SuccessText.Render("Broker SSL setup done!"))
		}
		if !certIssued {
			fmt.Println(styles.AlertImportant.Render(fmt.Sprintf("certbot did not issue a cert for %s. Re-run \"d8x setup broker-nginx\" once DNS/email is fixed.", brokerServerName)))
		}
	}

	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return fmt.Errorf("could not update config: %w", err)
	}
	if err := c.PublishRemoteConfig(cfg); err != nil {
		fmt.Printf("  %s failed to sync remote config: %s\n", notok, err)
	}

	return nil
}

type brokerServerDeployment struct {
	brokerFeeTBPS string

	brokerServerIpAddr string
}

// brokerServerKeyVolSetup creates a ./broker/keyfile.txt file with private key
// on server and sets up a docker volume with the keyfile.txt file. This
// BROKER_KEY_VOL_NAME is later attached to broker service and encrypted on
// startup.
func (c *Container) brokerServerKeyVolSetup(sshClient conn.SSHConnection, pk string) ([]byte, error) {
	// Prepend 0x prefix for pk
	pk = "0x" + strings.TrimPrefix(pk, "0x")

	if err := sshClient.CopyFilesOverSftp(
		conn.SftpCopySrcDest{Content: []byte(pk), Dst: "./broker/keyfile.txt"},
	); err != nil {
		return nil, fmt.Errorf("staging keyfile: %w", err)
	}
	cmd := fmt.Sprintf("cd ./broker && docker volume create %s", BROKER_KEY_VOL_NAME)
	cmd = fmt.Sprintf("%s && docker run --rm -v $PWD:/source -v %s:/dest -w /source alpine cp ./keyfile.txt /dest", cmd, BROKER_KEY_VOL_NAME)
	cmd = fmt.Sprintf("%s && rm ./keyfile.txt", cmd)

	return sshClient.ExecCommand(cmd)
}

// Convert given percentage string p to TBPS string
func convertPercentToTBPS(p string) (string, error) {
	// Allow to enter 0
	if p == "0" {
		return "0", nil
	}

	// For floating point - allow only point as separator
	if strings.Contains(p, ",") {
		return "", fmt.Errorf("invalid percent value, use dot '.' instead of comma ',': %s", p)
	}

	// Max 3 digits in the decimal fraction
	if strings.Contains(p, ".") {
		wholeDec := strings.Split(p, ".")

		if len(wholeDec) != 2 {
			return "", fmt.Errorf("invalid percent value: %s", p)
		}

		if len(wholeDec[1]) > 3 {
			return "", fmt.Errorf("invalid percent value, max 3 digits in the decimal fraction: %s", p)
		}
	}

	// Convert to float and multiply by 1000 to get TBPS
	parsedFloat, err := strconv.ParseFloat(p, 64)
	if err != nil {
		return "", fmt.Errorf("invalid percent value: %w", err)
	}

	tbps := parsedFloat * 1000

	return strconv.FormatFloat(tbps, 'f', 0, 64), nil
}

func (c *Container) reviewBrokerConfigs(chainConfig, rpc []byte) ([]byte, []byte, error) {
	files := []struct {
		repoPath string
		content  *[]byte
	}{
		{"broker-server/chainConfig.json", &chainConfig},
		{"broker-server/rpc.json", &rpc},
	}
	for {
		for _, f := range files {
			fmt.Println()
			fmt.Println(styles.PurpleBgText.Copy().Padding(0, 2).Render(c.SelectedEnv + "/" + f.repoPath))
			fmt.Println(string(*f.content))
		}
		fmt.Println()
		labels := []string{
			"Proceed with these values",
			"Edit broker-server/chainConfig.json",
			"Edit broker-server/rpc.json",
			"Abort",
		}
		selected, err := c.TUI.NewSelection(labels, components.SelectionOptAllowOnlySingleItem(), components.SelectionOptRequireSelection())
		if err != nil {
			return nil, nil, err
		}
		if len(selected) == 0 {
			continue
		}
		switch selected[0] {
		case labels[0]:
			return *files[0].content, *files[1].content, nil
		case labels[3]:
			return nil, nil, fmt.Errorf("aborted: broker-server review")
		case labels[1]:
			if err := c.editInfraRepoFile(files[0].repoPath, files[0].content); err != nil {
				fmt.Println(styles.AlertImportant.Render(err.Error()))
			}
		case labels[2]:
			if err := c.editInfraRepoFile(files[1].repoPath, files[1].content); err != nil {
				fmt.Println(styles.AlertImportant.Render(err.Error()))
			}
		}
	}
}

func (c *Container) editInfraRepoFile(repoRelPath string, content *[]byte) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		for _, candidate := range []string{"nano", "vim", "vi"} {
			if _, err := exec.LookPath(candidate); err == nil {
				editor = candidate
				break
			}
		}
	}
	if editor == "" {
		return fmt.Errorf("no editor available: set $EDITOR (e.g. \"export EDITOR=nano\") and try again")
	}

	tmpPath, err := ensureWorkDir(filepath.Join(c.SelectedEnv, repoRelPath))
	if err != nil {
		return fmt.Errorf("prepare temp file: %w", err)
	}
	if err := os.WriteFile(tmpPath, *content, 0600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	editorErr := cmd.Run()

	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		if editorErr != nil {
			return fmt.Errorf("editor failed and temp file unreadable: %w", editorErr)
		}
		return fmt.Errorf("read edited file: %w", err)
	}
	if editorErr != nil && bytes.Equal(edited, *content) {
		return fmt.Errorf("editor exited with error and no changes were saved: %w", editorErr)
	}
	var probe any
	if err := json.Unmarshal(edited, &probe); err != nil {
		return fmt.Errorf("edited %s is not valid JSON: %w", repoRelPath, err)
	}
	if bytes.Equal(edited, *content) {
		fmt.Println(styles.ItalicText.Render("No changes."))
		return nil
	}

	repoPath := c.SelectedEnv + "/" + repoRelPath
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN missing, cannot push %s", repoPath)
	}
	if err := ghCommitFiles(token, []ghCommitFile{{Path: repoPath, Content: string(edited)}}, fmt.Sprintf("update %s (modified during \"d8x setup broker-deploy\")", repoPath)); err != nil {
		return fmt.Errorf("push %s: %w", repoPath, err)
	}
	fmt.Printf("%s pushed %s to infra repo\n", ok, repoPath)
	*content = edited
	return nil
}

