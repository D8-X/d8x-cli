package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/D8-X/d8x-cli/internal/actions"
	"github.com/D8-X/d8x-cli/internal/configs"
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

// RunD8XCli is the entrypoint to D8X cli tool
func RunD8XCli() {
	container, err := actions.NewDefaultContainer()
	if err != nil {
		log.Fatal(err)
	}

	// Shared flags below

	// Default terraform files and state directory (./terraform)
	provisionTfDirFlag := &cli.StringFlag{
		Name:        "tf-dir",
		Value:       "./terraform",
		Required:    false,
		Usage:       "Terraform files directory path. Used for backwards compatibility only.",
		Destination: &container.ProvisioningTfDir,
	}

	// Initialize cli application and its subcommands and bind default values
	// for ac (via flags.Destination)
	app := &cli.App{
		Name:                 CmdName,
		HelpName:             CmdName,
		Usage:                CmdUsage,
		Description:          MainDescription,
		EnableBashCompletion: true,
		CommandNotFound: func(ctx *cli.Context, s string) {
			fmt.Printf("Unknown command %s\n", s)
		},
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
				Before: func(ctx *cli.Context) error {
					// Retrieve the user password whenever possible
					if container.UserPassword == "" {
						pwd, err := container.GetPassword(ctx)
						if err == nil && len(pwd) > 0 {
							container.UserPassword = pwd
							fmt.Printf("User password retrieved from %s\n", configs.DEFAULT_PASSWORD_FILE)
						}
					}

					subcommands := []string{
						"provision",
						"configure",
						"broker-deploy",
						"broker-nginx",
						"swarm-deploy",
						"swarm-nginx",
						"metrics-deploy",

						// Help is always included
						"help",
					}

					subcommand := ctx.Args().First()

					if subcommand != "" && slices.Index(subcommands, subcommand) == -1 {
						return fmt.Errorf("setup command does not have subcommand %s", subcommand)
					}

					return nil
				},
				Flags: []cli.Flag{provisionTfDirFlag},
				Subcommands: []*cli.Command{
					{
						Name:        "provision",
						Usage:       "Provision server resources with terraform",
						Action:      container.Provision,
						Description: ProvisionDescription,
						Flags:       []cli.Flag{provisionTfDirFlag},
					},
					{
						Name:        "configure",
						Usage:       "Configure servers with ansible",
						Action:      container.Configure,
						Description: ConfigureDescription,
					},
					{
						Name:   "broker-deploy",
						Usage:  "Deploy and configure broker-server deployment",
						Action: container.BrokerDeploy,
					},
					{
						Name:   "broker-nginx",
						Usage:  "Configure and setup nginx + certbot for broker server deployment",
						Action: container.BrokerServerNginxCertbotSetup,
					},
					{
						Name:        "swarm-deploy",
						Usage:       "Deploy and configure d8x-trader-backend swarm cluster",
						Action:      container.SwarmDeploy,
						Description: SwarmDeployDescription,
					},
					{
						Name:        "swarm-nginx",
						Usage:       "Configure and setup nginx + certbot for d8x-trader swarm deployment",
						Action:      container.SwarmNginx,
						Description: SwarmNginxDescription,
					},
					{
						Name:        "metrics-deploy",
						Usage:       "Deploy and configure metrics services (prometheus, grafana) on manager node",
						Action:      container.DeployMetrics,
						Description: DeployMetricsDescription,
					},
				},
			},
			{
				Name:   "update",
				Usage:  "Update service with new image version",
				Action: container.ServiceUpdate,
			},
			{
				Name:   "health",
				Usage:  "Perform health checks of deployed services",
				Action: container.HealthCheck,
			},
			{
				Name:      "ip",
				Usage:     "Retrieve node ip addresses",
				ArgsUsage: "manager|broker",
				Action:    container.Ips,
			},
			{
				Name:   "tf-destroy",
				Usage:  "Run terraform destroy for current setup",
				Action: container.TerraformDestroy,
			},
			{
				Name:   "ssh",
				Usage:  "Attach ssh session to one of your servers",
				Action: container.SSH,
			},
			{
				Name:      "grafana-tunnel",
				Usage:     "Create ssh tunnel to grafana service on manager",
				Action:    container.TunnelGrafana,
				ArgsUsage: "[port 8080]",
			},
			{
				Name:        "cp-configs",
				ArgsUsage:   "swarm|broker|tf-aws|tf-linode",
				Action:      container.CopyConfigs,
				Usage:       "Copy configuration files to current working directory",
				Description: "Copy specified configuration files to current working directory. Available configs are swarm, broker, tf-aws, tf-linode.",
			},
			{
				Name:   "backup-db",
				Action: container.BackupDb,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "output-dir",
						Usage: "Backup directory path. Backup files will be saved in this directory.",
					},
				},
				Description: "Backup database to local machine. Database credentials are read from d8x.conf.json file.",
			},
			{
				Name:        "db-tunnel",
				Action:      container.DbTunnel,
				ArgsUsage:   "[local port 5432]",
				Description: "Create a ssh tunnel to database server. Database credentials are read from d8x.conf.json file.",
			},
			{
				Name:   "fix-ingress",
				Usage:  "Fix faulty ingress network",
				Action: container.IngressFix,
			},
		},
		// Global flags accessible to all subcommands
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
				Value:       configs.DEFAULT_USER_NAME,
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
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "Suppress verbose output",
			},
		},
		Action: func(ctx *cli.Context) error {
			// Disallow running d8x with incorrect subcommands
			if ctx.Args().Len() == 0 {
				return container.Init(ctx)
			}
			return fmt.Errorf("unknown command %s, check --help for more info about available commands", ctx.Args().First())
		},
		Version: version.Get(),
		Before: func(ctx *cli.Context) error {
			// Cached ChainJson information
			chainJsonData, err := container.LoadChainJson()
			if err != nil {
				return fmt.Errorf("loading chain json information: %w", err)
			}

			// Create d8x.conf.json config read writer. We can only do this here,
			// because config directory is not know when initializing containter
			container.ConfigRWriter = configs.NewFileBasedD8XConfigRW(
				filepath.Join(container.ConfigDir, configs.DEFAULT_D8X_CONFIG_NAME),
			)

			// Initialize the input collector
			container.Input = &actions.InputCollector{
				ConfigRWriter: container.ConfigRWriter,
				TUI:           container.TUI,
				ChainJson:     chainJsonData,
				SSHKeyPath:    container.SshKeyPath,
			}

			// Chdir functionality
			if ch := ctx.String("chdir"); ch != "" {
				err := os.Chdir(ch)
				if err != nil {
					return fmt.Errorf("changing directory: %w", err)
				}
			}

			// Welcome msg
			if !ctx.Bool("quiet") {
				fmt.Println(
					styles.PurpleBgText.
						Copy().
						Padding(0, 2, 0, 2).
						Border(lipgloss.NormalBorder()).
						Render(D8XASCII),
				)
			}

			// Create config directory if it does not exist already
			if err := container.MakeConfigDir(); err != nil {
				return fmt.Errorf("could not create config directory: %w", err)
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
