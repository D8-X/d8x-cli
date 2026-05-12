package actions

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

func (c *Container) CopyAWSTFFiles() error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN is required")
	}
	if c.SelectedEnv == "" {
		return fmt.Errorf("no environment selected")
	}

	fmt.Println(styles.ItalicText.Render("Fetching Terraform configs from GitHub..."))
	if err := ghFetchDir(token, "terraform/aws", c.ProvisioningTfDir); err != nil {
		return fmt.Errorf("fetching terraform/aws from GitHub: %w", err)
	}

	tfvars, err := ghReadFile(token, c.SelectedEnv+"/terraform.tfvars")
	if err != nil {
		return fmt.Errorf("fetching %s/terraform.tfvars from GitHub: %w", c.SelectedEnv, err)
	}
	if err := os.WriteFile(filepath.Join(c.ProvisioningTfDir, "env.auto.tfvars"), []byte(tfvars.Content), 0644); err != nil {
		return fmt.Errorf("writing env.auto.tfvars: %w", err)
	}

	return nil
}


func (a *awsConfigurer) BuildTerraformCMD(c *Container) (*exec.Cmd, error) {
	if err := c.CopyAWSTFFiles(); err != nil {
		return nil, err
	}

	return a.generateTerraformCommand(), nil
}

func (a *awsConfigurer) PostProvisioningAction(c *Container) error {
	credsSaved := a.uploadRDSCredsToBitwarden(c)

	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		fmt.Println(styles.ErrorText.Render(fmt.Sprintf("could not read manager public IP from hosts.cfg: %v. Skipping known_hosts update; subsequent SSH operations may prompt for host key confirmation.", err)))
	} else if err := a.putManagerToKnownHosts(managerIp); err != nil {
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("could not update ~/.ssh/known_hosts for manager %s: %v. You may need to accept the host key manually on first SSH (e.g., `ssh %s` and type 'yes').", managerIp, err, managerIp),
			),
		)
	}

	envUpper := strings.ToUpper(c.SelectedEnv)
	if credsSaved {
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf(
			"RDS connection details saved to Bitwarden as AWS_RDS_HOST_%s / AWS_RDS_PORT_%s / AWS_RDS_USER_%s / AWS_RDS_PASSWORD_%s.",
			envUpper, envUpper, envUpper, envUpper,
		)))
	}

	dbDefault := readEnvSecret(c.SelectedEnv, "AWS_RDS_DB_NAME")
	if dbDefault == "" {
		dbDefault = "history"
	}
	fmt.Println("Name of the database the history service will connect to (used as the suffix in DATABASE_DSN):")
	dbName, ierr := c.TUI.NewInput(
		components.TextInputOptValue(dbDefault),
		components.TextInputOptDenyEmpty(),
	)
	if ierr != nil {
		return ierr
	}
	dbName = strings.TrimSpace(dbName)

	dbNameField := "AWS_RDS_DB_NAME_" + envUpper
	dbNameRecorded := false
	if os.Getenv("BW_SESSION") != "" && c.SelectedEnv != "" {
		result, _, err := SaveSecretToBitwardenItem(bwItemName, dbNameField, dbName)
		switch {
		case err != nil:
			fmt.Printf("%s warning: could not save %s to Bitwarden: %s\n", warning, dbNameField, err)
		case result == BwSkippedConflict:
			fmt.Printf("%s warning: %s already exists in Bitwarden with a different value; not overwriting. Edit it manually via \"bw edit\" if you want %q to take effect for future runs; for THIS session only, the operator-entered value is used.\n", warning, dbNameField, dbName)
		default:
			dbNameRecorded = true
		}
	}
	os.Setenv(dbNameField, dbName)
	if dbNameRecorded {
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf(
			"Database name %q recorded in Bitwarden as %s. \"d8x setup swarm-deploy\" will read this back and build DATABASE_DSN with the %q suffix.",
			dbName, dbNameField, dbName,
		)))
	}

	manualRecovery := fmt.Sprintf("connect via \"d8x ssh manager\" and run \"docker run --rm -it --network host postgres:16 psql 'postgres://%%s:%%s@%%s:%%s/postgres' -c 'CREATE DATABASE %s;'\" using the values from Bitwarden fields AWS_RDS_USER_%s / AWS_RDS_PASSWORD_%s / AWS_RDS_HOST_%s / AWS_RDS_PORT_%s", dbName, envUpper, envUpper, envUpper, envUpper)

	create, perr := c.TUI.NewPrompt(fmt.Sprintf("Auto-create the %q database on RDS now? (decline only if you'll create it manually on the manager before \"d8x setup swarm-deploy\")", dbName), true)
	if perr != nil {
		return perr
	}
	if create {
		if err := a.createRDSDatabases(c, dbName); err != nil {
			recordedNote := fmt.Sprintf("The name %q is still recorded in Bitwarden as %s, so the DSN built by \"d8x setup swarm-deploy\" will point at it.", dbName, dbNameField)
			if !dbNameRecorded {
				recordedNote = fmt.Sprintf("The name %q was NOT recorded in Bitwarden (see the warning above). Either fix that, or expect \"d8x setup swarm-deploy\" to fall back to the default \"history\" suffix.", dbName)
			}
			fmt.Printf("%s warning: could not auto-create %q on RDS: %s. %s To create the database manually later, %s. Alternatively, rerun \"d8x setup provision\" and accept the auto-create prompt to retry.\n", warning, dbName, err, recordedNote, manualRecovery)
		}
	} else {
		recordedNote := fmt.Sprintf("The name %q is recorded in Bitwarden as %s, so the DSN will point at %q.", dbName, dbNameField, dbName)
		if !dbNameRecorded {
			recordedNote = fmt.Sprintf("The name %q was NOT recorded in Bitwarden (see the warning above); \"d8x setup swarm-deploy\" will fall back to the default \"history\" suffix unless you set %s manually.", dbName, dbNameField)
		}
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf(
			"Skipping auto-create. %s Before running \"d8x setup swarm-deploy\", create the database manually: %s.",
			recordedNote, manualRecovery,
		)))
	}

	return nil
}

