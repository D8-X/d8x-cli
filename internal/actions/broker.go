package actions

import (
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
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

const BROKER_SERVER_REDIS_PWD_FILE = "./redis_broker_password.txt"

const BROKER_KEY_VOL_NAME = "keyvol"

func (c *Container) CollectBrokerInputs(ctx *cli.Context) error {
	// Make sure chain id is present in config
	chainId, err := c.GetChainId(ctx)
	if err != nil {
		return err
	}
	chainIdStr := strconv.Itoa(int(chainId))

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Collect HTTP rpc endpoints
	if err := c.CollectHTTPRPCUrls(cfg, chainIdStr); err != nil {
		return err
	}

	// Collect broker executor wallet address
	changeExecutorAddress := true
	if cfg.BrokerServerConfig.ExecutorAddress != "" {
		fmt.Printf("Found broker executor address: %s\n", cfg.BrokerServerConfig.ExecutorAddress)
		if ok, err := c.TUI.NewPrompt("Do you want to change broker executor address?", false); err != nil {
			return err
		} else if !ok {
			changeExecutorAddress = false
		}
	}
	if changeExecutorAddress {
		fmt.Println("Enter broker executor address:")
		brokerExecutorAddress, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("0x0000000000000000000000000000000000000000"),
			components.TextInputOptValue(cfg.BrokerServerConfig.ExecutorAddress),
		)
		if err != nil {
			return err
		}

		// Validate the address
		if !ValidWalletAddress(brokerExecutorAddress) {
			fmt.Println(styles.ErrorText.Render("invalid address provided, please set the allowedExecutors value in chainConfig.json"))
		} else {
			cfg.BrokerServerConfig.ExecutorAddress = brokerExecutorAddress
		}
	}

	return nil
}

