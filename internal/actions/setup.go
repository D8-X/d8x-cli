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
				// Make a backup of the existing config just in case
				backup := c.ConfigRWriter.GetPath() + ".backup-" + time.Now().Format("2006-01-02_15:04:05")
				if err := c.ConfigRWriter.WriteTo(backup, cfg); err != nil {
					return err
				}
				fmt.Printf("Backup of the existing configuration was saved to %s\n\n", backup)

				if err := c.ConfigRWriter.Write(&configs.D8XConfig{}); err != nil {
					return err
				}
			}
		}
	}

	// Collect all data needed for setup
	if err := c.Input.CollectFullSetupInput(ctx); err != nil {
		return err
	}

	if err := c.Provision(ctx); err != nil {
		return err
	}

	// Cooldown for 2 minutes before starting configuration
	t := c.provisioningTime.Add(2 * time.Minute)
	if time.Now().Before(t) {

		waitFor := time.Until(t)
		c.TUI.NewTimer(waitFor, "Waiting for SSHDs to start on nodes")
	}

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
		if err := c.DeployMetrics(ctx); err != nil {
			return err
		}
	}

	if c.Input.setup.deployBroker {
		if err := c.BrokerDeploy(ctx); err != nil {
			return err
		}

		if c.Input.runBrokerNginxCertbot {
			if err := c.BrokerServerNginxCertbotSetup(ctx); err != nil {
				return err
			}
		}
	}

	if c.Input.setup.deploySwarm {
		if err := c.SwarmDeploy(ctx); err != nil {
			return err
		}

		if c.Input.runSwarmNginxCertbot {
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
