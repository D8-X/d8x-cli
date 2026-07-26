package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
)

// EnsureEnvironment selects an environment, fetches hosts.cfg from the GitHub
// repo, and configures SSH key and password for the selected environment.
func (c *Container) EnsureEnvironment(cfg *configs.D8XConfig) (string, error) {
	return c.ensureEnvironment(cfg, false)
}

func (c *Container) EnsureProvisionedEnvironment(cfg *configs.D8XConfig) (string, error) {
	return c.ensureEnvironment(cfg, true)
}

func (c *Container) ensureEnvironment(cfg *configs.D8XConfig, provisionedOnly bool) (string, error) {
	if err := c.RequireBitwardenField("GITHUB_TOKEN"); err != nil {
		return "", err
	}
	token := os.Getenv("GITHUB_TOKEN")

	env := c.SelectedEnv
	var remoteCfg *configs.D8XConfig

	if env == "" {
		allDirs, err := ghListDirs(token)
		if err != nil {
			fmt.Printf("%s Cannot access repo '%s'. Check your GITHUB_TOKEN has access to it.\n", notok, getGhRepo())
			fmt.Println("Enter infra repo (owner/name) or press enter to retry:")
			repo, inputErr := c.TUI.NewInput(components.TextInputOptValue(getGhRepo()))
			if inputErr != nil {
				return "", inputErr
			}
			os.Setenv("INFRA_REPO", repo)
			allDirs, err = ghListDirs(token)
			if err != nil {
				return "", fmt.Errorf("cannot access repo '%s': %w", getGhRepo(), err)
			}
		}

		var environments []string
		var labels []string
		var envConfigs []configs.D8XConfig
		for _, e := range allDirs {
			cfgFile, err := ghReadFile(token, e+"/config.json")
			if err != nil {
				if !strings.Contains(err.Error(), "404") {
					fmt.Printf("%s environment '%s' skipped: config.json unreadable (%s)\n", notok, e, err)
				}
				continue
			}
			var ec configs.D8XConfig
			if err := json.Unmarshal([]byte(cfgFile.Content), &ec); err != nil {
				fmt.Printf("%s environment '%s' skipped: config.json is not valid JSON (%s)\n", notok, e, err)
				continue
			}

			hostsFile, hostsErr := ghReadFile(token, e+"/hosts.cfg")
			provisioned := hostsErr == nil && strings.TrimSpace(hostsFile.Content) != ""
			if provisionedOnly && !provisioned {
				continue
			}

			environments = append(environments, e)
			envConfigs = append(envConfigs, ec)
			label := e
			if ec.ChainId > 0 {
				label = fmt.Sprintf("%s  (chain %d)", e, ec.ChainId)
			}
			if !provisioned {
				label += "  [not provisioned]"
			}
			labels = append(labels, label)
		}
		if len(environments) == 0 {
			if provisionedOnly {
				return "", fmt.Errorf("no provisioned environments found in %s repo. Run \"d8x setup provision\" first", getGhRepo())
			}
			return "", fmt.Errorf("no environments found in %s repo", getGhRepo())
		}

		prompt := "Select environment:"
		if provisionedOnly {
			prompt = "Select the provisioned environment:"
		}
		fmt.Println(styles.ItalicText.Render(prompt))
		selected, err := c.TUI.NewSelection(labels, components.SelectionOptAllowOnlySingleItem(), components.SelectionOptRequireSelection())
		if err != nil {
			return "", err
		}
		idx := indexOf(labels, selected[0])
		env = environments[idx]
		remoteCfg = &envConfigs[idx]
	} else {
		cfgFile, err := ghReadFile(token, env+"/config.json")
		if err != nil {
			return "", fmt.Errorf("fetching %s/config.json from infra repo: %w", env, err)
		}
		var ec configs.D8XConfig
		if uErr := json.Unmarshal([]byte(cfgFile.Content), &ec); uErr != nil {
			return "", fmt.Errorf("%s/config.json on infra repo is not valid JSON: %w", env, uErr)
		}
		remoteCfg = &ec
	}
	fmt.Printf("Environment: %s\n", env)

	loadRemoteConfig(cfg, remoteCfg)
	if cfg.ServerProvider == "" {
		if recovered := recoverProviderFromTfvars(token, env); recovered != nil {
			fmt.Printf("%s recovered server_provider/linode_config from %s/terraform.tfvars (config.json was missing it)\n", warning, env)
			cfg.ServerProvider = recovered.ServerProvider
			cfg.LinodeConfig = recovered.LinodeConfig
			cfg.AWSConfig = recovered.AWSConfig
		}
	}
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return "", fmt.Errorf("writing config: %w", err)
	}

	var hostsContent []byte
	var hostsSHA string
	hostsFile, err := ghReadFile(token, env+"/hosts.cfg")
	switch {
	case err == nil:
		if strings.TrimSpace(hostsFile.Content) == "" {
			fmt.Printf("%s remote hosts.cfg for '%s' is empty — treating as not-yet-provisioned\n", notok, env)
			hostsSHA = hostsFile.SHA
		} else {
			hostsContent = []byte(hostsFile.Content)
			hostsSHA = hostsFile.SHA
		}
	case strings.Contains(err.Error(), "404"):
		fmt.Printf("%s no remote hosts.cfg for '%s' yet — assuming first provision\n", notok, env)
	default:
		return "", fmt.Errorf("could not fetch hosts.cfg for %s from GitHub: %w", env, err)
	}
	hostsRemotePath := env + "/hosts.cfg"
	c.HostsCfg = files.NewMemHostsFileInteractor(hostsContent, func(content string) error {
		if hostsSHA != "" && string(hostsContent) == content {
			if strings.TrimSpace(content) == "" {
				return nil
			}
			fmt.Printf("%s %s already up to date in infra repo\n", ok, hostsRemotePath)
			return nil
		}
		fmt.Printf("%s overwriting %s in infra repo\n", warning, hostsRemotePath)
		newSHA, werr := ghWriteFile(token, hostsRemotePath, content, hostsSHA, "update "+env+"/hosts.cfg")
		if werr != nil {
			return fmt.Errorf("pushing hosts.cfg to infra repo: %w", werr)
		}
		hostsSHA = newSHA
		hostsContent = []byte(content)
		fmt.Printf("%s pushed %s to infra repo\n", ok, hostsRemotePath)
		return nil
	})

	if hostsSHA != "" {
		if err := c.ensureSSHKey(env); err != nil {
			return "", err
		}
	}

	pwdKey := "SERVER_PASSWORD_" + strings.ToUpper(env)
	if pwd := os.Getenv(pwdKey); pwd != "" {
		c.UserPassword = pwd
	}

	c.SelectedEnv = env
	if c.Input != nil {
		c.Input.SelectedEnv = env
	}
	return env, nil
}

