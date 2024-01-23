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

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

const BROKER_SERVER_REDIS_PWD_FILE = "./redis_broker_password.txt"

const BROKER_KEY_VOL_NAME = "keyvol"

// UpdateBrokerChainConfigAllowedExecutors is updateFn for UpdateConfig for
// broker-server/chainConfig.json configuration. It updates allowedExecutors
// field and appends allowedExecutorAddress to the list for all chain ids.
func UpdateBrokerChainConfigAllowedExecutors(allowedExecutorAddress string) func(*[]map[string]any) error {
	return func(chainConfig *[]map[string]any) error {
		for i, conf := range *chainConfig {
			executors := []string{}
			if len(allowedExecutorAddress) > 0 {
				executors = append(executors, allowedExecutorAddress)
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
			// Update the entry
			(*chainConfig)[i] = conf
		}
		return nil
	}
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

var (
	brokerDeployChainConfig   = "./broker-server/chainConfig.json"
	brokerDeployRpcConfig     = "./broker-server/rpc.json"
	brokerDeployDockerCompose = "./broker-server/docker-compose.yml"
)

func (c *Container) CopyBrokerDeployConfigs() error {
	if err := c.EmbedCopier.Copy(
		configs.EmbededConfigs,
		files.EmbedCopierOp{Src: "embedded/broker-server/rpc.json", Dst: brokerDeployRpcConfig, Overwrite: false},
		files.EmbedCopierOp{Src: "embedded/broker-server/chainConfig.json", Dst: brokerDeployChainConfig, Overwrite: false},
		files.EmbedCopierOp{Src: "embedded/broker-server/docker-compose.yml", Dst: brokerDeployDockerCompose, Overwrite: true},
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

	if err := c.Input.CollectBrokerDeployInput(ctx); err != nil {
		return fmt.Errorf("collecting broker deploy input: %w", err)
	}

	// Refresh the cfg after input was collected
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
	if err := c.CopyBrokerDeployConfigs(); err != nil {
		return err
	}

	// Update chainConfig.json with referral executor address
	fmt.Printf("Updating %s config...\n", brokerDeployChainConfig)
	if err := UpdateConfig[[]map[string]any](
		brokerDeployChainConfig,
		UpdateBrokerChainConfigAllowedExecutors(
			cfg.BrokerServerConfig.ExecutorAddress,
		),
	); err != nil {
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

	// Generate and display broker-server redis password file
	redisPw, err := generatePassword(16)
	if err != nil {
		return fmt.Errorf("generating redis password: %w", err)
	}
	if err := c.FS.WriteFile(BROKER_SERVER_REDIS_PWD_FILE, []byte(redisPw)); err != nil {
		return fmt.Errorf("storing password in %s file: %w", BROKER_SERVER_REDIS_PWD_FILE, err)
	}
	fmt.Println(
		styles.SuccessText.Render("REDIS Password for broker-server was stored in " + BROKER_SERVER_REDIS_PWD_FILE + " file"),
	)

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

	if err := c.Input.CollectBrokerNginxInput(ctx); err != nil {
		return err
	}

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

	setupCertbot := c.Input.brokerNginxInput.setupCertbot
	setupNginx := c.Input.brokerNginxInput.setupNginx
	emailForCertbot := cfg.CertbotEmail
	brokerServerName := c.Input.brokerNginxInput.domainName

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

			// Update state
			cfg.BrokerNginxDeployed = true
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

			// Update state
			cfg.BrokerCertbotDeployed = true
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
