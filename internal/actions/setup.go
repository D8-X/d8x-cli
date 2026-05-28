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
			fmt.Sprintf("Existing configuration (%s) was found. Do you want to use it?", cfg.ServerProvider),
			true,
		)

		if err != nil {
			return err
		}
		if !keepConfig {
			// Print out a warning one more time to prevent accidental deletion
			// of config
			fmt.Println(
				styles.AlertImportant.Render("Warning! Existing configuration will be completely removed!"),
			)
			if yes, err := c.TUI.NewPrompt("Are you sure you want to continue?", false); err != nil {
				return err
			} else if yes {
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
	if c.Input.setup.deployMetrics {
		total++
	}
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
			if ok, _ := c.TUI.NewPrompt("Configuration failed, do you want to continue?", false); !ok {
				return err
			}
		}
	}

	// Deploy metrics stack if user wants to
	if c.Input.setup.deployMetrics {
		announce("metrics-deploy", "Deploy prometheus and grafana on the manager node")
		if err := c.DeployMetrics(ctx); err != nil {
			return err
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

	if ok, _ := c.TUI.NewPrompt("Do you want to perform services healthchecks?", true); ok {
		if err := c.HealthCheck(ctx); err != nil {
			return err
		}
	}

	return nil
}
