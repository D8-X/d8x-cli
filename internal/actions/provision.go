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
	selected, err := c.TUI.NewSelection([]string{
		string(ServerProviderLinode),
		string(ServerProviderAws),
	},
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)

	if err != nil {
		return err
	}

	if len(selected) <= 0 {
		return fmt.Errorf("at least one server provider must be selected")
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

	// Terraform init must run after we copy all the terraform files via
	// BuildTerraformCMD
	tfInit := exec.Command("terraform", "init")
	connectCMDToCurrentTerm(tfInit)
	if err := tfInit.Run(); err != nil {
		return err
	}

	if tfCmd != nil {
		connectCMDToCurrentTerm(tfCmd)
		err := tfCmd.Run()
		if err != nil {
			fmt.Println(styles.ErrorText.Render("Terraform apply failed, please check the output above for more details.\nPossible issues:\n\tDuplicate server label\n\tIncorrect server provider credentials\n\tSelected region was used first time"))
			return err
		}
	}

	// Set the provisioning time
	c.provisioningTime = time.Now()

	// Perform provider dependent actions
	if err := providerConfigurer.PostProvisioningAction(c); err != nil {
		return err
	}

	return nil
}

// ServerProviderConfigurer
type ServerProviderConfigurer interface {
	//  BuildTerraformCMD generates neccessary files and configs to start
	// terraform provisioning. Returned exec.Cmd can be used to execute
	// terraform apply
	BuildTerraformCMD(*Container) (*exec.Cmd, error)

	// PostProvisioningAction is called once BuildTerraformCMD Cmd is executed
	// successfuly. This method is used to perform provider specific actions
	// after the provisioning.
	PostProvisioningAction(*Container) error
}

// configureServerProviderForTF collects details about user specified provider
// and returns a server configurer
func (c *Container) configureServerProviderForTF(provider SupportedServerProvider) (ServerProviderConfigurer, error) {
	switch provider {
	case ServerProviderLinode:
		return c.createLinodeServerConfigurer()
	case ServerProviderAws:
		return c.createAWSServerConfigurer()
	}

	return nil, nil
}
