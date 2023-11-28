package actions

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/jackc/pgx/v5"
)

// Path to postgres rds credentials file
const RDS_CREDS_FILE = "aws_rds_postgres.txt"

// Default sudo user name for ubuntu AMIs (before configuration is done)
const AWS_DEFAULT_INITIAL_USER = "ubuntu"

var _ ServerProviderConfigurer = (*awsConfigurer)(nil)

type awsConfigurer struct {
	configs.D8XAWSConfig

	authorizedKey string
}

func (a *awsConfigurer) BuildTerraformCMD(c *Container) (*exec.Cmd, error) {
	if err := c.EmbedCopier.CopyMultiToDest(
		configs.EmbededConfigs,
		"./aws.tf",
		"embedded/trader-backend/tf-aws/main.tf",
		"embedded/trader-backend/tf-aws/routes.tf",
		"embedded/trader-backend/tf-aws/sg.tf",
		"embedded/trader-backend/tf-aws/pg.tf",
		"embedded/trader-backend/tf-aws/vars.tf",
		"embedded/trader-backend/tf-aws/output.tf",
	); err != nil {
		return nil, fmt.Errorf("generating aws.tf file: %w", err)
	}

	return a.generateTerraformCommand(), nil
}

func (a *awsConfigurer) PostProvisioningAction(c *Container) error {
	// Attempt to update known_hosts with manager's host key
	managerIp, _ := c.HostsCfg.GetMangerPublicIp()
	if err := a.putManagerToKnownHosts(managerIp); err != nil {
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("could not update ~/.ssh/known_hosts with manager ip address: %v", err),
			),
		)
	}

	return nil
}

// putManagerToKnownHosts attempts to put managerIpAddress to current user's
// known_hosts file, so that user does not need to manually accept the host when
// ansible configuration runs.
func (a *awsConfigurer) putManagerToKnownHosts(managerIpAddress string) error {
	cmd := exec.Command("bash", "-c", fmt.Sprintf("ssh-keyscan -H %s >> ~/.ssh/known_hosts", managerIpAddress))
	return cmd.Run()
}

// generateTerraformCommand generates terraform apply command for aws provider
func (a *awsConfigurer) generateTerraformCommand() *exec.Cmd {
	return exec.Command(
		"terraform",
		append(
			[]string{"apply", "-auto-approve"},
			a.generateVariables()...,
		)...,
	)
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
		"-var", fmt.Sprintf(`create_broker_server=%t`, a.CreateBrokerServer),
		"-var", fmt.Sprintf(`rds_creds_filepath=%s`, RDS_CREDS_FILE),
	}
}

// createAWSServerConfigurer creates new awsConfigurer from user input
func (c *Container) createAWSServerConfigurer() (ServerProviderConfigurer, error) {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return nil, err
	}

	awsCfg := &awsConfigurer{}
	// Text field values
	awsKey := ""
	awsSecret := ""
	awsRDSInstanceClass := "db.t4g.small"
	awsServerLabelPrefix := "d8x-cluster"
	if cfg.AWSConfig != nil {
		awsKey = cfg.AWSConfig.AccesKey
		awsSecret = cfg.AWSConfig.SecretKey
		if cfg.AWSConfig.RDSInstanceClass != "" {
			awsRDSInstanceClass = cfg.AWSConfig.RDSInstanceClass
		}
		if cfg.AWSConfig.LabelPrefix != "" {
			awsServerLabelPrefix = cfg.AWSConfig.LabelPrefix
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

	fmt.Println("Enter server tag prefix (must be unique between deployments): ")
	labelPrefix, err := c.TUI.NewInput(
		components.TextInputOptValue(awsServerLabelPrefix),
		components.TextInputOptPlaceholder("my-cluster"),
	)
	if err != nil {
		return nil, err
	}
	awsCfg.LabelPrefix = labelPrefix

	// Broker-server
	createBrokerServer, err := c.TUI.NewPrompt("Do you want to provision a broker server?", true)
	if err != nil {
		return nil, err
	}
	awsCfg.CreateBrokerServer = createBrokerServer
	// Set to deploy in container for current session
	c.CreateBrokerServer = createBrokerServer

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

// parseAwsRDSCredentialsFile parses postgres credentials file generated by
// terraform into map
func parseAwsRDSCredentialsFile(contents []byte) map[string]string {
	credsMap := map[string]string{}
	for _, line := range strings.Split(string(contents), "\n") {
		kv := strings.Split(line, ":")
		if len(kv) == 2 {
			credsMap[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return credsMap
}

// createRDSDatabases automatically creates new databases on provisioned RDS
// postgres instance.
func (a *awsConfigurer) createRDSDatabases(c *Container, historyDbName, referralDbName string) error {
	// Get RDS credentials
	creds, err := os.ReadFile(RDS_CREDS_FILE)
	if err != nil {
		return err
	}
	credsMap := parseAwsRDSCredentialsFile(creds)

	fmt.Println(styles.ItalicText.Render(
		fmt.Sprintf("Creating databases %s, %s on %s ...", historyDbName, referralDbName, credsMap["host"]),
	))

	ip, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}

	sshConn, err := conn.NewSSHConnection(ip, AWS_DEFAULT_INITIAL_USER, c.SshKeyPath)
	if err != nil {
		return err
	}

	jumpHost := sshConn.GetClient()

	pgPort, err := strconv.Atoi(credsMap["port"])
	if err != nil {
		return fmt.Errorf("could not convert port to int: %w", err)
	}

	// pgx.ConnConfig must be created via ParseConfig!
	pgCnfg, err := pgx.ParseConfig("postgresql://user:passwd@" + credsMap["host"] + ":5432/postgres?sslmode=allow")
	if err != nil {
		return err
	}
	pgCnfg.Host = credsMap["host"]
	pgCnfg.User = credsMap["user"]
	pgCnfg.Password = credsMap["password"]
	pgCnfg.Port = uint16(pgPort)
	pgCnfg.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return jumpHost.Dial(network, addr)
	}

	pgConn, err := pgx.ConnectConfig(context.Background(), pgCnfg)

	if err != nil {
		return fmt.Errorf("connecting to postgres instance: %w", err)
	}

	if _, err := pgConn.Exec(context.Background(), "CREATE DATABASE "+historyDbName); err != nil {
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("creating history database: %v", err),
			),
		)
	} else {
		fmt.Println(
			styles.SuccessText.Render(
				fmt.Sprintf("History database %s was created!", historyDbName),
			),
		)
	}
	if _, err := pgConn.Exec(context.Background(), "CREATE DATABASE "+referralDbName); err != nil {
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("creating referral database: %v", err),
			),
		)
	} else {
		fmt.Println(
			styles.SuccessText.Render(
				fmt.Sprintf("Referral database %s was created!", referralDbName),
			),
		)
	}

	return nil
}
