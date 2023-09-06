package actions

import (
	"fmt"
	"path/filepath"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
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

	hostsCfg, err := files.LoadHostsFile("./hosts.cfg")
	if err != nil {
		return err
	}

	brokerIpAddr, err := hostsCfg.GetBrokerPublicIp()
	if err != nil {
		return fmt.Errorf("could not determine broker ip address: %w", err)
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

	// Nginx setup
	// ok, err := components.NewPrompt("Do you want to setup nginx for broker-server?", true)
	// if err != nil {
	// 	return err
	// }
	// if ok {
	// 	fmt.Println(styles.ItalicText.Render("Setting up nginx for broker-server..."))
	// }

	return nil
}

type brokerServerDeployment struct {
	brokerKey     string
	brokerFeeTBPS string

	brokerServerIpAddr string
}
