package actions

import (
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/urfave/cli/v2"
)

func (ac *Container) Setup(ctx *cli.Context) error {
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

	return nil
}
