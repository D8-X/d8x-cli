package actions

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

func (c *Container) Ips(ctx *cli.Context) error {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureEnvironment(cfg); err != nil {
		return err
	}

	onlyIp := ctx.Bool("quiet")
	target := ctx.Args().First()
	if target == "" {
		managerIp, mErr := c.HostsCfg.GetMangerPublicIp()
		brokerIp, bErr := c.HostsCfg.GetBrokerPublicIp()
		if mErr != nil && bErr != nil {
			return fmt.Errorf("no manager or broker IP available; has this env been provisioned?")
		}
		if mErr == nil {
			fmt.Printf("Manager node public IP address: %s\n", managerIp)
		}
		if bErr == nil {
			fmt.Printf("Broker node public IP address:  %s\n", brokerIp)
		}
		return nil
	}
	switch target {
	case "manager":
		ip, err := c.HostsCfg.GetMangerPublicIp()
		if err != nil {
			return err
		}
		if onlyIp {
			fmt.Println(ip)
			return nil
		}
		fmt.Printf("Manager node public IP address: %s\n", ip)
	case "broker":
		ip, err := c.HostsCfg.GetBrokerPublicIp()
		if err != nil {
			return err
		}
		if onlyIp {
			fmt.Println(ip)
			return nil
		}
		fmt.Printf("Broker node public IP address: %s\n", ip)
	default:
		return fmt.Errorf("unknown argument %q; supported values: manager, broker (or omit for both)", target)
	}

	return nil
}