func loadRemoteConfig(cfg, remoteCfg *configs.D8XConfig) {
	if remoteCfg == nil {
		return
	}

	remoteCopy, mErr := json.Marshal(remoteCfg)
	if mErr == nil {
		var clone configs.D8XConfig
		if uErr := json.Unmarshal(remoteCopy, &clone); uErr == nil {
			*cfg = clone
		} else {
			*cfg = *remoteCfg
		}
	} else {
		*cfg = *remoteCfg
	}

	if cfg.Services == nil {
		cfg.Services = make(map[configs.D8XServiceName]configs.D8XService)
	}
	if cfg.HttpRpcList == nil {
		cfg.HttpRpcList = make(map[string][]string)
	}
	if cfg.WsRpcList == nil {
		cfg.WsRpcList = make(map[string][]string)
	}

	if cfg.ChainId != 0 {
		fmt.Printf("%s loaded env config from infra repo: chain_id=%d\n", ok, cfg.ChainId)
	}
}

func recoverProviderFromTfvars(token, env string) *configs.D8XConfig {
	if token == "" || env == "" {
		return nil
	}
	tfvarsFile, err := ghReadFile(token, env+"/terraform.tfvars")
	if err != nil {
		return nil
	}
	vars := parseTfvars(tfvarsFile.Content)
	if len(vars) == 0 {
		return nil
	}
	_, hasBrokerSize := vars["broker_size"]
	_, hasRDSClass := vars["rds_instance_class"]
	out := &configs.D8XConfig{}
	workers, _ := strconv.Atoi(vars["num_workers"])
	switch {
	case hasRDSClass:
		out.ServerProvider = configs.D8XServerProviderAWS
		out.AWSConfig = &configs.D8XAWSConfig{
			Region:             vars["region"],
			LabelPrefix:        vars["server_label_prefix"],
			NumWorker:          workers,
			RDSInstanceClass:   vars["rds_instance_class"],
			CreateBrokerServer: vars["create_broker_server"] == "true",
			DeploySwarm:        vars["create_swarm"] == "true",
		}
	case hasBrokerSize || vars["region"] != "":
		out.ServerProvider = configs.D8XServerProviderLinode
		out.LinodeConfig = &configs.D8XLinodeConfig{
			Region:             vars["region"],
			LabelPrefix:        vars["server_label_prefix"],
			NumWorker:          workers,
			BrokerServerSize:   vars["broker_size"],
			CreateBrokerServer: vars["create_broker_server"] == "true",
			DeploySwarm:        vars["create_swarm"] == "true",
		}
	default:
		return nil
	}
	return out
}

