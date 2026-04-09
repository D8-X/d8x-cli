package actions

import (
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
		return "", fmt.Errorf("listing environments from GitHub: %w", err)
	}

	var environments []string
	var labels []string
	for _, e := range allDirs {
		sites, err := ghReadFile(token, e+"/sites.conf")
		if err != nil {
			continue
		}
		environments = append(environments, e)
		label := e
		if chainID := extractChainID(sites.Content); chainID != "" {
			label = fmt.Sprintf("%s  (chain %s)", e, chainID)
		}
		labels = append(labels, label)
	}
	if len(environments) == 0 {
		return "", fmt.Errorf("no environments found in %s repo", ghRepo)
	}

	fmt.Println(styles.ItalicText.Render("Select environment:"))
	selected, err := c.TUI.NewSelection(labels, components.SelectionOptAllowOnlySingleItem(), components.SelectionOptRequireSelection())
	if err != nil {
		return "", err
	}
	env := environments[indexOf(labels, selected[0])]
	fmt.Printf("Environment: %s\n", env)

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

	// SSH key from .env or prompt
	envKey := "SSH_KEY_PATH_" + strings.ToUpper(env)
	sshKey := os.Getenv(envKey)
	if sshKey == "" {
		fmt.Printf("%s not found in .env. Enter SSH key path for %s:\n", envKey, env)
		sshKey, err = c.TUI.NewInput(components.TextInputOptValue("./id_ed25519"))
		if err != nil {
			return "", err
		}
	}
	if strings.HasPrefix(sshKey, "~/") {
		home, _ := os.UserHomeDir()
		sshKey = filepath.Join(home, sshKey[2:])
	}
	if _, err := os.Stat(sshKey); err != nil {
		return "", fmt.Errorf("SSH key not found at %s", sshKey)
	}
	c.SshKeyPath = sshKey

	// Password from .env
	pwdKey := "SERVER_PASSWORD_" + strings.ToUpper(env)
	if pwd := os.Getenv(pwdKey); pwd != "" {
		c.UserPassword = pwd
	}

	c.SelectedEnv = env
	return env, nil
}

// extractChainID parses the chain ID from the first server_name in sites.conf.
// e.g. "server_name api-8453.d8x.xyz;" -> "8453"
func extractChainID(sitesConf string) string {
	for _, line := range strings.Split(sitesConf, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "server_name ") {
			continue
		}
		name := strings.TrimPrefix(line, "server_name ")
		name = strings.TrimSuffix(name, ";")
		name = strings.TrimSpace(name)
		parts := strings.SplitN(name, "-", 2)
		if len(parts) < 2 {
			continue
		}
		id := ""
		for _, ch := range parts[1] {
			if ch >= '0' && ch <= '9' {
				id += string(ch)
			} else {
				break
			}
		}
		if id != "" {
			return id
		}
	}
	return ""
}

func indexOf(list []string, item string) int {
	for i, v := range list {
		if v == item {
			return i
		}
	}
	return 0
}
