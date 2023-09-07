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
	"golang.org/x/crypto/ssh"
)

func (c *Container) BrokerServerDeployment(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting broker server deployment and nginx configuration...")

	// Dest filenames, TODO - centralize this via flags
	var (
		chainConfig   = "./chainConfig.json"
		dockerCompose = "./docker-compose-broker-server.yml"
	)
	// Copy the config files and prompt user to edit it
	if err := c.EmbedCopier.Copy(
		configs.BrokerServerConfigs,
		chainConfig,
		"broker-server/chainConfig.json",
	); err != nil {
		return err
	}
	if err := c.EmbedCopier.Copy(
		configs.BrokerServerConfigs,
		dockerCompose,
		"broker-server/docker-compose.yml",
	); err != nil {
		return err
	}

	absChainConfig, err := filepath.Abs(chainConfig)
	if err != nil {
		return err
	}
	components.NewConfirmation(
		"Please review the configuration file and ensure values are correct before proceeding:" + "\n" +
			styles.AlertImportant.Render(absChainConfig),
	)

	bsd := brokerServerDeployment{}

	brokerIpAddr, err := c.getBrokerServerIp()
	if err != nil {
		return err
	}
	bsd.brokerServerIpAddr = brokerIpAddr

	// Collect required information
	fmt.Println("Enter your broker private key:")
	pk, err := components.NewInput(
		components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
	)
	if err != nil {
		return err
	}
	bsd.brokerKey = pk
	fmt.Println("Enter your broker fee tbps value:")
	tbps, err := components.NewInput(
		components.TextInputOptPlaceholder("60"),
		components.TextInputOptValue("60"),
	)
	if err != nil {
		return err
	}
	bsd.brokerFeeTBPS = tbps
	password, err := c.getPassword(ctx)
	if err != nil {
		return err
	}

	// Upload the files and exec in ./broker directory
	fmt.Println(styles.ItalicText.Render("Copying files to broker-server..."))
	sshClient, err := conn.NewSSHClient(
		bsd.brokerServerIpAddr,
		c.DefaultClusterUserName,
		c.SshKeyPath,
	)
	if err != nil {
		return fmt.Errorf("establishing ssh connection: %w", err)
	}
	if err := conn.CopyFilesOverSftp(sshClient,
		conn.SftpCopySrcDest{Src: chainConfig, Dst: "./broker/chainConfig.json"},
		conn.SftpCopySrcDest{Src: dockerCompose, Dst: "./broker/docker-compose.yml"},
	); err != nil {
		return err
	}

	// Exec broker-server deployment cmd
	fmt.Println(styles.ItalicText.Render("Starting docker compose on broker-server..."))
	cmd := "cd ./broker && echo '%s' | sudo -S BROKER_KEY=%s BROKER_FEE_TBPS=%s docker compose up -d"
	out, err := conn.SSHExecCommand(
		sshClient,
		fmt.Sprintf(cmd, password, bsd.brokerKey, bsd.brokerFeeTBPS),
	)
	if err != nil {
		fmt.Printf("%s\n\n%s", out, styles.ErrorText.Render("Something went wrong during broker-server deployment ^^^"))
		return err
	} else {
		fmt.Println(styles.SuccessText.Render("broker-server deployed!"))
	}

	return nil
}

func (c *Container) getBrokerServerIp() (string, error) {
	hostsCfg, err := c.LoadHostsFile("./hosts.cfg")
	if err != nil {
		return "", err
	}

	brokerIpAddr, err := hostsCfg.GetBrokerPublicIp()
	if err != nil {
		return "", fmt.Errorf("could not determine broker ip address: %w", err)
	}
	return brokerIpAddr, nil
}

func (c *Container) BrokerServerNginxCertbotSetup(ctx *cli.Context) error {
	styles.PrintCommandTitle("Performing nginx and certbot setup for broker server...")

	nginxConfigNameTPL := "./nginx-broker.tpl.conf"
	nginxConfigName := "./nginx-broker.configured.conf"
	if err := c.EmbedCopier.Copy(
		configs.NginxConfigs,
		nginxConfigNameTPL,
		"nginx/nginx-broker.conf",
	); err != nil {
		return err
	}
	if err := c.EmbedCopier.Copy(
		configs.AnsiblePlaybooks,
		"./playbooks/broker.ansible.yaml",
		"playbooks/broker.ansible.yaml",
	); err != nil {
		return err
	}

	password, err := c.getPassword(ctx)
	if err != nil {
		return err
	}

	brokerIpAddr, err := c.getBrokerServerIp()
	if err != nil {
		return err
	}

	fmt.Println(styles.AlertImportant.Render("Before proceeding with nginx and certbot setup, please ensure you have correctly added your DNS A records!"))
	fmt.Println("Broker server IP address: ", brokerIpAddr)
	setupNginx, err := components.NewPrompt("Do you want to setup nginx for broker-server?", true)
	if err != nil {
		return err
	}
	setupCertbot, err := components.NewPrompt("Do you want to setup SSL with certbot for broker-server?", true)
	if err != nil {
		return err
	}
	emailForCertbot := ""
	if setupCertbot {
		fmt.Println("Enter your email address for certbot notifications: ")
		email, err := components.NewInput(
			components.TextInputOptPlaceholder("email@domain.com"),
		)
		if err != nil {
			return err
		}
		emailForCertbot = email
	}

	fmt.Println("Enter Broker-server HTTP (sub)domain (e.g. broker.d8x.xyz):")
	brokerServerName, err := components.NewInput(
		components.TextInputOptPlaceholder("your-broker.domain.com"),
	)
	if err != nil {
		return err
	}

	if setupNginx {
		fmt.Println(styles.ItalicText.Render("Setting up nginx for manager node"))

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
			"-i", "./hosts.cfg",
			"-u", c.DefaultClusterUserName,
			"./playbooks/broker.ansible.yaml",
		}
		cmd := exec.Command("ansible-playbook", args...)
		connectCMDToCurrentTerm(cmd)
		if err := cmd.Run(); err != nil {
			return err
		} else {
			fmt.Println(styles.SuccessText.Render("Broker server nginx setup done!"))
		}
	}

	if setupCertbot {
		sshClient, err := conn.NewSSHClient(
			brokerIpAddr,
			c.DefaultClusterUserName,
			c.SshKeyPath,
		)
		if err != nil {
			return err
		}

		out, err := c.certbotNginxSetup(sshClient, password, emailForCertbot, []string{brokerServerName})
		fmt.Println(string(out))
		if err != nil {
			return err
		} else {
			fmt.Println(styles.SuccessText.Render("Broker server certificates setup done!"))
		}
	}

	return nil
}

// certbotNginxSetup performs certificate issuance for given domains. Nginx and
// DNS A records must be setup beforehand.
func (c *Container) certbotNginxSetup(sshClient *ssh.Client, userSudoPassword, email string, domains []string) ([]byte, error) {
	cmd := fmt.Sprintf(
		`echo '%s' | sudo -S certbot --nginx -d %s -n  --agree-tos -m %s`,
		userSudoPassword,
		strings.Join(domains, ","),
		email,
	)

	return conn.SSHExecCommand(sshClient, cmd)
}

type brokerServerDeployment struct {
	brokerKey     string
	brokerFeeTBPS string

	brokerServerIpAddr string
}