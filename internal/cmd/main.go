package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/D8-X/d8x-cli/internal/actions"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/D8-X/d8x-cli/internal/version"
	"github.com/charmbracelet/lipgloss"
	"github.com/urfave/cli/v2"
)

const D8XASCII = ` ____     ___   __  __
|  _ \   ( _ )  \ \/ /
| | | |  / _ \   \  / 
| |_| | | (_) |  /  \ 
|____/   \___/  /_/\_\
`

// CmdName defines the name of cli tool
const CmdName = "d8x"

const CmdUsage = "D8X Backend management CLI tool"

// MainDescription is the description text for d8x cli tool
const MainDescription = `D8X Perpetual Exchange broker backend setup and management CLI tool 

<More description entered here>

Running d8x without any subcommands or init command will perform initalization
of ~/.config/d8x directory, as well as prompt you to install any missing
dependencies.

D8X CLI relies on the following external tools: terraform, ansible. You can
manually install them or let the cli attempt to perform the installation of
these dependencies.
`

// RunD8XCli is the entrypoint to D8X cli tool
func RunD8XCli() {
	ac := &actions.Container{}

	// Initialize cli application and its subcommands and bind default values
	// for ac (via flags.Destination)
	app := &cli.App{
		Name:        CmdName,
		HelpName:    CmdName,
		Usage:       CmdUsage,
		Description: MainDescription,
		Commands: []*cli.Command{
			{
				Name:   "init",
				Action: ac.Init,
				Usage:  "Initialize dependencies and configuration directory",
			},
			{
				Name:        "setup",
				Description: "Full D8X backend cluster setup",
				Action:      ac.Setup,
				Usage:       "Spin up  your clusters and services",
			},
		},
		// Global flags accesible to all subcommands
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "config-directory",
				// Set the defaul path to configuration directory on user's home dir
				Value:       "~/.config/d8x",
				Destination: &ac.ConfigDir,
			},
		},
		Action:  ac.Init,
		Version: version.Get(),
		Before: func(ctx *cli.Context) error {
			fmt.Println(styles.PurpleBgText.Padding(0, 2, 0, 2).Border(lipgloss.NormalBorder()).Render(D8XASCII))
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
