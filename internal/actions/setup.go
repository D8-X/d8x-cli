package actions

import (
	"github.com/urfave/cli/v2"
)

func (ac *Container) Setup(ctx *cli.Context) error {
	if err := ac.Provision(ctx); err != nil {
		return err
	}

	return nil
}
