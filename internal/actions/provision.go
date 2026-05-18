package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

type SupportedServerProvider string

const (
	ServerProviderLinode SupportedServerProvider = "linode"
	ServerProviderAws    SupportedServerProvider = "aws"
)

var TF_FILES_DIR = filepath.Join(os.TempDir(), "d8x-cli", "terraform")

func (c *Container) Provision(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting provisioning...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	env, err := c.EnsureEnvironment(cfg)
	if err != nil {
		return err
	}
	c.ProvisioningTfDir = c.tfDir()
	if err := os.MkdirAll(c.ProvisioningTfDir, 0700); err != nil {
		return fmt.Errorf("preparing terraform work dir: %w", err)
	}
	if err := c.ensureSSHKey(env); err != nil {
		return err
	}

	if c.HostsCfg != nil {
		if managerIp, mErr := c.HostsCfg.GetMangerPublicIp(); mErr == nil && managerIp != "" {
			fmt.Println(styles.AlertImportant.Render(fmt.Sprintf(
				"Env %q appears to be already provisioned (manager IP %s present in hosts.cfg).",
				c.SelectedEnv, managerIp,
			)))
			fmt.Println(styles.ItalicText.Render("Re-running provision will recreate or modify cloud servers and almost certainly cause downtime."))
			proceed, perr := c.TUI.NewPrompt("Force re-run of \"d8x setup provision\" anyway?", false)
			if perr != nil {
				return perr
			}
			if !proceed {
				fmt.Println(styles.ItalicText.Render("Skipping provision; run \"d8x tf-destroy\" first if you want a clean re-provision."))
				return nil
			}
		}
	}

	preCfgSnapshot := snapshotProvisioningCfg(cfg)
	preTfvars := buildTfvars(cfg)

	c.printProvisionSummary(cfg)
	const (
		choiceProceed = "Proceed with these values"
		choiceEdit    = "Edit values before proceeding"
		choiceStop    = "Stop here"
	)
	picked, err := c.TUI.NewSelection(
		[]string{choiceProceed, choiceEdit, choiceStop},
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)
	if err != nil {
		return err
	}
	switch picked[0] {
	case choiceStop:
		fmt.Println(styles.ItalicText.Render("Provisioning stopped at operator request. Edit values in the infra repo and rerun \"d8x setup provision\" when ready."))
		return nil
	case choiceEdit:
		fmt.Println(styles.ItalicText.Render("Step through each field; press enter to keep the shown value."))
		if err := c.editProvisioningSummary(cfg); err != nil {
			return err
		}
		if err := c.ConfigRWriter.Write(cfg); err != nil {
			return err
		}
		if err := c.Input.CollectProvisioningCredentialsOnly(ctx); err != nil {
			return err
		}
	default:
		if err := c.Input.CollectProvisioningCredentialsOnly(ctx); err != nil {
			return err
		}
	}

	if !hasProvisionTargets(cfg) {
		fmt.Println(styles.AlertImportant.Render(
			"Nothing to provision: both \"create broker server\" and \"deploy swarm\" are false for this env.",
		))
		fmt.Println(styles.ItalicText.Render(
			"Set at least one of them to true: rerun \"d8x setup provision\" and pick \"Edit values before proceeding\", or fix the values directly in <env>/config.json on the infra repo.",
		))
		return fmt.Errorf("aborted: nothing to provision (no broker, no swarm)")
	}

	postCfgSnapshot := snapshotProvisioningCfg(cfg)
	postTfvars := buildTfvars(cfg)
	if preCfgSnapshot != postCfgSnapshot || preTfvars != postTfvars {
		const source = "\"d8x setup provision\""
		if err := c.PublishRemoteConfigWithSource(cfg, source); err != nil {
			fmt.Printf("  %s could not push updated config.json: %s\n", notok, err)
		}
		if err := c.PublishRemoteTfvars(cfg, source); err != nil {
			fmt.Printf("  %s could not push updated terraform.tfvars: %s\n", notok, err)
		}
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
	tfInit.Dir = c.ProvisioningTfDir
	connectCMDToCurrentTerm(tfInit)
	if err := c.RunCmd(tfInit); err != nil {
		return err
	}

	if tfCmd != nil {
		// Set the tf dir
		tfCmd.Dir = c.ProvisioningTfDir

		connectCMDToCurrentTerm(tfCmd)
		err := c.RunCmd(tfCmd)
		if err != nil {
			fmt.Println(styles.ErrorText.Render("Terraform apply failed, please check the output above for more details.\nPossible issues:\n\tDuplicate server label\n\tIncorrect server provider credentials\n\tSelected region was used first time"))
			return err
		}
	}

	// Set the provisioning time
	c.provisioningTime = time.Now()

	if hostsContent, herr := os.ReadFile(c.hostsCfgPath()); herr == nil {
		c.HostsCfg = files.NewMemHostsFileInteractor(hostsContent, hostsCfgGitHubPusher(c.SelectedEnv))
	}

	// Perform provider dependent actions
	if err := providerConfigurer.PostProvisioningAction(c); err != nil {
		return err
	}

	// Update the input
	if err := c.Input.PostProvisioningHook(); err != nil {
		return err
	}

	if token := os.Getenv("GITHUB_TOKEN"); token != "" && c.SelectedEnv != "" {
		hostsContent, err := os.ReadFile(c.hostsCfgPath())
		if err == nil {
			path := c.SelectedEnv + "/hosts.cfg"
			existing, _ := ghReadFile(token, path)
			sha := ""
			if existing != nil {
				sha = existing.SHA
				if existing.Content == string(hostsContent) {
					fmt.Printf("  %s hosts.cfg already up to date on GitHub (%s)\n", ok, path)
					return nil
				}
			}
			_, err := ghWriteFile(token, path, string(hostsContent), sha, "update "+path)
			if err != nil {
				fmt.Printf("  %s Could not push hosts.cfg to GitHub: %s\n", notok, err)
			} else {
				fmt.Printf("  %s hosts.cfg pushed to GitHub (%s)\n", ok, path)
			}
		}
	}

	return nil
}

func hostsCfgGitHubPusher(env string) func(content string) error {
	return func(content string) error {
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" || env == "" {
			return nil
		}
		path := env + "/hosts.cfg"
		sha := ""
		if existing, _ := ghReadFile(token, path); existing != nil {
			sha = existing.SHA
			if existing.Content == content {
				fmt.Printf("%s %s already up to date in infra repo\n", ok, path)
				return nil
			}
		}
		fmt.Printf("%s overwriting %s in infra repo\n", warning, path)
		if _, err := ghWriteFile(token, path, content, sha, "update "+path); err != nil {
			return fmt.Errorf("pushing hosts.cfg to infra repo: %w", err)
		}
		fmt.Printf("%s pushed %s to infra repo\n", ok, path)
		return nil
	}
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

func (c *Container) printProvisionSummary(cfg *configs.D8XConfig) {
	fmt.Println()
	fmt.Println(styles.CommandTitleText.Render(fmt.Sprintf("Current provisioning values for %q (chain %d):", c.SelectedEnv, cfg.ChainId)))
	fmt.Printf("  provider          : %s\n", cfg.ServerProvider)
	switch cfg.ServerProvider {
	case configs.D8XServerProviderLinode:
		if cfg.LinodeConfig != nil {
			fmt.Printf("  region            : %s\n", cfg.LinodeConfig.Region)
			fmt.Printf("  label prefix      : %s\n", cfg.LinodeConfig.LabelPrefix)
			fmt.Printf("  workers           : %d\n", cfg.LinodeConfig.NumWorker)
			fmt.Printf("  broker server size: %s\n", cfg.LinodeConfig.BrokerServerSize)
			fmt.Printf("  create broker     : %t\n", cfg.LinodeConfig.CreateBrokerServer)
			fmt.Printf("  deploy swarm      : %t\n", cfg.LinodeConfig.DeploySwarm)
			fmt.Printf("  external db id    : %s\n", cfg.LinodeConfig.DbId)
		}
	case configs.D8XServerProviderAWS:
		if cfg.AWSConfig != nil {
			fmt.Printf("  region            : %s\n", cfg.AWSConfig.Region)
			fmt.Printf("  label prefix      : %s\n", cfg.AWSConfig.LabelPrefix)
			fmt.Printf("  workers           : %d\n", cfg.AWSConfig.NumWorker)
			fmt.Printf("  rds instance class: %s\n", cfg.AWSConfig.RDSInstanceClass)
			fmt.Printf("  create broker     : %t\n", cfg.AWSConfig.CreateBrokerServer)
			fmt.Printf("  deploy swarm      : %t\n", cfg.AWSConfig.DeploySwarm)
		}
	}
}

func (c *Container) editProvisioningSummary(cfg *configs.D8XConfig) error {
	switch cfg.ServerProvider {
	case configs.D8XServerProviderLinode:
		if cfg.LinodeConfig == nil {
			cfg.LinodeConfig = &configs.D8XLinodeConfig{}
		}
		lc := cfg.LinodeConfig
		v, err := c.promptString("region:", lc.Region)
		if err != nil {
			return err
		}
		lc.Region = v
		v, err = c.promptString("label prefix:", lc.LabelPrefix)
		if err != nil {
			return err
		}
		lc.LabelPrefix = v
		n, err := c.promptInt("workers:", lc.NumWorker)
		if err != nil {
			return err
		}
		lc.NumWorker = n
		v, err = c.promptString("broker server size:", defaultStr(lc.BrokerServerSize, "g6-dedicated-2"))
		if err != nil {
			return err
		}
		lc.BrokerServerSize = v
		v, err = c.promptString("swarm node size:", defaultStr(lc.SwarmNodeSize, "g6-dedicated-2"))
		if err != nil {
			return err
		}
		lc.SwarmNodeSize = v
		b, err := c.TUI.NewPrompt("create broker server?", lc.CreateBrokerServer)
		if err != nil {
			return err
		}
		lc.CreateBrokerServer = b
		b, err = c.TUI.NewPrompt("deploy swarm?", lc.DeploySwarm)
		if err != nil {
			return err
		}
		lc.DeploySwarm = b
		useExternalDb := lc.DbId != ""
		useExternalDb, err = c.TUI.NewPrompt("use an external Linode managed Postgres cluster?", useExternalDb)
		if err != nil {
			return err
		}
		if useExternalDb {
			v, err := c.promptString("external Linode database cluster ID:", lc.DbId)
			if err != nil {
				return err
			}
			lc.DbId = v
		} else {
			lc.DbId = ""
		}
	case configs.D8XServerProviderAWS:
		if cfg.AWSConfig == nil {
			cfg.AWSConfig = &configs.D8XAWSConfig{}
		}
		ac := cfg.AWSConfig
		v, err := c.promptString("region:", ac.Region)
		if err != nil {
			return err
		}
		ac.Region = v
		v, err = c.promptString("label prefix:", ac.LabelPrefix)
		if err != nil {
			return err
		}
		ac.LabelPrefix = v
		n, err := c.promptInt("workers:", ac.NumWorker)
		if err != nil {
			return err
		}
		ac.NumWorker = n
		v, err = c.promptString("rds instance class:", defaultStr(ac.RDSInstanceClass, "db.t4g.small"))
		if err != nil {
			return err
		}
		ac.RDSInstanceClass = v
		b, err := c.TUI.NewPrompt("create broker server?", ac.CreateBrokerServer)
		if err != nil {
			return err
		}
		ac.CreateBrokerServer = b
		b, err = c.TUI.NewPrompt("deploy swarm?", ac.DeploySwarm)
		if err != nil {
			return err
		}
		ac.DeploySwarm = b
	default:
		return fmt.Errorf("server_provider is empty in %s/config.json on the infra repo; cannot edit values", c.SelectedEnv)
	}
	return nil
}

func (c *Container) promptString(label, current string) (string, error) {
	fmt.Println(label)
	v, err := c.TUI.NewInput(components.TextInputOptValue(current))
	if err != nil {
		return "", err
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return current, nil
	}
	return v, nil
}

func (c *Container) promptInt(label string, current int) (int, error) {
	for {
		fmt.Println(label)
		v, err := c.TUI.NewInput(components.TextInputOptValue(strconv.Itoa(current)))
		if err != nil {
			return 0, err
		}
		v = strings.TrimSpace(v)
		if v == "" {
			return current, nil
		}
		n, parseErr := strconv.Atoi(v)
		if parseErr == nil && n >= 0 {
			return n, nil
		}
		fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Invalid value %q; expected a non-negative integer.", v)))
	}
}

func hasProvisionTargets(cfg *configs.D8XConfig) bool {
	switch cfg.ServerProvider {
	case configs.D8XServerProviderLinode:
		if cfg.LinodeConfig == nil {
			return false
		}
		return cfg.LinodeConfig.CreateBrokerServer || cfg.LinodeConfig.DeploySwarm
	case configs.D8XServerProviderAWS:
		if cfg.AWSConfig == nil {
			return false
		}
		return cfg.AWSConfig.CreateBrokerServer || cfg.AWSConfig.DeploySwarm
	}
	return false
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func snapshotProvisioningCfg(cfg *configs.D8XConfig) string {
	type snap struct {
		Provider string                  `json:"provider"`
		Linode   *configs.D8XLinodeConfig `json:"linode,omitempty"`
		AWS      *configs.D8XAWSConfig    `json:"aws,omitempty"`
	}
	s := snap{Provider: string(cfg.ServerProvider)}
	if cfg.LinodeConfig != nil {
		lc := *cfg.LinodeConfig
		lc.Token = ""
		s.Linode = &lc
	}
	if cfg.AWSConfig != nil {
		aws := *cfg.AWSConfig
		aws.AccesKey = ""
		aws.SecretKey = ""
		aws.RDSCredentialsFilePath = ""
		s.AWS = &aws
	}
	data, _ := json.Marshal(s)
	return string(data)
}

func (c *InputCollector) CollectProvisioningCredentialsOnly(ctx *cli.Context) error {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if c.SSHKeyPath == "" {
		return fmt.Errorf("SSH key path not set; ensureSSHKey must run before CollectProvisioningCredentialsOnly")
	}
	authorizedKey, err := getPublicKey(c.SSHKeyPath)
	if err != nil {
		return fmt.Errorf("reading SSH public key: %w", err)
	}
	if strings.TrimSpace(authorizedKey) == "" {
		return fmt.Errorf("SSH public key at %s.pub is empty; cannot pass to terraform", c.SSHKeyPath)
	}
	switch cfg.ServerProvider {
	case configs.D8XServerProviderLinode:
		token := readEnvSecret(c.SelectedEnv, "LINODE_TOKEN")
		if token == "" {
			fmt.Println("Enter your Linode API token")
			t, err := c.TUI.NewInput(
				components.TextInputOptPlaceholder("<YOUR LINODE API TOKEN>"),
				components.TextInputOptMasked(),
			)
			if err != nil {
				return err
			}
			token = t
			if os.Getenv("BW_SESSION") != "" {
				saveAndReport(envSuffixedField(c.SelectedEnv, "LINODE_TOKEN"), token)
			}
		}
		if cfg.LinodeConfig != nil {
			cfg.LinodeConfig.Token = token
		}
		c.provisioning.selectedServerProvider = ServerProviderLinode
		c.provisioning.collectedLinodeConfigurer = &linodeConfigurer{
			D8XLinodeConfig: *cfg.LinodeConfig,
			authorizedKey:   authorizedKey,
		}
	case configs.D8XServerProviderAWS:
		access := readEnvSecret(c.SelectedEnv, "AWS_ACCESS_KEY")
		if access == "" {
			fmt.Println("Enter your AWS Access Key: ")
			a, err := c.TUI.NewInput(
				components.TextInputOptPlaceholder("<AWS_ACCESS_KEY>"),
				components.TextInputOptMasked(),
				components.TextInputOptDenyEmpty(),
			)
			if err != nil {
				return err
			}
			access = a
			if os.Getenv("BW_SESSION") != "" {
				saveAndReportPersonal(envSuffixedField(c.SelectedEnv, "AWS_ACCESS_KEY"), access)
			}
		}
		secret := readEnvSecret(c.SelectedEnv, "AWS_SECRET_KEY")
		if secret == "" {
			fmt.Println("Enter your AWS Secret Key: ")
			s, err := c.TUI.NewInput(
				components.TextInputOptPlaceholder("<AWS_SECRET_KEY>"),
				components.TextInputOptMasked(),
			)
			if err != nil {
				return err
			}
			secret = s
			if os.Getenv("BW_SESSION") != "" {
				saveAndReportPersonal(envSuffixedField(c.SelectedEnv, "AWS_SECRET_KEY"), secret)
			}
		}
		if cfg.AWSConfig != nil {
			cfg.AWSConfig.AccesKey = access
			cfg.AWSConfig.SecretKey = secret
		}
		c.provisioning.selectedServerProvider = ServerProviderAws
		c.provisioning.collectedAwsConfigurer = &awsConfigurer{
			D8XAWSConfig:  *cfg.AWSConfig,
			authorizedKey: authorizedKey,
		}
	default:
		return fmt.Errorf("server_provider is empty in %s/config.json on the infra repo; cannot run provision", c.SelectedEnv)
	}
	return c.ConfigRWriter.Write(cfg)
}

// CollectNumberOfWorkers collects number of workers input from user
func (c *InputCollector) CollectNumberOfWorkers(defaultNum string) (int, error) {
	fmt.Println("Enter number of worker servers to create: ")
	numWorkers, err := c.TUI.NewInput(
		components.TextInputOptValue(defaultNum),
		components.TextInputOptPlaceholder("4"),
		components.TextInputOptValidation(func(s string) bool {
			_, err := strconv.Atoi(s)
			return err == nil
		}, "please provide a valid number"),
	)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(numWorkers)
}
