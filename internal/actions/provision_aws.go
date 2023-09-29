package actions

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/jackc/pgx/v5"
	"github.com/urfave/cli/v2"
)

// Path to postgres rds credentials file
const RDS_CREDS_FILE = "aws_rds_postgres.txt"

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
		"embedded/trader-backend/tf-aws/vars.tf",
		"embedded/trader-backend/tf-aws/output.tf",
	); err != nil {
		return nil, fmt.Errorf("generating aws.tf file: %w", err)
	}

	return a.generateTerraformCommand(), nil
}

func (a *awsConfigurer) PostProvisioningAction(c *Container) error {
	a.pullRDSCaCert(c.PgCrtPath)
	fmt.Println(styles.AlertImportant.Render("Important RDS instance information"))
	confirmText := `
RDS Postgres credentials are stored in %s file. Please make
sure you use these credentials from this file when providing
DATABASE_DSN_REFERRAL and DATABASE_DSN_HISTORY .env values.
`
	confirmText = fmt.Sprintf(confirmText, RDS_CREDS_FILE)
	if err := c.TUI.NewConfirmation(strings.TrimSpace(confirmText)); err != nil {
		return err
	}

	// Create databases so users don't have to do this manually
	ok, err := c.TUI.NewPrompt("Do you want to automatically create new databases for history and referral services on provisioned RDS instance?", true)
	if err != nil {
		return err
	}
	if ok {
		fmt.Println("Enter the name of history database:")
		historyDbName, err := c.TUI.NewInput()
		if err != nil {
			return err
		}
		fmt.Println("Enter the name of history database:")
		referralDbName, err := c.TUI.NewInput()
		if err != nil {
			return err
		}
		if err := a.createRDSDatabases(c, historyDbName, referralDbName); err != nil {
			return fmt.Errorf("creating new databases in RDS instance: %w", err)
		}
	}

	return nil
}

// pullRDSCaCert pulls RDS CA cert from AWS
func (a *awsConfigurer) pullRDSCaCert(pgCertPath string) error {
	url :=
		fmt.Sprintf(
			"https://truststore.pki.rds.amazonaws.com/%[1]s/%[1]s-bundle.pem",
			a.Region,
		)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("downloading RDS CA cert: %w", err)
	}
	defer resp.Body.Close()

	pemString, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := os.WriteFile(pgCertPath, pemString, 0666); err != nil {
		return fmt.Errorf("could not write RDS CA cert: %w", err)
	}
	return nil
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

	// Broker-server
	createBrokerServer, err := c.TUI.NewPrompt("Do you want to provision a broker-server server?", true)
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

// createRDSDatabases automatically creates new databases on provisioned RDS
// postgres instance.
func (a *awsConfigurer) createRDSDatabases(c *Container, historyDbName, referralDbName string) error {
	// Get RDS credentials
	creds, err := os.ReadFile(RDS_CREDS_FILE)
	if err != nil {
		return err
	}

	// Parse postgres credentials into map
	credsMap := map[string]string{}
	for _, line := range strings.Split(string(creds), "\n") {
		kv := strings.Split(line, ":")
		if len(kv) == 2 {
			credsMap[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	ip, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}

	sshConn, err := conn.NewSSHConnection(ip, "ubuntu", c.SshKeyPath)
	if err != nil {
		return err
	}

	jumpHost := sshConn.GetClient()

	pgPort, err := strconv.Atoi(credsMap["port"])
	if err != nil {
		return fmt.Errorf("could not convert port to int: %w", err)
	}

	pgCnfg, err := pgx.ParseConfig("postgresql://d8xtrader:passwd@d8x-cluster-pg.cl3dizhgcaqw.eu-central-1.rds.amazonaws.com:5432/postgres?sslmode=allow")
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
	}
	if _, err := pgConn.Exec(context.Background(), "CREATE DATABASE "+referralDbName); err != nil {
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("creating referral database: %v", err),
			),
		)
	}

	return nil
}

func (c *Container) Test(ctx *cli.Context) error {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	a := &awsConfigurer{
		D8XAWSConfig: *cfg.AWSConfig,
	}
	fmt.Printf("%+v\n", c.SshKeyPath)

	return a.createRDSDatabases(c, "test_history", "test_referral")
}
