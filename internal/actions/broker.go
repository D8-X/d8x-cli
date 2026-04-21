package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)


const BROKER_KEY_VOL_NAME = "keyvol"

var (
	brokerDeployChainConfig   = "./broker-server/chainConfig.json"
	brokerDeployRpcConfig     = "./broker-server/rpc.json"
	brokerDeployDockerCompose = "./broker-server/docker-compose.yml"

	// Optional .env file path. If found, this .env file will be copied to the
	// broker-server deployment.
	brokerEnvFile = "./broker-server/.env"
)

func (c *Container) CopyBrokerDeployConfigs() error {
	if err := c.EmbedCopier.Copy(
		configs.EmbededConfigs,
		files.EmbedCopierOp{Src: "embedded/broker-server/rpc.json", Dst: brokerDeployRpcConfig, Overwrite: false},
		files.EmbedCopierOp{Src: "embedded/broker-server/chainConfig.json", Dst: brokerDeployChainConfig, Overwrite: false},
		files.EmbedCopierOp{Src: "embedded/broker-server/docker-compose.yml", Dst: brokerDeployDockerCompose, Overwrite: false},
	); err != nil {
		return fmt.Errorf("copying configs to local file system: %w", err)
	}
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
	if _, err := c.EnsureEnvironment(cfg); err != nil {
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

	// Dest filenames for copying from embed. TODO - centralize this via flags
	if err := c.CopyBrokerDeployConfigs(); err != nil {
		return err
	}

	absChainConfig, err := filepath.Abs(brokerDeployChainConfig)
	if err != nil {
		return err
	}
	absRpcConfig, err := filepath.Abs(brokerDeployRpcConfig)
	if err != nil {
		return err
	}
	c.TUI.NewConfirmation(
		"Please review the configuration files and ensure values are correct before proceeding:" + "\n" +
			styles.AlertImportant.Render(absChainConfig+"\n"+absRpcConfig),
	)

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

	// Retrieve required information from user input
	pk := c.Input.brokerDeployInput.privateKey
	bsd.brokerFeeTBPS = c.Input.brokerDeployInput.feeTBPS

	// Upload the files and exec in ./broker directory
	fmt.Println(styles.ItalicText.Render("Copying files to broker-server..."))
	sshClient, err := c.CreateSSHConn(
		bsd.brokerServerIpAddr,
		c.DefaultClusterUserName,
		c.SshKeyPath,
	)
	if err != nil {
		return fmt.Errorf("establishing ssh connection: %w", err)
	}
	if err := sshClient.CopyFilesOverSftp(
		conn.SftpCopySrcDest{Src: brokerDeployChainConfig, Dst: "./broker/chainConfig.json"},
		conn.SftpCopySrcDest{Src: brokerDeployRpcConfig, Dst: "./broker/rpc.json"},
		conn.SftpCopySrcDest{Src: brokerDeployDockerCompose, Dst: "./broker/docker-compose.yml"},
	); err != nil {
		return err
	}

	// Optional. Copy the .env file to broker dir on server if it exists
	if _, err := os.Stat(brokerEnvFile); err == nil {
		if err := sshClient.CopyFilesOverSftp(
			conn.SftpCopySrcDest{Src: brokerEnvFile, Dst: "./broker/.env"},
		); err != nil {
			return err
		}
	}

	// Prepare the volume with unencrypted keyfile for storing private key which
	// will be encrypted on broker-server startup
	fmt.Println(styles.ItalicText.Render("Preparing Docker volumes..."))
	out, err := c.brokerServerKeyVolSetup(sshClient, pk)
	if err != nil {
		fmt.Printf("%s\n\n%s", out, styles.ErrorText.Render("Something went wrong during broker-server volume deployment ^^^"))
		return err
	}

	// Exec broker-server deployment cmd
	brokerPrivateIp, err := c.HostsCfg.GetBrokerPrivateIp()
	if err != nil || brokerPrivateIp == "" {
		return fmt.Errorf("broker_private_ip not found in hosts.cfg")
	}
	fmt.Println(styles.ItalicText.Render("Starting docker compose on broker-server..."))
	cmd := "cd ./broker && BROKER_FEE_TBPS=%s REDIS_PW=%s CHAIN_ID=%d BROKER_PRIVATE_IP=%s docker compose up -d"
	out, err = sshClient.ExecCommand(
		fmt.Sprintf(cmd, bsd.brokerFeeTBPS, redisPw, cfg.ChainId, brokerPrivateIp),
	)
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
	env, err := c.EnsureEnvironment(cfg)
	if err != nil {
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

	// Test and reload
	if out, err := sshConn.ExecCommand(fmt.Sprintf("echo '%s' | sudo -S nginx -t 2>&1", password)); err != nil {
		return fmt.Errorf("nginx config test failed:\n%s", string(out))
	}
	sshExecSudo(sshConn, password, "systemctl reload nginx")
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
		cmd := fmt.Sprintf("echo '%s' | sudo -S certbot --nginx -d %s --non-interactive --agree-tos -m %s 2>&1", password, brokerServerName, emailForCertbot)
		out, err := sshConn.ExecCommand(cmd)
		if err != nil {
			fmt.Printf("  %s certbot failed: %s\n", notok, strings.TrimSpace(string(out)))
		} else {
			fmt.Printf("  %s %s\n", ok, brokerServerName)
		}

		sshExecSudo(sshConn, password, "systemctl enable snap.certbot.renew.timer && systemctl start snap.certbot.renew.timer")
		if val, ok := cfg.Services[configs.D8XServiceBrokerServer]; ok {
			val.UsesHTTPS = true
			cfg.Services[configs.D8XServiceBrokerServer] = val
		}
		cfg.BrokerCertbotDeployed = true
		fmt.Println(styles.SuccessText.Render("Broker SSL setup done!"))
	}

	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return fmt.Errorf("could not update config: %w", err)
	}
	if err := c.PublishRemoteConfig(cfg); err != nil {
		fmt.Printf("  %s failed to sync remote config: %s\n", notok, err)
	}

	return nil
}

// certbotNginxSetup performs certificate issuance for given domains. Nginx and
// DNS A records must be setup beforehand.
func (c *Container) certbotNginxSetup(sshConn conn.SSHConnection, userSudoPassword, email string, domains []string) ([]byte, error) {
	cmd := fmt.Sprintf(
		`echo '%s' | sudo -S certbot --nginx -d %s -n  --agree-tos -m %s`,
		userSudoPassword,
		strings.Join(domains, ","),
		email,
	)

	return sshConn.ExecCommand(cmd)
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

	cmd := fmt.Sprintf("cd ./broker && docker volume create %s", BROKER_KEY_VOL_NAME)
	cmd = fmt.Sprintf("%s && echo -n '%s' > ./keyfile.txt", cmd, pk)
	cmd = fmt.Sprintf("%s && docker run --rm -v $PWD:/source -v %s:/dest -w /source alpine cp ./keyfile.txt /dest", cmd, BROKER_KEY_VOL_NAME)

	// Remove keyfile once volume is created
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
