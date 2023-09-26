package actions

import (
	"fmt"
	"os"
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

	var env []string = []string{}

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
		awsConfigurer := &awsConfigurer{D8XAWSConfig: *a, authorizedKey: authorizedKey}
		args = append(args, awsConfigurer.generateVariables()...)

	case configs.D8XServerProviderLinode:
		args = append(args, "-var", `authorized_keys=[""]`)
		env = append(env, fmt.Sprintf("LINODE_TOKEN=%s", cfg.LinodeConfig.Token))
	}

	cmd := exec.Command("terraform", args...)
	cmd.Env = append(os.Environ(), env...)

	connectCMDToCurrentTerm(cmd)
	return c.RunCmd(cmd)
}
