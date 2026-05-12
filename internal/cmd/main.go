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

const D8XASCII = `                          D8X CLI                          `

// CmdName defines the name of cli tool
const CmdName = "d8x"

const CmdUsage = ""

// RunD8XCli is the entrypoint to D8X cli tool
func RunD8XCli() {
	container, err := actions.NewDefaultContainer()
	if err != nil {
		log.Fatal(err)
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
					if container.UserPassword == "" {
						if pwd := ctx.String(flags.Password); pwd != "" {
							container.UserPassword = pwd
						}
					}

					subcommands := []string{
						"new-env",
						"provision", "prov",
						"configure", "config",
						"broker-deploy",
						"broker-nginx",
						"swarm-deploy", "sd",
						"swarm-nginx", "sn",
						"metrics-deploy",
						"staging-origins", "so",
						"rpc",

						// Help is always included
						"help",
					}

					subcommand := ctx.Args().First()

					if subcommand != "" && slices.Index(subcommands, subcommand) == -1 {
						return fmt.Errorf("setup command does not have subcommand %s", subcommand)
					}

					return nil
				},
				Subcommands: []*cli.Command{
					{
						Name: "new-env",
						Usage:   "Create a new environment in the infra repo",
						Action:  container.NewEnvironment,
					},
					{
						Name:        "provision",
						Aliases:     []string{"prov"},
						Usage:       "Provision server resources with terraform",
						Action:      container.Provision,
						Description: ProvisionDescription,
					},
					{
						Name:        "configure",
						Aliases:     []string{"config"},
						Usage:       "Configure servers with ansible",
						Action:      container.Configure,
						Description: ConfigureDescription,
					},
					{
						Name: "broker-deploy",
						Usage:   "Deploy and configure broker-server deployment",
						Action: container.BrokerDeploy,
					},
					{
						Name: "broker-nginx",
						Usage:   "Configure and setup nginx + certbot for broker server deployment",
						Action: container.BrokerServerNginxCertbotSetup,
					},
					{
						Name:        "swarm-deploy",
						Aliases:     []string{"sd"},
						Usage:       "Deploy and configure d8x-trader-backend swarm cluster",
						Action:      container.SwarmDeploy,
						Description: SwarmDeployDescription,
					},
					{
						Name:        "swarm-nginx",
						Aliases:     []string{"sn"},
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
					{
						Name:    "staging-origins",
						Aliases: []string{"so"},
						Usage:   "Update whitelisted staging origins",
						Action:  container.UpdateStagingOrigins,
					},
					{
						Name:   "rpc",
						Usage:  "View, add, or remove RPC URLs on the live cluster",
						Action: container.SetupRpc,
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
				Name:        "tf-destroy",
				Usage:       "Destroy all provisioned servers and infrastructure for an environment (irreversible)",
				Action:      container.TerraformDestroy,
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
				Description: "Backup database to local machine. Database credentials are loaded from the selected environment in the infra repo and Bitwarden.",
			},
			{
				Name:        "db-tunnel",
				Action:      container.DbTunnel,
				ArgsUsage:   "[local port 5432]",
				Description: "Create a ssh tunnel to database server. Database credentials are loaded from the selected environment in the infra repo and Bitwarden.",
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
				Name:        flags.PrivateKeyPath,
				EnvVars:     []string{"SSH_KEY_PATH"},
				Destination: &container.SshKeyPath,
				Usage:       "SSH key path (loaded from Bitwarden as SSH_KEY_{ENV})",
			},
			&cli.StringFlag{
				Name:        flags.User,
				Value:       configs.DEFAULT_USER_NAME,
				Destination: &container.DefaultClusterUserName,
				Usage:       "SSH user on servers",
			},
			&cli.StringFlag{
				Name:        flags.Password,
				EnvVars:     []string{"SERVER_PASSWORD"},
				Destination: &container.UserPassword,
				Usage:       "Server sudo password (loaded from Bitwarden as SERVER_PASSWORD_{ENV})",
			},
			&cli.StringFlag{
				Name:    flags.GithubToken,
				EnvVars: []string{"GITHUB_TOKEN"},
				Usage:   "GitHub token (loaded from Bitwarden)",
			},
			&cli.StringFlag{
				Name:    flags.NginxApiKey,
				EnvVars: []string{"NGINX_API_KEY"},
				Usage:   "Nginx API key (loaded from Bitwarden)",
			},
			&cli.StringFlag{
				Name:  "chdir",
				Usage: "Change working directory before executing",
			},
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "Suppress verbose output",
			},
		},
		Action: func(ctx *cli.Context) error {
			if ctx.Args().Len() == 0 {
				cli.ShowAppHelp(ctx)
				return nil
			}
			return fmt.Errorf("unknown command %s, check --help for more info about available commands", ctx.Args().First())
		},
		Version: version.Get(),
		Before: func(ctx *cli.Context) error {
			arg := ctx.Args().First()
			if arg != "help" && arg != "" && !ctx.Bool("help") && !ctx.Bool("version") {
				container.LoadSecretsFromBitwarden()
			}

			// Cached ChainJson information
			chainJsonData, err := container.LoadChainJson()
			if err != nil {
				return fmt.Errorf("loading chain json information: %w", err)
			}

			container.ConfigRWriter = configs.NewInMemoryD8XConfigRW(nil)

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


			return nil
		},
		After: func(ctx *cli.Context) error {
			dir := filepath.Join(os.TempDir(), "d8x-cli")
			if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
				fmt.Printf("warning: failed to clean up temp dir %s: %s\n", dir, err)
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
