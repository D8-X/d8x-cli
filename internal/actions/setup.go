package actions

import (
	"fmt"
	"time"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) Setup(ctx *cli.Context) error {
	styles.PrintCommandTitle("Running full setup...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Ignore init errors, since we might encounter them on mac
	if err := c.Init(ctx); err != nil {
		fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Init error: %v", err)))
	}

	env, err := c.EnsureEnvironment(cfg)
	if err != nil {
		return err
	}
	if err := c.ensureSSHKey(env); err != nil {
		return err
	}

	// Prompt to clean up config when it exists
	if !cfg.IsEmpty() {
		keepConfig, err := c.TUI.NewPrompt(
			yesNoQuestion(
				fmt.Sprintf("Existing configuration (%s) was found. Reuse it?", cfg.ServerProvider),
				"keeps the stored provider, server sizes and secrets, and asks only for values that are missing",
				"offers to clear the loaded configuration so every value is collected again from scratch",
			),
			true,
		)

		if err != nil {
			return err
		}
		if !keepConfig {
			// Print out a warning one more time to prevent accidental deletion
			// of config
			fmt.Println(
				styles.AlertImportant.Render("Warning! This discards every value the cli has loaded for this environment."),
			)
			clearCfg, err := c.TUI.NewPrompt(
				yesNoQuestion(
					"Clear the loaded configuration now?",
					"empties the in memory config; provider, server sizes and secrets are re-read from the infra repo and Bitwarden on the next env selection",
					"keeps the existing configuration and continues with it",
				),
				false,
			)
			if err != nil {
				return err
			}
			if clearCfg {
				if err := c.ConfigRWriter.Write(&configs.D8XConfig{}); err != nil {
					return err
				}
				fmt.Println(styles.ItalicText.Render("In-memory config cleared. Truthful state will be re-loaded from infra repo + Bitwarden on the next env selection."))
			}
		}
	}

	// Collect all data needed for setup
	if err := c.Input.CollectFullSetupInput(ctx); err != nil {
		return err
	}

	total := 2
	if c.Input.setup.deployBroker {
		total++
		if c.Input.runBrokerNginxCertbot {
			total++
		}
	}
	if c.Input.setup.deploySwarm {
		total++
		if c.Input.runSwarmNginxCertbot {
			total++
		}
	}
	step := 0
	announce := func(name, desc string) {
		step++
		banner := styles.PurpleBgText.Copy().Padding(0, 2).Render(
			fmt.Sprintf(" STEP %d/%d: %s ", step, total, name),
		)
		fmt.Println()
		fmt.Println(banner)
		if desc != "" {
			fmt.Println(styles.ItalicText.Render(desc))
		}
		fmt.Println()
	}

	announce("provision", "Run terraform to create the cloud servers (manager, workers, broker)")
	if err := c.Provision(ctx); err != nil {
		return err
	}

	// Cooldown for 2 minutes before starting configuration
	t := c.provisioningTime.Add(2 * time.Minute)
	if time.Now().Before(t) {

		waitFor := time.Until(t)
		c.TUI.NewTimer(waitFor, "Waiting for SSHDs to start on nodes")
	}

	announce("configure", "Run ansible to install docker, swarm, users, ssh keys on the servers")
	// If configuration fails we might still want to proceed with other actions
	// in case this is a retry
	if err := c.Configure(ctx); err != nil {
		// On linode: when subsequent setup runs are performed, old servers will
		// not be accessible because of permit root login is set to false and we
		// can't provide dynamic user list to ansible.
		if cfg.ServerProvider == configs.D8XServerProviderLinode && (cfg.SwarmDeployed || cfg.BrokerDeployed) {
			fmt.Println("Some configuration steps failed, but we will continue with other actions...")
		} else {
			cont, perr := c.TUI.NewPrompt(
				yesNoQuestion(
					"The \"configure\" step failed. Continue with the remaining setup steps?",
					"runs broker deploy, swarm deploy and nginx anyway, which may fail again or leave the cluster half configured",
					"stops here and returns the configure error so you can fix it and rerun \"d8x setup\"",
				),
				false,
			)
			if perr != nil {
				return perr
			}
			if !cont {
				return err
			}
		}
	}

	if c.Input.setup.deployBroker {
		announce("broker-deploy", "Deploy the broker server (signs orders) on its host")
		if err := c.BrokerDeploy(ctx); err != nil {
			return err
		}

		if c.Input.runBrokerNginxCertbot {
			announce("broker-nginx", "Set up nginx and certbot SSL in front of the broker server")
			if err := c.BrokerServerNginxCertbotSetup(ctx); err != nil {
				return err
			}
		}
	}

	if c.Input.setup.deploySwarm {
		announce("swarm-deploy", "Deploy the trader-backend swarm stack (api, history, redis, ...)")
		if err := c.SwarmDeploy(ctx); err != nil {
			return err
		}

		if c.Input.runSwarmNginxCertbot {
			announce("swarm-nginx", "Set up nginx and certbot SSL in front of the swarm services")
			if err := c.SwarmNginx(ctx); err != nil {
				return err
			}
		}
	}

	runHealth, err := c.TUI.NewPrompt(
		yesNoQuestion(
			"Run health checks against the deployed services now?",
			"probes the api, history, candles and broker endpoints and prints which ones answered",
			"finishes setup without probing; you can run \"d8x health\" at any time later",
		),
		true,
	)
	if err != nil {
		return err
	}
	if runHealth {
		if err := c.HealthCheck(ctx); err != nil {
			return err
		}
	}

	return nil
}
