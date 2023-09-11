package actions

import (
	"fmt"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/urfave/cli/v2"
)

func (c *Container) Ips(ctx *cli.Context) error {
	hostsCfg, err := c.LoadHostsFile(configs.DEFAULT_HOSTS_FILE)
	if err != nil {
		return err
	}

	switch ctx.Args().First() {
	case "manager":
		ip, err := hostsCfg.GetMangerPublicIp()
		if err != nil {
			return err
		}
		fmt.Printf("Manager node public IP address: %s\n", ip)
	case "broker":
		ip, err := hostsCfg.GetBrokerPublicIp()
		if err != nil {
			return err
		}
		fmt.Printf("Broker node public IP address: %s\n", ip)
	default:
		return fmt.Errorf("Unknown argument: %s. Supported values: manager, broker", ctx.Args().First())
	}

	return nil
}
