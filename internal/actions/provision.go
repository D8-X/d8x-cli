package actions

import (
	"fmt"
	"os/exec"
	"time"

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

	if err := c.Input.CollectProvisioningData(ctx); err != nil {
		return err
	}
	providerConfigurer := c.Input.GetServerProviderConfigurer()
	if providerConfigurer == nil {
		return fmt.Errorf("misconfigured server provider details")
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

	// Update the input
	if err := c.Input.PostProvisioningHook(); err != nil {
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