func (c *Container) UpdateBrokerChainConfigJson(chainConfigPath string, cfg *configs.D8XConfig) error {
	contents, err := os.ReadFile(chainConfigPath)
	if err != nil {
		return err
	}

	chainConfig := []map[string]any{}

	if err := json.Unmarshal(contents, &chainConfig); err != nil {
		return err
	}

	for i, conf := range chainConfig {
		if int(conf["chainId"].(float64)) == int(cfg.ChainId) {
			conf["brokerPayoutAddr"] = []string{cfg.BrokerServerConfig.ExecutorAddress}
			chainConfig[i] = conf
			break
		}
	}

	out, err := json.MarshalIndent(chainConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(chainConfigPath, out, 0644)
}

// BrokerDeploy collects information related to broker-server
// deploymend, copies the configurations files to remote broker host and deploys
// the docker-compose d8x-broker-server setup.
func (c *Container) BrokerDeploy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting broker server deployment configuration...")

	if err := c.CollectBrokerInputs(ctx); err != nil {
		return err
	}

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

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
	var (
		chainConfig   = "./broker-server/chainConfig.json"
		rpcConfig     = "./broker-server/rpc.json"
		dockerCompose = "./broker-server/docker-compose.yml"
	)
	// Copy the config files and nudge user to review them
	if err := c.EmbedCopier.Copy(
		configs.EmbededConfigs,
		files.EmbedCopierOp{Src: "embedded/broker-server/rpc.json", Dst: rpcConfig, Overwrite: false},
		files.EmbedCopierOp{Src: "embedded/broker-server/chainConfig.json", Dst: chainConfig, Overwrite: false},
		files.EmbedCopierOp{Src: "embedded/broker-server/docker-compose.yml", Dst: dockerCompose, Overwrite: true},
	); err != nil {
		return err
	}

	// Upate rpc.json config
	httpWsGetter := c.getHttpWsRpcs(strconv.Itoa(int(cfg.ChainId)), cfg)
	rpconfigFilePath := "./broker-server/rpc.json"
	httpRpcs, _ := httpWsGetter()
	fmt.Printf("Updating %s config...\n", rpconfigFilePath)
	if err := c.editRpcConfigUrls(rpconfigFilePath, cfg.ChainId, nil, httpRpcs); err != nil {
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("Could not update %s, please double check the config file: %+v", rpconfigFilePath, err),
			),
		)
	}

	// Update chainConfig.json
	if err := c.UpdateBrokerChainConfigJson(chainConfig, cfg); err != nil {
		return err
	}

	absChainConfig, err := filepath.Abs(chainConfig)
	if err != nil {
		return err
	}
	absRpcConfig, err := filepath.Abs(rpcConfig)
	if err != nil {
		return err
	}
	c.TUI.NewConfirmation(
		"Please review the configuration files and ensure values are correct before proceeding:" + "\n" +
			styles.AlertImportant.Render(absChainConfig+"\n"+absRpcConfig),
	)

	// Generate and display broker-server redis password file
	redisPw, err := c.generatePassword(16)
	if err != nil {
		return fmt.Errorf("generating redis password: %w", err)
	}
	if err := c.FS.WriteFile(BROKER_SERVER_REDIS_PWD_FILE, []byte(redisPw)); err != nil {
		return fmt.Errorf("storing password in %s file: %w", BROKER_SERVER_REDIS_PWD_FILE, err)
	}
	fmt.Println(
		styles.SuccessText.Render("REDIS Password for broker-server was stored in " + BROKER_SERVER_REDIS_PWD_FILE + " file"),
	)

	// Collect required information
	fmt.Println("Enter your broker private key:")
	pk, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
		components.TextInputOptMasked(),
	)
	if err != nil {
		return err
	}
	if addr, err := PrivateKeyToAddress(pk); err != nil {
		return fmt.Errorf("ivalid private key: %w", err)
	} else {
		fmt.Printf("Provided broker address: %s\n\n", addr.String())
	}

	fmt.Println("Enter your broker fee percentage (%) value:")
	feePercentage, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("0.06"),
		components.TextInputOptValue("0.06"),
	)
	if err != nil {
		return err
	}
	if tbpsFromPercentage, err := convertPercentToTBPS(feePercentage); err != nil {
		return fmt.Errorf("invalid tbps value: %w", err)
	} else {
		bsd.brokerFeeTBPS = tbpsFromPercentage
	}

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
		conn.SftpCopySrcDest{Src: chainConfig, Dst: "./broker/chainConfig.json"},
		conn.SftpCopySrcDest{Src: rpcConfig, Dst: "./broker/rpc.json"},
		conn.SftpCopySrcDest{Src: dockerCompose, Dst: "./broker/docker-compose.yml"},
	); err != nil {
		return err
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
	fmt.Println(styles.ItalicText.Render("Starting docker compose on broker-server..."))
	cmd := "cd ./broker && BROKER_FEE_TBPS=%s REDIS_PW=%s docker compose up -d"
	out, err = sshClient.ExecCommand(
		fmt.Sprintf(cmd, bsd.brokerFeeTBPS, redisPw),
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

	fmt.Println(styles.SuccessText.Render("Broker server deployment done!"))

	return nil
}

func (c *Container) BrokerServerNginxCertbotSetup(ctx *cli.Context) error {
	styles.PrintCommandTitle("Performing nginx and certbot setup for broker server...")

	// Load config which we will later use to write details about broker sever
	// service.
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	nginxConfigNameTPL := "./nginx-broker.tpl.conf"
	nginxConfigName := "./nginx-broker.configured.conf"

	if err := c.EmbedCopier.Copy(
		configs.EmbededConfigs,
		files.EmbedCopierOp{Src: "embedded/nginx/nginx-broker.conf", Dst: nginxConfigNameTPL, Overwrite: true},
		files.EmbedCopierOp{Src: "embedded/playbooks/broker.ansible.yaml", Dst: "./playbooks/broker.ansible.yaml", Overwrite: true},
	); err != nil {
		return err
	}

	password, err := c.GetPassword(ctx)
	if err != nil {
		return err
	}

	brokerIpAddr, err := c.HostsCfg.GetBrokerPublicIp()
	if err != nil {
		fmt.Println(
			styles.ErrorText.Render("Broker server ip address was not found. Did you provision broker server?"),
		)
		return err
	}

	setupNginx, err := c.TUI.NewPrompt("Do you want to setup nginx for broker-server?", true)
	if err != nil {
		return err
	}
	setupCertbot, err := c.TUI.NewPrompt("Do you want to setup SSL with certbot for broker-server?", true)
	if err != nil {
		return err
	}
	emailForCertbot := ""
	if setupCertbot {
		fmt.Println("Enter your email address for certbot notifications: ")
		email, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("email@domain.com"),
		)
		if err != nil {
			return err
		}
		emailForCertbot = email
	}

	fmt.Println("Enter Broker-server HTTP (sub)domain (e.g. broker.d8x.xyz):")
	brokerServerName, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("your-broker.domain.com"),
	)
	if err != nil {
		return err
	}

	// Print alert about DNS
	fmt.Println(styles.AlertImportant.Render("Before proceeding with nginx and certbot setup, please ensure you have correctly added your DNS A records!"))
	fmt.Println("Broker server IP address:", brokerIpAddr)
	fmt.Println("Broker domain:", brokerServerName)
	c.TUI.NewConfirmation("Press enter to continue...")

	if setupNginx {
		fmt.Println(styles.ItalicText.Render("Setting up nginx for broker node"))

		if err := c.FS.ReplaceAndCopy(
			nginxConfigNameTPL,
			nginxConfigName,
			[]files.ReplacementTuple{
				{
					Find:    `%broker_server%`,
					Replace: brokerServerName,
				},
			},
		); err != nil {
			return fmt.Errorf("could not create nginx configuration: %w", err)
		}

		// Run ansible-playbook for nginx setup on broker server
		args := []string{
			"--extra-vars", fmt.Sprintf(`ansible_ssh_private_key_file='%s'`, c.SshKeyPath),
			"--extra-vars", "ansible_host_key_checking=false",
			"--extra-vars", fmt.Sprintf(`ansible_become_pass='%s'`, password),
			"-i", configs.DEFAULT_HOSTS_FILE,
			"-u", c.DefaultClusterUserName,
			"./playbooks/broker.ansible.yaml",
		}
		cmd := exec.Command("ansible-playbook", args...)
		connectCMDToCurrentTerm(cmd)
		if err := c.RunCmd(cmd); err != nil {
			return err
		} else {
			fmt.Println(styles.SuccessText.Render("Broker server nginx setup done!"))

			// Add config entry for the service
			cfg.Services[configs.D8XServiceBrokerServer] = configs.D8XService{
				Name:     configs.D8XServiceBrokerServer,
				HostName: brokerServerName,
			}
		}
	}

	if setupCertbot {
		fmt.Println(styles.ItalicText.Render("Setting up certbot for broker server..."))

		sshConn, err := c.CreateSSHConn(
			brokerIpAddr,
			c.DefaultClusterUserName,
			c.SshKeyPath,
		)
		if err != nil {
			return err
		}

		out, err := c.certbotNginxSetup(sshConn, password, emailForCertbot, []string{brokerServerName})
		fmt.Println(string(out))
		if err != nil {
			return err
		} else {
			fmt.Println(styles.SuccessText.Render("Broker server certificates setup done!"))

			// Update config
			if val, ok := cfg.Services[configs.D8XServiceBrokerServer]; ok {
				val.UsesHTTPS = true
				cfg.Services[configs.D8XServiceBrokerServer] = val
			}

		}
	}

	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return fmt.Errorf("could not update config: %w", err)
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