func parseTfvars(content string) map[string]string {
	return parseEnvBytes([]byte(content))
}

func (c *Container) RequireProvisionedHosts(cmd string, roles ...string) error {
	if c.HostsCfg == nil {
		return fmt.Errorf("env %q has no hosts.cfg loaded — \"d8x setup %s\" needs provisioned servers. Run \"d8x setup provision\" first", c.SelectedEnv, cmd)
	}
	if len(roles) == 0 {
		roles = []string{"manager"}
	}
	for _, role := range roles {
		switch role {
		case "manager":
			if _, err := c.HostsCfg.GetMangerPublicIp(); err != nil {
				return fmt.Errorf("env %q has no manager IP in hosts.cfg — \"d8x setup %s\" needs provisioned servers. Run \"d8x setup provision\" first", c.SelectedEnv, cmd)
			}
		case "broker":
			if _, err := c.HostsCfg.GetBrokerPublicIp(); err != nil {
				return fmt.Errorf("env %q has no broker IP in hosts.cfg — \"d8x setup %s\" needs a provisioned broker server. Run \"d8x setup provision\" first", c.SelectedEnv, cmd)
			}
		}
	}
	return nil
}

func (c *Container) PublishRemoteConfig(cfg *configs.D8XConfig) error {
	return c.PublishRemoteConfigWithSource(cfg, "")
}

func (c *Container) PublishRemoteTfvars(cfg *configs.D8XConfig, source string) error {
	if c.SelectedEnv == "" {
		return nil
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN missing, cannot publish terraform.tfvars")
	}
	content := buildTfvars(cfg)
	if content == "" {
		return fmt.Errorf("cannot build terraform.tfvars: server_provider=%q with %s config missing", cfg.ServerProvider, cfg.ServerProvider)
	}
	path := c.SelectedEnv + "/terraform.tfvars"
	sha := ""
	if existing, err := ghReadFile(token, path); err == nil {
		sha = existing.SHA
		if existing.Content == content {
			return nil
		}
	}
	msg := "update " + path
	if source != "" {
		msg = fmt.Sprintf("update %s (modified during %s)", path, source)
	}
	if _, err := ghWriteFile(token, path, content, sha, msg); err != nil {
		return fmt.Errorf("pushing %s to infra repo: %w", path, err)
	}
	fmt.Printf("%s pushed %s to infra repo\n", ok, path)
	return nil
}

func buildTfvars(cfg *configs.D8XConfig) string {
	switch cfg.ServerProvider {
	case configs.D8XServerProviderLinode:
		if cfg.LinodeConfig == nil {
			return ""
		}
		brokerSize := "g6-dedicated-2"
		if cfg.LinodeConfig.BrokerServerSize != "" {
			brokerSize = cfg.LinodeConfig.BrokerServerSize
		}
		return fmt.Sprintf(`region               = "%s"
num_workers          = %d
broker_size          = "%s"
server_label_prefix  = "%s"
create_broker_server = %t
create_swarm         = %t
`,
			cfg.LinodeConfig.Region,
			cfg.LinodeConfig.NumWorker,
			brokerSize,
			cfg.LinodeConfig.LabelPrefix,
			cfg.LinodeConfig.CreateBrokerServer,
			cfg.LinodeConfig.DeploySwarm,
		)
	case configs.D8XServerProviderAWS:
		if cfg.AWSConfig == nil {
			return ""
		}
		rdsClass := "db.t4g.small"
		if cfg.AWSConfig.RDSInstanceClass != "" {
			rdsClass = cfg.AWSConfig.RDSInstanceClass
		}
		return fmt.Sprintf(`region               = "%s"
server_label_prefix  = "%s"
num_workers          = %d
create_broker_server = %t
create_swarm         = %t
rds_instance_class   = "%s"
`,
			cfg.AWSConfig.Region,
			cfg.AWSConfig.LabelPrefix,
			cfg.AWSConfig.NumWorker,
			cfg.AWSConfig.CreateBrokerServer,
			cfg.AWSConfig.DeploySwarm,
			rdsClass,
		)
	}
	return ""
}

