package actions

import (
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) ServiceUpdate(ctx *cli.Context) error {
	styles.ItalicText.Render("Updating swarm services...")

	// To update broker-server simply rerun the broker-deploy

	// ---

	// To update swarm services-  prompt user select which service to update

	// Pull few latest tags from ghcr for requested service
	// Docker registry SDK
	// Display a list of tags with sha hashes that are availble for user to select from

	// Once specific image is selected - update the service on manager

	// docker service update --image "ghcr.io/d8-x/d8x-trader-main:dev@sha256:aea8e56d6077c733a1d553b4291149712c022b8bd72571d2a852a5478e1ec559" stack_api

	return nil
}
