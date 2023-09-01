package actions

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

func (ac *Container) Setup(ctx *cli.Context) error {
	fmt.Println("Doin some setup yah!")
	return nil
}