func (a *awsConfigurer) uploadRDSCredsToBitwarden(c *Container) bool {
	if os.Getenv("BW_SESSION") == "" {
		fmt.Println(styles.ItalicText.Render("BW_SESSION not set; not uploading RDS credentials to Bitwarden. They remain in ./aws_rds_postgres.txt only. Unlock Bitwarden (\"bw unlock\") and rerun \"d8x setup provision\" to persist them."))
		return false
	}
	if c.SelectedEnv == "" {
		return false
	}
	creds, err := os.ReadFile(RDS_CREDS_FILE)
	if err != nil {
		fmt.Printf("%s warning: could not read %s after terraform: %s. RDS credentials not uploaded to Bitwarden.\n", warning, RDS_CREDS_FILE, err)
		return false
	}
	credsMap := parseAwsRDSCredentialsFile(creds)
	envUpper := strings.ToUpper(c.SelectedEnv)
	fields := []struct {
		key       string
		fieldName string
	}{
		{"host", "AWS_RDS_HOST_" + envUpper},
		{"port", "AWS_RDS_PORT_" + envUpper},
		{"user", "AWS_RDS_USER_" + envUpper},
		{"password", "AWS_RDS_PASSWORD_" + envUpper},
	}
	savedAll := true
	for _, f := range fields {
		v := credsMap[f.key]
		if v == "" {
			savedAll = false
			continue
		}
		result, _, err := SaveSecretToBitwardenItem(bwItemName, f.fieldName, v)
		switch {
		case err != nil:
			fmt.Printf("%s warning: could not save %s to Bitwarden: %s\n", warning, f.fieldName, err)
			savedAll = false
		case result == BwSkippedConflict:
			fmt.Printf("%s warning: %s already exists in Bitwarden with a different value; not overwriting.\n", warning, f.fieldName)
			savedAll = false
		default:
			os.Setenv(f.fieldName, v)
		}
	}
	return savedAll
}

func loadRDSCredsFromBitwarden(env string) map[string]string {
	if env == "" {
		return nil
	}
	envUpper := strings.ToUpper(env)
	keys := map[string]string{
		"host":     "AWS_RDS_HOST_" + envUpper,
		"port":     "AWS_RDS_PORT_" + envUpper,
		"user":     "AWS_RDS_USER_" + envUpper,
		"password": "AWS_RDS_PASSWORD_" + envUpper,
	}
	out := map[string]string{}
	for k, fieldName := range keys {
		v := os.Getenv(fieldName)
		if v == "" {
			return nil
		}
		out[k] = v
	}
	return out
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
	cmd := exec.Command(
		"terraform",
		append(
			[]string{"apply", "-auto-approve"},
			a.generateVariables()...,
		)...,
	)

	return cmd
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
		"-var", fmt.Sprintf(`create_swarm=%t`, a.DeploySwarm),
		"-var", fmt.Sprintf(`num_workers=%d`, a.NumWorker),
	}
}