func (c *Container) PublishRemoteConfigWithSource(cfg *configs.D8XConfig, source string) error {
	if c.SelectedEnv == "" {
		return nil
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN missing, cannot publish remote config")
	}

	public := *cfg
	public.SwarmRedisPassword = ""
	public.DatabaseDSN = ""
	public.BrokerServerConfig.RedisPassword = ""
	public.HttpRpcList = nil
	public.WsRpcList = nil
	public.UserSuppliedPriceFeedEndpoints = nil
	if public.LinodeConfig != nil {
		lc := *public.LinodeConfig
		lc.Token = ""
		public.LinodeConfig = &lc
	}
	if public.AWSConfig != nil {
		aws := *public.AWSConfig
		aws.AccesKey = ""
		aws.SecretKey = ""
		aws.RDSCredentialsFilePath = ""
		public.AWSConfig = &aws
	}

	data, err := json.MarshalIndent(public, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling public config: %w", err)
	}
	path := c.SelectedEnv + "/config.json"
	sha := ""
	if existing, err := ghReadFile(token, path); err == nil {
		sha = existing.SHA
		if existing.Content == string(data) {
			fmt.Printf("%s %s already up to date in infra repo\n", ok, path)
			return nil
		}
	}
	msg := "update " + path
	if source != "" {
		msg = fmt.Sprintf("update %s (modified during %s)", path, source)
	}
	if _, err := ghWriteFile(token, path, string(data), sha, msg); err != nil {
		return fmt.Errorf("pushing %s to infra repo: %w", path, err)
	}
	fmt.Printf("%s pushed sanitized config to infra repo (%s)\n", ok, path)
	return nil
}

func (c *Container) ensureSSHKey(env string) error {
	upperEnv := strings.ToUpper(env)
	fieldName := "SSH_KEY_" + upperEnv
	sshKey := os.Getenv(fieldName)
	if sshKey != "" {
		if _, statErr := os.Stat(sshKey); statErr != nil {
			fmt.Printf("%s SSH_KEY_%s pointed at %s but the file is missing; re-staging from Bitwarden.\n", warning, upperEnv, sshKey)
			sshKey = ""
			os.Unsetenv(fieldName)
		}
	}
	if sshKey == "" && c.BitwardenFields != nil {
		if content, exists := c.BitwardenFields[fieldName]; exists && content != "" {
			staged, werr := writeSSHKeyToTempFile(fieldName, content)
			if werr != nil {
				return fmt.Errorf("re-staging SSH key from Bitwarden: %w", werr)
			}
			os.Setenv(fieldName, staged)
			sshKey = staged
			fmt.Printf("%s re-staged SSH key from Bitwarden field %s\n", ok, fieldName)
		}
	}
	if sshKey == "" {
		bootPath, berr := c.bootstrapSSHKey(env)
		if berr != nil {
			return berr
		}
		sshKey = bootPath
	}
	keyContent, err := os.ReadFile(sshKey)
	if err != nil {
		return fmt.Errorf("SSH key not found at %s", sshKey)
	}
	if !strings.Contains(string(keyContent), "PRIVATE KEY") {
		return fmt.Errorf("file %s does not look like a valid SSH private key", sshKey)
	}
	c.SshKeyPath = sshKey
	if c.Input != nil {
		c.Input.SSHKeyPath = sshKey
	}

	if os.Getenv("BW_SESSION") != "" && os.Getenv(fieldName) == "" {
		result, _, serr := SaveSecretToBitwardenItem(bwItemName, fieldName, string(keyContent))
		switch {
		case serr != nil:
			fmt.Printf("%s warning: could not save SSH key to Bitwarden (%s): %s\n", warning, fieldName, serr)
		case result == BwSkippedConflict:
			fmt.Printf("%s warning: %s already exists in Bitwarden with a different value. Local key not synced. Run \"bw edit\" manually or rotate the key.\n", warning, fieldName)
		default:
			os.Setenv(fieldName, sshKey)
			fmt.Printf("%s uploaded SSH key to Bitwarden as %s\n", ok, fieldName)
		}
	}
	return nil
}

