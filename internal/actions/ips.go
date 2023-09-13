package actions

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

func (c *Container) Ips(ctx *cli.Context) error {

	switch ctx.Args().First() {
	case "manager":
		ip, err := c.HostsCfg.GetMangerPublicIp()
		if err != nil {
			return err
		}
		fmt.Printf("Manager node public IP address: %s\n", ip)
	case "broker":
		ip, err := c.HostsCfg.GetBrokerPublicIp()
		if err != nil {
			return err
		}
		fmt.Printf("Broker node public IP address: %s\n", ip)
	default:
		return fmt.Errorf("Unknown argument: %s. Supported values: manager, broker", ctx.Args().First())
	}

	return nil
}
