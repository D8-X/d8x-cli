package actions

import (
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

	// Prompt to clean up config when it exists
	if !cfg.IsEmpty() {
		if ok, err := c.TUI.NewPrompt("Existing configuration was found. Do you want to remove it? (Recommended for only fresh start)", false); ok {
			if err != nil {
				return err
			}
			if err := c.ConfigRWriter.Write(&configs.D8XConfig{}); err != nil {
				return err
			}
		}
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
		if ok, _ := c.TUI.NewPrompt("Configuration failed, do you want to continue?", false); !ok {
			return err
		}
	}

	if c.CreateBrokerServer {
		if err := c.BrokerDeploy(ctx); err != nil {
			return err
		}

		if err := c.BrokerServerNginxCertbotSetup(ctx); err != nil {
			return err
		}
	}

	if err := c.SwarmDeploy(ctx); err != nil {
		return err
	}

	if err := c.SwarmNginx(ctx); err != nil {
		return err
	}

	if ok, _ := c.TUI.NewPrompt("Do you want to perform services healthchecks?", true); ok {
		if err := c.HealthCheck(ctx); err != nil {
			return err
		}
	}

	return nil
}
