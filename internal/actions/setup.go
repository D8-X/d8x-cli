package actions

import (
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (ac *Container) Setup(ctx *cli.Context) error {
	styles.PrintCommandTitle("Running full setup...")

	// Clean up the config
	if ok, err := components.NewPrompt("Do you want to start clean and flush all configs (recommended for first time setup)?", true); ok {
		if err != nil {
			return err
		}
		if err := ac.ConfigRWriter.Write(&configs.D8XConfig{}); err != nil {
			return err
		}
	}

	if err := ac.Provision(ctx); err != nil {
		return err
	}

	// Cooldown for 2 minutes before starting configuration
	t := ac.provisioningTime.Add(2 * time.Minute)
	if time.Now().Before(t) {
		waitFor := t.Sub(time.Now())
		components.NewTimer(waitFor, "Waiting for SSHDs to start on nodes")
	}

	if err := ac.Configure(ctx); err != nil {
		return err
	}

	if err := ac.BrokerServerDeployment(ctx); err != nil {
		return err
	}

	if err := ac.BrokerServerNginxCertbotSetup(ctx); err != nil {
		return err
	}

	if err := ac.SwarmDeploy(ctx); err != nil {
		return err
	}

	if err := ac.SwarmNginx(ctx); err != nil {
		return err
	}

	if ok, _ := components.NewPrompt("Do you want to perform services healthchecks?", true); ok {
		if err := ac.HealthCheck(ctx); err != nil {
			return err
		}
	}

	return nil
}
