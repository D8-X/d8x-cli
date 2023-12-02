package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func (c *Container) CollectBrokerFee() (string, error) {
	fmt.Println("Enter your broker fee percentage (%) value:")
	feePercentage, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("0.06"),
		components.TextInputOptValue("0.06"),
		components.TextInputOptEnding("%"),
	)
	if err != nil {
		return "", err
	}
	tbpsFromPercentage, err := convertPercentToTBPS(feePercentage)
	if err != nil {
		fmt.Println(styles.ErrorText.Render("invalid tbps value: " + err.Error()))
		return c.CollectBrokerFee()
	}

	return tbpsFromPercentage, nil
}

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

	// Collect broker (referral) executor wallet address
	changeExecutorAddress := true
	if cfg.BrokerServerConfig.ExecutorAddress != "" {
		fmt.Printf("Found referral executor address: %s\n", cfg.BrokerServerConfig.ExecutorAddress)
		if ok, err := c.TUI.NewPrompt("Do you want to change referral executor address?", false); err != nil {
			return err
		} else if !ok {
			changeExecutorAddress = false
		}
	}
	if changeExecutorAddress {
		brokerExecutorAddress, err := c.CollectAndValidateWalletAddress("Enter referral executor address:", cfg.BrokerServerConfig.ExecutorAddress)
		if err != nil {
			return err
		}

		cfg.BrokerServerConfig.ExecutorAddress = brokerExecutorAddress
	}

	return c.ConfigRWriter.Write(cfg)
}

// UpdateBrokerChainConfigJson appends allowedExecutor addresses
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
			executors := []string{}
			if len(cfg.BrokerServerConfig.ExecutorAddress) > 0 {
				executors = append(executors, cfg.BrokerServerConfig.ExecutorAddress)
			}

			// Make sure we don't overwrite existing allowedExecutors
			v, ok := conf["allowedExecutors"].([]any)
			if ok {
				for _, executorAddr := range v {
					if a, ok2 := executorAddr.(string); ok2 {
						executors = append(executors, a)
					}
				}
			}
			executors = slices.Compact(executors)
			conf["allowedExecutors"] = executors

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

func (c *Container) GetBrokerChainConfigJsonAllowedExecutors(chainConfigPath string, cfg *configs.D8XConfig) ([]string, error) {
	contents, err := os.ReadFile(chainConfigPath)
	if err != nil {
		return nil, err
	}

	chainConfig := []map[string]any{}

	if err := json.Unmarshal(contents, &chainConfig); err != nil {
		return nil, err
	}

	allowedExecutors := []string{}
	for _, conf := range chainConfig {
		if int(conf["chainId"].(float64)) == int(cfg.ChainId) {
			v, ok := conf["allowedExecutors"].([]any)
			if ok {
				allowedExecutors = make([]string, len(v))
				for i, executorAddr := range v {
					if a, ok2 := executorAddr.(string); ok2 {
						allowedExecutors[i] = a
					}
				}
			}
		}
	}
	return allowedExecutors, nil
}

// BrokerDeploy collects information related to broker-server
// deploymend, copies the configurations files to remote broker host and deploys
// the docker-compose d8x-broker-server setup.
func (c *Container) BrokerDeploy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting broker server deployment configuration...")

	guideUser, err := c.TUI.NewPrompt("Would you like the cli to guide you through the configuration?", true)
	if err != nil {
		return err
	}
	if guideUser {
		if err := c.CollectBrokerInputs(ctx); err != nil {
			return err
		}
	}

	cfg, err := c.ConfigRWriter.Read()
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

	if guideUser {
		// Upate rpc.json config
		rpconfigFilePath := "./broker-server/rpc.json"
		httpRpcs, _ := DistributeRpcs(
			// Broker is #3 in the list
			3,
			strconv.Itoa(int(cfg.ChainId)),
			cfg,
		)

		fmt.Printf("Updating %s config...\n", rpconfigFilePath)
		if err := c.editRpcConfigUrls(rpconfigFilePath, cfg.ChainId, nil, httpRpcs); err != nil {
			fmt.Println(
				styles.ErrorText.Render(
					fmt.Sprintf("Could not update %s, please double check the config file: %+v", rpconfigFilePath, err),
				),
			)
		}

		// Update chainConfig.json
		fmt.Printf("Updating %s config...\n", chainConfig)
		if err := c.UpdateBrokerChainConfigJson(chainConfig, cfg); err != nil {
			return err
		}
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
	pk, _, err := c.CollectAndValidatePrivateKey("Enter your broker private key:")
	if err != nil {
		return err
	}

	tbpsFromPercentage, err := c.CollectBrokerFee()
	if err != nil {
		return err
	}
	bsd.brokerFeeTBPS = tbpsFromPercentage

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

	// Collect domain name
	_, err = c.CollectSetupDomain(cfg)
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
		email, err := c.CollectCertbotEmail(cfg)
		if err != nil {
			return err
		}
		emailForCertbot = email
	}

	domainValue := cfg.SuggestSubdomain(configs.D8XServiceBrokerServer, c.getChainType(strconv.Itoa(int(cfg.ChainId))))
	if v, ok := cfg.Services[configs.D8XServiceBrokerServer]; ok {
		if v.HostName != "" {
			domainValue = v.HostName
		}
	}
	brokerServerName, err := c.CollectInputWithConfirmation(
		"Enter Broker-server HTTP (sub)domain (e.g. broker.d8x.xyz):",
		"Is this correct?",
		components.TextInputOptPlaceholder("your-broker.domain.com"),
		components.TextInputOptValue(domainValue),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return err
	}
	brokerServerName = TrimHttpsPrefix(brokerServerName)

	fmt.Printf("Using broker domain: %s\n", brokerServerName)

	// Print alert about DNS
	fmt.Println(styles.AlertImportant.Render("Please create the following DNS record on your domain provider's website now:"))
	fmt.Println("Hostname:", brokerServerName)
	fmt.Println("Type: A")
	fmt.Println("IP address:", brokerIpAddr)

	c.TUI.NewConfirmation("Press enter when done...")

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
			restart, err2 := c.TUI.NewPrompt("Certbot setup failed, do you want to restart the broker-nginx setup?", true)
			if err2 != nil {
				return err2
			}
			if restart {
				return c.BrokerServerNginxCertbotSetup(ctx)
			}
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
