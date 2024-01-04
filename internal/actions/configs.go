package actions

import (
	"fmt"

	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) CopyConfigs(ctx *cli.Context) error {

	allArgs := ctx.Args()

	for _, arg := range allArgs.Slice() {
		switch arg {
		case "manager":
			if err := c.CopySwarmDeployConfigs(); err != nil {
				return fmt.Errorf("failed to copy swarm configs: %w", err)
			} else {
				fmt.Println(styles.SuccessText.Render("Successfully copied swarm configs"))
			}
		case "broker":
			if err := c.CopyBrokerDeployConfigs(); err != nil {
				return fmt.Errorf("failed to copy broker configs: %w", err)
			} else {
				fmt.Println(styles.SuccessText.Render("Successfully copied broker configs"))
			}
		default:
			return fmt.Errorf("unknown argument: %s", arg)
		}
	}

	return nil
}
