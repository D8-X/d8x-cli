package actions

import (
	"fmt"
	"os/exec"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
)

type awsConfigurer struct {
	configs.D8XAWSConfig

	authorizedKey string
}

func (a *awsConfigurer) BuildTerraformCMD(c *Container) (*exec.Cmd, error) {
	if err := c.EmbedCopier.CopyMultiToDest(
		configs.EmbededConfigs,
		// Dest
		"./aws.tf",
		// Embed paths must be in this order: main.tf vars.tf
		"embedded/trader-backend/tf-aws/main.tf",
		"embedded/trader-backend/tf-aws/routes.tf",
		"embedded/trader-backend/tf-aws/sg.tf",
		"embedded/trader-backend/tf-aws/vars.tf",
		"embedded/trader-backend/tf-aws/output.tf",
	); err != nil {
		return nil, fmt.Errorf("generating aws.tf file: %w", err)
	}

	cmd := exec.Command("terraform",
		append([]string{"apply", "-auto-approve"}, a.generateVariables()...)...,
	)

	return cmd, nil
}

// generateVariables generates terraform variables for aws provider
func (a *awsConfigurer) generateVariables() []string {
	return []string{
		"-var", fmt.Sprintf(`server_label_prefix=%s`, a.LabelPrefix),
		"-var", fmt.Sprintf(`aws_access_key=%s`, a.AccesKey),
		"-var", fmt.Sprintf(`aws_secret_key=%s`, a.SecretKey),
		"-var", fmt.Sprintf(`region=%s`, a.Region),
		// Do not include the quotes here
		"-var", fmt.Sprintf(`authorized_key=%s`, a.authorizedKey),
		"-var", fmt.Sprintf("db_instance_class=%s", a.RDSInstanceClass),
	}
}

// awsServerConfigurer creates new awsConfigurer
func (c *Container) awsServerConfigurer() (ServerProviderConfigurer, error) {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return nil, err
	}

	awsCfg := &awsConfigurer{}
	// Text field values
	awsKey := ""
	awsSecret := ""
	awsRDSInstanceClass := "db.t4g.small"
	if cfg.AWSConfig != nil {
		awsKey = cfg.AWSConfig.AccesKey
		awsSecret = cfg.AWSConfig.SecretKey
		if cfg.AWSConfig.RDSInstanceClass != "" {
			awsRDSInstanceClass = cfg.AWSConfig.RDSInstanceClass
		}
	}

	fmt.Println("Enter your AWS Access Key: ")
	accessKey, err := c.TUI.NewInput(
		components.TextInputOptValue(awsKey),
		components.TextInputOptPlaceholder("<AWS_ACCESS_KEY>"),
	)
	if err != nil {
		return nil, err
	}
	awsCfg.AccesKey = accessKey

	fmt.Println("Enter your AWS Secret Key: ")
	secretKey, err := c.TUI.NewInput(
		components.TextInputOptValue(awsSecret),
		components.TextInputOptMasked(),
		components.TextInputOptPlaceholder("<AWS_SECRET_KEY>"),
	)
	if err != nil {
		return nil, err
	}
	awsCfg.SecretKey = secretKey

	fmt.Println("Enter your AWS cluster region: ")
	region, err := c.TUI.NewInput(
		components.TextInputOptValue("eu-central-1"),
		components.TextInputOptPlaceholder("us-west-1"),
	)
	if err != nil {
		return nil, err
	}
	awsCfg.Region = region

	fmt.Println("Enter your AWS RDS DB instance class: ")
	dbClass, err := c.TUI.NewInput(
		components.TextInputOptValue(awsRDSInstanceClass),
		components.TextInputOptPlaceholder("db.t3.medium"),
	)
	if err != nil {
		return nil, err
	}
	awsCfg.RDSInstanceClass = dbClass

	fmt.Println("Enter server tag prefix: ")
	labelPrefix, err := c.TUI.NewInput(
		components.TextInputOptValue("d8x-cluster"),
		components.TextInputOptPlaceholder("my-cluster"),
	)
	if err != nil {
		return nil, err
	}
	awsCfg.LabelPrefix = labelPrefix

	// SSH key check
	if err := c.ensureSSHKeyPresent(); err != nil {
		return nil, err
	}
	pub, err := c.getPublicKey()
	if err != nil {
		return nil, err
	}
	awsCfg.authorizedKey = pub

	// Store aws details in configuration file
	cfg.ServerProvider = configs.D8XServerProviderAWS
	cfg.AWSConfig = &awsCfg.D8XAWSConfig
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return nil, err
	}

	return awsCfg, nil
}
