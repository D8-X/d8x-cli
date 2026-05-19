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
		Metadata:             map[string]any{"container": container},
		CommandNotFound: func(ctx *cli.Context, s string) {
			fmt.Printf("Unknown command %s\n", s)
		},
		Commands: []*cli.Command{

			{
				Name:   "init",
				Action: container.Init,
				Usage:  "Check for required dependencies (terraform, ansible) and offer to install missing ones on Linux",
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
						"rm-env",
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
						Name:   "new-env",
						Usage:  "Create a new environment in the infra repo",
						Action: withNextStep("new-env", container.NewEnvironment),
					},
					{
						Name:   "rm-env",
						Usage:  "Remove a non-provisioned environment from the infra repo",
						Action: container.RemoveEnvironment,
					},
					{
						Name:        "provision",
						Aliases:     []string{"prov"},
						Usage:       "Provision server resources with terraform",
						Action:      withNextStep("provision", container.Provision),
						Description: ProvisionDescription,
					},
					{
						Name:        "configure",
						Aliases:     []string{"config"},
						Usage:       "Configure servers with ansible",
						Action:      withNextStep("configure", container.Configure),
						Description: ConfigureDescription,
					},
					{
						Name:   "broker-deploy",
						Usage:  "Deploy and configure broker-server deployment",
						Action: withNextStep("broker-deploy", container.BrokerDeploy),
					},
					{
						Name:   "broker-nginx",
						Usage:  "Configure and setup nginx + certbot for broker server deployment",
						Action: withNextStep("broker-nginx", container.BrokerServerNginxCertbotSetup),
					},
					{
						Name:        "swarm-deploy",
						Aliases:     []string{"sd"},
						Usage:       "Deploy and configure d8x-trader-backend swarm cluster",
						Action:      withNextStep("swarm-deploy", container.SwarmDeploy),
						Description: SwarmDeployDescription,
					},
					{
						Name:        "swarm-nginx",
						Aliases:     []string{"sn"},
						Usage:       "Configure and setup nginx + certbot for d8x-trader swarm deployment",
						Action:      withNextStep("swarm-nginx", container.SwarmNginx),
						Description: SwarmNginxDescription,
					},
					{
						Name:        "metrics-deploy",
						Usage:       "Deploy and configure metrics services (prometheus, grafana) on manager node",
						Action:      withNextStep("metrics-deploy", container.DeployMetrics),
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
			if !isHelpOrVersionInvocation(os.Args) {
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

var setupSequence = []struct {
	name string
	desc string
}{
	{"new-env", "Create the env directory in the infra repo (config + nginx + tfvars)"},
	{"provision", "Run terraform to create the cloud servers (manager, workers, broker)"},
	{"configure", "Run ansible to install docker, swarm, users, ssh keys on the servers"},
	{"swarm-deploy", "Deploy the trader-backend swarm stack (api, history, redis, ...)"},
	{"swarm-nginx", "Set up nginx plus certbot SSL in front of the swarm services"},
	{"broker-deploy", "Deploy the broker server (signs orders) on its host"},
	{"broker-nginx", "Set up nginx plus certbot SSL in front of the broker server"},
	{"metrics-deploy", "Deploy prometheus and grafana on the manager node (optional)"},
}

var valueTakingFlags = map[string]struct{}{
	"--password": {}, "-password": {},
	"--user": {}, "-user": {},
	"--github-token": {}, "-github-token": {},
	"--nginx-api-key": {}, "-nginx-api-key": {},
	"--chdir": {}, "-chdir": {},
}

func isHelpOrVersionInvocation(args []string) bool {
	if len(args) <= 1 {
		return true
	}
	skipNext := false
	for _, a := range args[1:] {
		if skipNext {
			skipNext = false
			continue
		}
		if _, ok := valueTakingFlags[a]; ok {
			skipNext = true
			continue
		}
		switch a {
		case "help", "h", "--help", "-h", "--version", "-v":
			return true
		}
	}
	return false
}

var stepActions = map[string]cli.ActionFunc{}

func withNextStep(name string, action cli.ActionFunc) cli.ActionFunc {
	wrapped := func(ctx *cli.Context) error {
		printStepBanner(name)
		if ctx.App.Metadata == nil {
			ctx.App.Metadata = map[string]any{}
		}
		prev := ctx.App.Metadata["activeStep"]
		ctx.App.Metadata["activeStep"] = name
		err := action(ctx)
		ctx.App.Metadata["activeStep"] = prev
		if err != nil {
			return err
		}
		return promptAndDispatchNextStep(ctx, name)
	}
	stepActions[name] = wrapped
	return wrapped
}

func printStepBanner(name string) {
	idx, ok := stepIndex(name)
	if !ok {
		return
	}
	s := setupSequence[idx]
	banner := styles.PurpleBgText.Copy().Padding(0, 2).Render(
		fmt.Sprintf(" STEP %d/%d: %s ", idx+1, len(setupSequence), s.name),
	)
	fmt.Println()
	fmt.Println(banner)
	fmt.Println(styles.ItalicText.Render(s.desc))
	fmt.Println()
}

func promptAndDispatchNextStep(ctx *cli.Context, current string) error {
	container, ok := ctx.App.Metadata["container"].(*actions.Container)
	idx, found := stepIndex(current)
	if !found || idx+1 >= len(setupSequence) {
		return nil
	}
	next := setupSequence[idx+1]
	nextAction := stepActions[next.name]
	if nextAction == nil || !ok || container == nil {
		fmt.Println()
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf("Next: \"d8x setup %s\" (%s)", next.name, next.desc)))
		return nil
	}
	question := fmt.Sprintf("Continue to STEP %d/%d: %s?", idx+2, len(setupSequence), next.name)
	proceed, err := container.TUI.NewPrompt(question, true)
	if err != nil {
		return err
	}
	if !proceed {
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf("Stopped before \"d8x setup %s\". Run it later to continue.", next.name)))
		return nil
	}
	return nextAction(ctx)
}

func stepIndex(name string) (int, bool) {
	for i, s := range setupSequence {
		if s.name == name {
			return i, true
		}
	}
	return 0, false
}
