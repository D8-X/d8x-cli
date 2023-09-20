package actions

import (
	"fmt"
	"os/exec"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) TerraformDestroy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Running terraform destroy...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	fmt.Printf("Using provider from config: %s\n", cfg.ServerProvider)

	var args []string = []string{
		"destroy", "-auto-approve",
	}

	// Provide necessary variables for terraform destroy based on the provider
	switch cfg.ServerProvider {
	case configs.D8XServerProviderAWS:
		a := cfg.AWSConfig
		if a == nil {
			return fmt.Errorf("aws config is not defined")
		}

		authorizedKey, err := c.getPublicKey()
		if err != nil {
			return err
		}

		args = append(args,
			"-var", fmt.Sprintf(`aws_access_key=%s`, a.AccesKey),
			"-var", fmt.Sprintf(`aws_secret_key=%s`, a.SecretKey),
			"-var", fmt.Sprintf(`region=%s`, a.Region),
			"-var", fmt.Sprintf(`authorized_key=%s`, authorizedKey),
		)

	case configs.D8XServerProviderLinode:
		// TODO
	}

	cmd := exec.Command("terraform", args...)
	connectCMDToCurrentTerm(cmd)
	return c.RunCmd(cmd)
}
