package actions

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
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

	// Load config for storing server provider details
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Cp inventory.tpl for hosts.cfg
	if err := c.EmbedCopier.CopyMultiToDest(
		configs.EmbededConfigs,
		"./inventory.tpl",
		"embedded/trader-backend/inventory.tpl",
	); err != nil {
		return fmt.Errorf("generating inventory.tpl file: %w", err)
	}

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

	// Perform provider dependent actions
	switch i := providerConfigurer.(type) {
	case linodeConfigurer:
		// Pull the cert
		if err := i.pullPgCert(c.HttpClient, c.PgCrtPath); err != nil {
			return err
		}

		// Write linode config to cfg
		cfg.ServerProvider = configs.D8XServerProviderLinode
		cfg.LinodeConfig = &configs.D8XLinodeConfig{
			Token:       i.linodeToken,
			DbId:        i.linodeDbId,
			Region:      i.linodeRegion,
			LabelPrefix: i.linodeNodesLabelPrefix,
		}

		if err := c.ConfigRWriter.Write(cfg); err != nil {
			return err
		}
	}

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
	case ServerProviderAws:
		return c.awsServerConfigurer()
	}

	return nil, nil
}
