package actions

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

// BrokerDeploy collects information related to broker-server
// deploymend, copies the configurations files to remote broker host and deploys
// the docker-compose d8x-broker-server setup.
func (c *Container) BrokerDeploy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting broker server deployment configuration...")

	// Dest filenames, TODO - centralize this via flags
	var (
		chainConfig   = "./broker-server/chainConfig.json"
		dockerCompose = "./broker-server/docker-compose.yml"
	)
	// Copy the config files and nudge user to review them
	if err := c.EmbedCopier.Copy(
		configs.EmbededConfigs,
		files.EmbedCopierOp{Src: "embedded/broker-server/chainConfig.json", Dst: chainConfig, Overwrite: false},
		files.EmbedCopierOp{Src: "embedded/broker-server/docker-compose.yml", Dst: dockerCompose, Overwrite: true},
	); err != nil {
		return err
	}
	absChainConfig, err := filepath.Abs(chainConfig)
	if err != nil {
		return err
	}
	c.TUI.NewConfirmation(
		"Please review the configuration file and ensure values are correct before proceeding:" + "\n" +
			styles.AlertImportant.Render(absChainConfig),
	)

	bsd := brokerServerDeployment{}

	brokerIpAddr, err := c.HostsCfg.GetBrokerPublicIp()
	if err != nil {
		return err
	}
	bsd.brokerServerIpAddr = brokerIpAddr

	// Collect required information
	fmt.Println("Enter your broker private key:")
	pk, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
		components.TextInputOptMasked(),
	)
	if err != nil {
		return err
	}
	bsd.brokerKey = pk
	fmt.Println("Enter your broker fee tbps value:")
	tbps, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("60"),
		components.TextInputOptValue("60"),
	)
	if err != nil {
		return err
	}
	bsd.brokerFeeTBPS = tbps
	password, err := c.GetPassword(ctx)
	if err != nil {
		return err
	}

	// redis password
	redisPw, err := c.generatePassword(16)
	if err != nil {
		return fmt.Errorf("generating password: %w", err)
	}
	if err := c.FS.WriteFile("./redis_broker_password.txt", []byte(redisPw)); err != nil {
		return fmt.Errorf("storing password in ./redis_broker_password.txt file: %w", err)
	}
	fmt.Println(
		styles.SuccessText.Render("REDIS Password for broker-server was stored in ./redis_broker_password.txt file"),
	)

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
		conn.SftpCopySrcDest{Src: dockerCompose, Dst: "./broker/docker-compose.yml"},
	); err != nil {
		return err
	}

	// Exec broker-server deployment cmd
	fmt.Println(styles.ItalicText.Render("Starting docker compose on broker-server..."))
	cmd := "cd ./broker && echo '%s' | sudo -S BROKER_KEY=%s BROKER_FEE_TBPS=%s REDIS_PW=%s docker compose up -d"
	out, err := sshClient.ExecCommand(
		fmt.Sprintf(cmd, password, bsd.brokerKey, bsd.brokerFeeTBPS, redisPw),
	)
	if err != nil {
		fmt.Printf("%s\n\n%s", out, styles.ErrorText.Render("Something went wrong during broker-server deployment ^^^"))
		return err
	} else {
		fmt.Println(styles.SuccessText.Render("broker-server deployed!"))
	}

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
	brokerKey     string
	brokerFeeTBPS string

	brokerServerIpAddr string
}
