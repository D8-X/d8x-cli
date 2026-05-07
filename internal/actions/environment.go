package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
)

// EnsureEnvironment selects an environment, fetches hosts.cfg from the GitHub
// repo, and configures SSH key and password for the selected environment.
func (c *Container) EnsureEnvironment(cfg *configs.D8XConfig) (string, error) {
	if c.SelectedEnv != "" {
		fmt.Printf("Environment: %s\n", c.SelectedEnv)
		return c.SelectedEnv, nil
	}

	if err := c.RequireBitwardenField("GITHUB_TOKEN"); err != nil {
		return "", err
	}
	token := os.Getenv("GITHUB_TOKEN")

	// List environments from GitHub and let user pick
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

		_, hostsErr := ghReadFile(token, e+"/hosts.cfg")
		provisioned := hostsErr == nil

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
		return "", fmt.Errorf("no environments found in %s repo", getGhRepo())
	}

	fmt.Println(styles.ItalicText.Render("Select environment:"))
	selected, err := c.TUI.NewSelection(labels, components.SelectionOptAllowOnlySingleItem(), components.SelectionOptRequireSelection())
	if err != nil {
		return "", err
	}
	idx := indexOf(labels, selected[0])
	env := environments[idx]
	fmt.Printf("Environment: %s\n", env)

	remoteCfg := &envConfigs[idx]
	loadRemoteConfig(cfg, remoteCfg)
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return "", fmt.Errorf("writing config: %w", err)
	}

	var hostsContent []byte
	var hostsSHA string
	hostsFile, err := ghReadFile(token, env+"/hosts.cfg")
	switch {
	case err == nil:
		hostsContent = []byte(hostsFile.Content)
		hostsSHA = hostsFile.SHA
	case strings.Contains(err.Error(), "404"):
		fmt.Printf("%s no remote hosts.cfg for '%s' yet — assuming first provision\n", notok, env)
	default:
		return "", fmt.Errorf("could not fetch hosts.cfg for %s from GitHub: %w", env, err)
	}
	hostsRemotePath := env + "/hosts.cfg"
	c.HostsCfg = files.NewMemHostsFileInteractor(hostsContent, func(content string) error {
		fmt.Printf("%s overwriting %s in infra repo\n", warning, hostsRemotePath)
		newSHA, werr := ghWriteFile(token, hostsRemotePath, content, hostsSHA, "update "+hostsRemotePath+" - d8x hosts update")
		if werr != nil {
			return fmt.Errorf("pushing hosts.cfg to infra repo: %w", werr)
		}
		hostsSHA = newSHA
		fmt.Printf("%s pushed %s to infra repo\n", ok, hostsRemotePath)
		return nil
	})

	// SSH key: check SSH_KEY_{ENV} (from Bitwarden) then SSH_KEY_PATH_{ENV} (from .env)
	upperEnv := strings.ToUpper(env)
	sshKey := os.Getenv("SSH_KEY_" + upperEnv)
	if sshKey == "" {
		sshKey = os.Getenv("SSH_KEY_PATH_" + upperEnv)
	}
	envKey := "SSH_KEY_PATH_" + upperEnv
	if sshKey == "" {
		fmt.Printf("%s not found in Bitwarden (field SSH_KEY_%s). Enter SSH key path for %s:\n", envKey, upperEnv, env)
		sshKey, err = c.TUI.NewInput(components.TextInputOptValue("./id_ed25519"))
		if err != nil {
			return "", err
		}
	}
	if strings.HasPrefix(sshKey, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not resolve home directory: %w", err)
		}
		sshKey = filepath.Join(home, sshKey[2:])
	}
	keyContent, err := os.ReadFile(sshKey)
	if err != nil {
		return "", fmt.Errorf("SSH key not found at %s", sshKey)
	}
	if !strings.Contains(string(keyContent), "PRIVATE KEY") {
		return "", fmt.Errorf("file %s does not look like a valid SSH private key", sshKey)
	}
	c.SshKeyPath = sshKey

	// Password from .env
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

	var (
		linodeToken                    string
		awsAccess, awsSecret, awsRDS   string
		swarmRedisPw                   = cfg.SwarmRedisPassword
		databaseDsn                    = cfg.DatabaseDSN
		brokerRedisPw                  = cfg.BrokerServerConfig.RedisPassword
		httpRpcList                    = cfg.HttpRpcList
		wsRpcList                      = cfg.WsRpcList
		userSuppliedPriceFeedEndpoints = cfg.UserSuppliedPriceFeedEndpoints
	)
	if cfg.LinodeConfig != nil {
		linodeToken = cfg.LinodeConfig.Token
	}
	if cfg.AWSConfig != nil {
		awsAccess = cfg.AWSConfig.AccesKey
		awsSecret = cfg.AWSConfig.SecretKey
		awsRDS = cfg.AWSConfig.RDSCredentialsFilePath
	}

	remoteCopy, err := json.Marshal(remoteCfg)
	if err == nil {
		var clone configs.D8XConfig
		if err := json.Unmarshal(remoteCopy, &clone); err == nil {
			*cfg = clone
		} else {
			*cfg = *remoteCfg
		}
	} else {
		*cfg = *remoteCfg
	}

	if cfg.LinodeConfig != nil && linodeToken != "" {
		cfg.LinodeConfig.Token = linodeToken
	}
	if cfg.AWSConfig != nil {
		if awsAccess != "" {
			cfg.AWSConfig.AccesKey = awsAccess
		}
		if awsSecret != "" {
			cfg.AWSConfig.SecretKey = awsSecret
		}
		if awsRDS != "" {
			cfg.AWSConfig.RDSCredentialsFilePath = awsRDS
		}
	}
	cfg.SwarmRedisPassword = swarmRedisPw
	cfg.DatabaseDSN = databaseDsn
	cfg.BrokerServerConfig.RedisPassword = brokerRedisPw
	if len(cfg.HttpRpcList) == 0 {
		cfg.HttpRpcList = httpRpcList
	}
	if len(cfg.WsRpcList) == 0 {
		cfg.WsRpcList = wsRpcList
	}
	if len(cfg.UserSuppliedPriceFeedEndpoints) == 0 {
		cfg.UserSuppliedPriceFeedEndpoints = userSuppliedPriceFeedEndpoints
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

	missing := []string{}
	if cfg.ServerProvider == "" {
		missing = append(missing, "server_provider")
	}
	if !cfg.SwarmDeployed {
		missing = append(missing, "swarm_deployed")
	}
	if !cfg.BrokerDeployed {
		missing = append(missing, "broker_deployed")
	}
	if cfg.ChainId == 0 {
		fmt.Printf("%s loaded env config from infra repo (no chain_id set in remote)\n", warning)
	} else {
		fmt.Printf("%s loaded env config from infra repo: chain_id=%d\n", ok, cfg.ChainId)
	}
	if len(missing) > 0 {
		fmt.Printf("%s remote config.json is missing or empty for: %s — run a deploy command to publish current state\n", warning, strings.Join(missing, ", "))
	}
}

func (c *Container) PublishRemoteConfig(cfg *configs.D8XConfig) error {
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
	}
	if _, err := ghWriteFile(token, path, string(data), sha, "update "+path+" - d8x config sync"); err != nil {
		return fmt.Errorf("pushing %s to infra repo: %w", path, err)
	}
	fmt.Printf("%s pushed sanitized config to infra repo (%s)\n", ok, path)
	return nil
}

func indexOf(list []string, item string) int {
	for i, v := range list {
		if v == item {
			return i
		}
	}
	return 0
}
