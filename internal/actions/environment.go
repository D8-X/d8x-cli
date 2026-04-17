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

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return "", fmt.Errorf("GITHUB_TOKEN is required in .env file")
	}

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
			continue
		}
		var ec configs.D8XConfig
		if err := json.Unmarshal([]byte(cfgFile.Content), &ec); err != nil {
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
	mergeRemoteConfig(cfg, remoteCfg)
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return "", fmt.Errorf("writing config: %w", err)
	}

	// Fetch hosts.cfg from GitHub
	hostsPath := filepath.Join(c.ConfigDir, env+"-hosts.cfg")
	hostsFile, err := ghReadFile(token, env+"/hosts.cfg")
	if err != nil {
		return "", fmt.Errorf("could not fetch hosts.cfg for %s from GitHub: %w", env, err)
	}
	os.MkdirAll(filepath.Dir(hostsPath), 0755)
	if err := os.WriteFile(hostsPath, []byte(hostsFile.Content), 0644); err != nil {
		return "", fmt.Errorf("writing hosts.cfg: %w", err)
	}
	c.HostsCfg = files.NewFSHostsFileInteractor(hostsPath)

	// SSH key: check SSH_KEY_{ENV} (from Bitwarden) then SSH_KEY_PATH_{ENV} (from .env)
	upperEnv := strings.ToUpper(env)
	sshKey := os.Getenv("SSH_KEY_" + upperEnv)
	if sshKey == "" {
		sshKey = os.Getenv("SSH_KEY_PATH_" + upperEnv)
	}
	envKey := "SSH_KEY_PATH_" + upperEnv
	if sshKey == "" {
		fmt.Printf("%s not found in .env. Enter SSH key path for %s:\n", envKey, env)
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

func mergeRemoteConfig(cfg, remoteCfg *configs.D8XConfig) {
	if remoteCfg == nil {
		return
	}

	if remoteCfg.SwarmRedisPassword != "" || remoteCfg.DatabaseDSN != "" || remoteCfg.BrokerServerConfig.RedisPassword != "" {
		fmt.Printf("%s remote config.json contains secret fields - ignoring them. Keep secrets in Bitwarden or local .env.\n", notok)
	}

	var merged []string

	if remoteCfg.ServerProvider != "" {
		cfg.ServerProvider = remoteCfg.ServerProvider
		merged = append(merged, "server_provider")
	}
	if remoteCfg.LinodeConfig != nil {
		cfg.LinodeConfig = remoteCfg.LinodeConfig
		merged = append(merged, "linode_config")
	}
	if remoteCfg.AWSConfig != nil {
		cfg.AWSConfig = remoteCfg.AWSConfig
		merged = append(merged, "aws_config")
	}
	if remoteCfg.ChainId != 0 {
		cfg.ChainId = remoteCfg.ChainId
		merged = append(merged, "chain_id")
	}
	if remoteCfg.CertbotEmail != "" {
		cfg.CertbotEmail = remoteCfg.CertbotEmail
		merged = append(merged, "certbot_email")
	}
	if remoteCfg.SwarmRemoteBrokerHTTPUrl != "" {
		cfg.SwarmRemoteBrokerHTTPUrl = remoteCfg.SwarmRemoteBrokerHTTPUrl
		merged = append(merged, "swarm_remote_broker_http_url")
	}
	if len(remoteCfg.UserSuppliedPriceFeedEndpoints) > 0 {
		cfg.UserSuppliedPriceFeedEndpoints = remoteCfg.UserSuppliedPriceFeedEndpoints
		merged = append(merged, "user_supplied_price_feed_endpoints")
	}
	if remoteCfg.BrokerServerConfig.FeeTBPS != "" {
		cfg.BrokerServerConfig.FeeTBPS = remoteCfg.BrokerServerConfig.FeeTBPS
		merged = append(merged, "broker_server_config.fee_tbps")
	}
	if remoteCfg.BrokerServerConfig.FeeInputPercent != "" {
		cfg.BrokerServerConfig.FeeInputPercent = remoteCfg.BrokerServerConfig.FeeInputPercent
		merged = append(merged, "broker_server_config.fee_input_percent")
	}

	if cfg.Services == nil {
		cfg.Services = make(map[configs.D8XServiceName]configs.D8XService)
	}
	for name, service := range remoteCfg.Services {
		cfg.Services[name] = service
	}
	if len(remoteCfg.Services) > 0 {
		merged = append(merged, "services")
	}

	if cfg.HttpRpcList == nil {
		cfg.HttpRpcList = make(map[string][]string)
	}
	for chainID, rpcs := range remoteCfg.HttpRpcList {
		cfg.HttpRpcList[chainID] = rpcs
	}
	if len(remoteCfg.HttpRpcList) > 0 {
		merged = append(merged, "http_rpc_list")
	}

	if cfg.WsRpcList == nil {
		cfg.WsRpcList = make(map[string][]string)
	}
	for chainID, rpcs := range remoteCfg.WsRpcList {
		cfg.WsRpcList[chainID] = rpcs
	}
	if len(remoteCfg.WsRpcList) > 0 {
		merged = append(merged, "ws_rpc_list")
	}

	if len(merged) > 0 {
		fmt.Printf("%s merged fields from remote config.json: %s\n", ok, strings.Join(merged, ", "))
	}
}

func indexOf(list []string, item string) int {
	for i, v := range list {
		if v == item {
			return i
		}
	}
	return 0
}