func (c *Container) bootstrapSSHKey(env string) (string, error) {
	if os.Getenv("BW_SESSION") == "" {
		return "", fmt.Errorf("BW_SESSION not set. Unlock Bitwarden first (bw unlock) to bootstrap an SSH key for env %q", env)
	}

	upperEnv := strings.ToUpper(env)
	fieldName := "SSH_KEY_" + upperEnv

	fmt.Printf("%s no SSH key registered for env %q (Bitwarden field %s missing).\n", notok, env, fieldName)

	choices := []string{
		"Generate a new ed25519 keypair and upload to Bitwarden",
		"Import an existing private key from a local file (uploaded once, never read again)",
	}
	picked, err := c.TUI.NewSelection(
		choices,
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)
	if err != nil {
		return "", err
	}

	tempPath := filepath.Join(os.TempDir(), "d8x-cli", fieldName)
	if err := os.MkdirAll(filepath.Dir(tempPath), 0700); err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	if picked[0] == choices[0] {
		_ = os.Remove(tempPath)
		_ = os.Remove(tempPath + ".pub")
		keygen := fmt.Sprintf("yes | ssh-keygen -N \"\" -t ed25519 -C d8x-%s -f %s", env, tempPath)
		cmd := exec.Command("bash", "-c", keygen)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if rerr := c.RunCmd(cmd); rerr != nil {
			return "", fmt.Errorf("ssh-keygen failed: %w", rerr)
		}
		fmt.Printf("%s generated ed25519 keypair at %s (+ .pub)\n", ok, tempPath)
	} else {
		fmt.Println("Enter path to existing SSH private key (will be read once and uploaded to Bitwarden):")
		path, perr := c.TUI.NewInput(components.TextInputOptDenyEmpty())
		if perr != nil {
			return "", perr
		}
		path = strings.TrimSpace(path)
		if strings.HasPrefix(path, "~/") {
			home, herr := os.UserHomeDir()
			if herr != nil {
				return "", fmt.Errorf("resolving home dir: %w", herr)
			}
			path = filepath.Join(home, path[2:])
		}
		content, rerr := os.ReadFile(path)
		if rerr != nil {
			return "", fmt.Errorf("reading %s: %w", path, rerr)
		}
		if !strings.Contains(string(content), "PRIVATE KEY") {
			return "", fmt.Errorf("%s does not look like a valid SSH private key", path)
		}
		if werr := os.WriteFile(tempPath, content, 0600); werr != nil {
			return "", fmt.Errorf("staging SSH key at %s: %w", tempPath, werr)
		}
		if out, kerr := exec.Command("ssh-keygen", "-y", "-f", tempPath).Output(); kerr == nil {
			_ = os.WriteFile(tempPath+".pub", out, 0644)
		}
		fmt.Printf("%s imported SSH key from %s into %s\n", ok, path, tempPath)
	}

	keyContent, rerr := os.ReadFile(tempPath)
	if rerr != nil {
		return "", fmt.Errorf("reading staged SSH key: %w", rerr)
	}
	result, _, serr := SaveSecretToBitwardenItem(bwItemName, fieldName, string(keyContent))
	if serr != nil {
		return "", fmt.Errorf("uploading SSH key to Bitwarden (%s): %w", fieldName, serr)
	}
	if result == BwSkippedConflict {
		return "", fmt.Errorf("%s already exists in Bitwarden with a different value. Either remove it via \"bw edit\" or reuse the existing key", fieldName)
	}
	os.Setenv(fieldName, tempPath)
	fmt.Printf("%s uploaded %s to Bitwarden item %q\n", ok, fieldName, bwItemName)
	return tempPath, nil
}

func indexOf(list []string, item string) int {
	for i, v := range list {
		if v == item {
			return i
		}
	}
	return 0
}
