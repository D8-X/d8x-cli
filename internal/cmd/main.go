package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/D8-X/d8x-cli/internal/actions"
	"github.com/D8-X/d8x-cli/internal/flags"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/D8-X/d8x-cli/internal/version"
	"github.com/charmbracelet/lipgloss"
	"github.com/urfave/cli/v2"
)

const D8XASCII = ` ____     ___   __  __
|  _ \   ( _ )  \ \/ /
| | | |  / _ \   \  / 
| |_| | | (_) |  /  \ 
|____/   \___/  /_/\_\
`

// CmdName defines the name of cli tool
const CmdName = "d8x"

const CmdUsage = "D8X Backend management CLI tool"

// MainDescription is the description text for d8x cli tool
const MainDescription = `D8X Perpetual Exchange broker backend setup and management CLI tool 

<More description entered here>

Running d8x without any subcommands or init command will perform initalization
of ./.d8x-config directory (--config-directory), as well as prompt you to
install any missing dependencies.

D8X CLI relies on the following external tools: terraform, ansible. You can
manually install them or let the cli attempt to perform the installation of
these dependencies automatically. Note that for automatic installation you will
need to have python3 and pip installed on your system
`

const SetupDescription = `Command setup performs complete D8X cluster setup.

In essence setup calls the following subcommands in sequence:
	- provision
	- configure
	- broker-deploy
	- broker-nginx
	- swarm-deploy
	- swarm-nginx

See individual command's help for information how each step operates.

`

// RunD8XCli is the entrypoint to D8X cli tool
func RunD8XCli() {
	container := actions.NewDefaultContainer()

	// Initialize cli application and its subcommands and bind default values
	// for ac (via flags.Destination)
	app := &cli.App{
		Name:        CmdName,
		HelpName:    CmdName,
		Usage:       CmdUsage,
		Description: MainDescription,
		Commands: []*cli.Command{
			{
				Name:   "init",
				Action: container.Init,
				Usage:  "Initialize configuration directory and install dependencies",
			},
			{
				Name:        "setup",
				Usage:       "Full setup of d8x backend cluster",
				Description: SetupDescription,
				Action:      container.Setup,
				Subcommands: []*cli.Command{
					{
						Name:   "provision",
						Usage:  "Provision server resources with terraform",
						Action: container.Provision,
					},
					{
						Name:   "configure",
						Usage:  "Configure servers with ansible",
						Action: container.Configure,
					},
					{
						Name:   "broker-deploy",
						Usage:  "Deploy and configure broker-server deployment",
						Action: container.BrokerServerDeployment,
					},
					{
						Name:   "broker-nginx",
						Usage:  "Configure and setup nginx + certbot for broker server deployment",
						Action: container.BrokerServerNginxCertbotSetup,
					},
					{
						Name:   "swarm-deploy",
						Usage:  "Deploy and configure d8x-trader-backend swarm cluster",
						Action: container.SwarmDeploy,
					},
					{
						Name:   "swarm-nginx",
						Usage:  "Configure and setup nginx + certbot for d8x-trader swarm deployment",
						Action: container.SwarmNginx,
					},
				},
			},
		},
		// Global flags accesible to all subcommands
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: flags.ConfigDir,
				// Set the defaul path to configuration directory on user's home
				// dir
				Value:       "./.d8x-config",
				Destination: &container.ConfigDir,
				Usage:       "Configs and secrets directory",
			},
			&cli.StringFlag{
				Name:        flags.PrivateKeyPath,
				Value:       "./id_ed25519",
				Destination: &container.SshKeyPath,
				Usage:       "Default ssh key path used to access servers",
			},
			&cli.StringFlag{
				Name:        flags.User,
				Value:       "d8xtrader",
				Destination: &container.DefaultClusterUserName,
				Usage:       "User which will be created on each server during provisioning and configuration. Also used ssh'ing into servers.",
			},
			&cli.StringFlag{
				Name:        flags.Password,
				Destination: &container.UserPassword,
				Usage:       "User's password used for tasks requiring elevated permissions, if not provided, default password file will be read.",
			},
			&cli.StringFlag{
				Name:  "chdir",
				Usage: "Change directory to provided one before executing anything",
			},
			&cli.StringFlag{
				Name:        flags.PgCertPath,
				Destination: &container.PgCrtPath,
				Value:       "./pg.crt",
				Usage:       "pg.crt certificate path",
			},
		},
		Action:  container.Init,
		Version: version.Get(),
		Before: func(ctx *cli.Context) error {
			if ch := ctx.String("chdir"); ch != "" {
				err := os.Chdir(ch)
				if err != nil {
					return fmt.Errorf("changing directory: %w", err)
				}
			}

			fmt.Println(styles.PurpleBgText.Copy().Padding(0, 2, 0, 2).Border(lipgloss.NormalBorder()).Render(D8XASCII))
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}