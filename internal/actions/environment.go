package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
		if hostsSHA != "" && string(hostsContent) == content {
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
		if existing.Content == string(data) {
			fmt.Printf("%s %s already up to date in infra repo\n", ok, path)
			return nil
		}
	}
	if _, err := ghWriteFile(token, path, string(data), sha, "update "+path); err != nil {
		return fmt.Errorf("pushing %s to infra repo: %w", path, err)
	}
	fmt.Printf("%s pushed sanitized config to infra repo (%s)\n", ok, path)
	return nil
}

func (c *Container) ensureSSHKey(env string) error {
	upperEnv := strings.ToUpper(env)
	sshKey := os.Getenv("SSH_KEY_" + upperEnv)
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

	fieldName := "SSH_KEY_" + upperEnv
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
