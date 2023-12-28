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

	ok, err := c.TUI.NewPrompt("Are you sure you want to run terraform destroy? This will destroy all the resources created by d8x-cli. This action is irreversible!", false)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Not destroying...")
		return nil
	}

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
		authorizedKey, err := getPublicKey(c.SshKeyPath)
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
	cmd.Dir = TF_FILES_DIR

	connectCMDToCurrentTerm(cmd)
	if err := c.RunCmd(cmd); err != nil {
		return err
	}

	// Update d8x config values and set deployment statuses to false
	cfg.ResetDeploymentStatus()

	return c.ConfigRWriter.Write(cfg)
}
