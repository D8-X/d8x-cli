package actions

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

type SupportedServerProvider string

const (
	ServerProviderLinode SupportedServerProvider = "linode"
	ServerProviderAws    SupportedServerProvider = "aws"
)

func (c *Container) Provision(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting provisioning...")

	fmt.Println("Select your server provider")

	// List of supported server providers
	selected, err := components.NewSelection([]string{
		string(ServerProviderLinode),
		// string(ServerProviderAws),
	},
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)

	if err != nil {
		return err
	}

	if len(selected) <= 0 {
		return fmt.Errorf("At least one server provider must be selected")
	}

	providerConfigurer, err := c.configureServerProviderForTF(SupportedServerProvider(selected[0]))
	if err != nil {
		return fmt.Errorf("collecting server provider details: %w", err)
	}

	// Terraform apply for selected server provider
	tfCmd, err := providerConfigurer.BuildTerraformCMD(c)
	if err != nil {
		return err
	}

	// Exec terraform init
	tfInit := exec.Command("terraform", "init")
	connectCMDToCurrentTerm(tfInit)
	if err := tfInit.Run(); err != nil {
		return err
	}

	if tfCmd != nil {
		connectCMDToCurrentTerm(tfCmd)
		err := tfCmd.Run()
		if err != nil {
			return err
		}
	}

	// Set the provisioning time
	c.provisioningTime = time.Now()

	return nil
}

// ServerProviderConfigurer generates neccessary files and configs to start
// terraform provisioning. Returned exec.Cmd can be used to execute terraform
// apply
type ServerProviderConfigurer interface {
	BuildTerraformCMD(*Container) (*exec.Cmd, error)
}

// configureServerProviderForTF collects details about user specified provider
// and returns a server configurer
func (c *Container) configureServerProviderForTF(provider SupportedServerProvider) (ServerProviderConfigurer, error) {
	switch provider {
	case ServerProviderLinode:
		return c.linodeServerConfigurer()
	}

	return nil, nil
}