// CollectAwProviderDetails collects aws provider details from user input,
// creates a new awsConfigurer and fills in configuration details to cfg.
func (c *InputCollector) CollectAwProviderDetails(cfg *configs.D8XConfig) (awsConfigurer, error) {
	awsCfg := awsConfigurer{}

	// Default text field values
	awsRDSInstanceClass := "db.t4g.small"
	awsServerLabelPrefix := "d8x-cluster"
	awsDefaultNumberWorkers := "4"
	awsDefaultRegion := "eu-central-1"

	if cfg.AWSConfig != nil {
		if cfg.AWSConfig.RDSInstanceClass != "" {
			awsRDSInstanceClass = cfg.AWSConfig.RDSInstanceClass
		}
		if cfg.AWSConfig.LabelPrefix != "" {
			awsServerLabelPrefix = cfg.AWSConfig.LabelPrefix
		}
		if cfg.AWSConfig.NumWorker != 0 {
			awsDefaultNumberWorkers = strconv.Itoa(cfg.AWSConfig.NumWorker)
		}
		if cfg.AWSConfig.Region != "" {
			awsDefaultRegion = cfg.AWSConfig.Region
		}
	}

	// Check for swarm deployment
	awsCfg.DeploySwarm = c.setup.deploySwarm

	accessKey := readEnvSecret(c.SelectedEnv, "AWS_ACCESS_KEY")
	if accessKey == "" {
		fmt.Println("Enter your AWS Access Key: ")
		var err error
		accessKey, err = c.TUI.NewInput(
			components.TextInputOptPlaceholder("<AWS_ACCESS_KEY>"),
			components.TextInputOptMasked(),
			components.TextInputOptDenyEmpty(),
		)
		if err != nil {
			return awsCfg, err
		}
	}
	awsCfg.AccesKey = accessKey
	if os.Getenv("BW_SESSION") != "" {
		saveAndReportPersonal(envSuffixedField(c.SelectedEnv, "AWS_ACCESS_KEY"), accessKey)
	}

	secretKey := readEnvSecret(c.SelectedEnv, "AWS_SECRET_KEY")
	if secretKey == "" {
		fmt.Println("Enter your AWS Secret Key: ")
		var err error
		secretKey, err = c.TUI.NewInput(
			components.TextInputOptPlaceholder("<AWS_SECRET_KEY>"),
			components.TextInputOptMasked(),
		)
		if err != nil {
			return awsCfg, err
		}
	}
	awsCfg.SecretKey = secretKey
	if os.Getenv("BW_SESSION") != "" {
		saveAndReportPersonal(envSuffixedField(c.SelectedEnv, "AWS_SECRET_KEY"), secretKey)
	}

	fmt.Println("Enter your AWS cluster region: ")

	region, err := c.TUI.NewInput(
		components.TextInputOptValue(awsDefaultRegion),
		components.TextInputOptPlaceholder("us-west-1"),
	)
	if err != nil {
		return awsCfg, err
	}
	awsCfg.Region = region

	prefixDefault := awsServerLabelPrefix
	if c.SelectedEnv != "" {
		prefixDefault = c.SelectedEnv
	}
	fmt.Println("Enter server tag prefix (must be unique between deployments; defaults to env name):")
	labelPrefix, err := c.TUI.NewInput(
		components.TextInputOptValue(prefixDefault),
		components.TextInputOptPlaceholder("my-cluster"),
	)
	if err != nil {
		return awsCfg, err
	}
	awsCfg.LabelPrefix = labelPrefix
	awsCfg.CreateBrokerServer = c.setup.deployBroker

	// Collect swarm details
	if c.setup.deploySwarm {
		fmt.Println("Enter your AWS RDS DB instance class: ")
		dbClass, err := c.TUI.NewInput(
			components.TextInputOptValue(awsRDSInstanceClass),
			components.TextInputOptPlaceholder("db.t3.medium"),
		)
		if err != nil {
			return awsCfg, err
		}
		awsCfg.RDSInstanceClass = dbClass

		numWorkers, err := c.CollectNumberOfWorkers(awsDefaultNumberWorkers)
		if err != nil {
			return awsCfg, fmt.Errorf("incorrect number of workers: %w", err)
		}
		awsCfg.NumWorker = numWorkers
	}

	cfgAws := awsCfg.D8XAWSConfig
	cfgAws.AccesKey = ""
	cfgAws.SecretKey = ""
	cfgAws.RDSCredentialsFilePath = ""
	cfg.AWSConfig = &cfgAws
	cfg.ServerProvider = configs.D8XServerProviderAWS

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

func (a *awsConfigurer) createRDSDatabases(c *Container, historyDbName string) error {
	credsMap := loadRDSCredsFromBitwarden(c.SelectedEnv)
	if credsMap == nil {
		creds, err := os.ReadFile(RDS_CREDS_FILE)
		if err != nil {
			return fmt.Errorf("RDS credentials not found in Bitwarden and could not read %s: %w", RDS_CREDS_FILE, err)
		}
		credsMap = parseAwsRDSCredentialsFile(creds)
	}

	envUpper := strings.ToUpper(c.SelectedEnv)
	fmt.Println(styles.ItalicText.Render(
		fmt.Sprintf("Creating empty database %q on RDS host %s (using credentials from Bitwarden field AWS_RDS_USER_%s / AWS_RDS_PASSWORD_%s) ...", historyDbName, credsMap["host"], envUpper, envUpper),
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
				fmt.Sprintf("CREATE DATABASE %q failed on %s: %v", historyDbName, credsMap["host"], err),
			),
		)
		return err
	}
	fmt.Println(
		styles.SuccessText.Render(
			fmt.Sprintf("Created empty database %q on %s. The history container will connect here and migrate its own schema on first boot. After \"d8x setup swarm-deploy\" persists DATABASE_DSN_%s to Bitwarden, you can connect anytime via \"d8x db-tunnel\".", historyDbName, credsMap["host"], envUpper),
		),
	)

	return nil
}

