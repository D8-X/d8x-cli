package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/flags"
	"github.com/urfave/cli/v2"
)

// getPublicKey returns the public key contents
func getPublicKey(sshKeyPath string) (string, error) {
	pubkeyfile := fmt.Sprintf("%s.pub", sshKeyPath)
	pub, err := os.ReadFile(pubkeyfile)
	if err != nil {
		return "", fmt.Errorf("reading public key %s: %w", pubkeyfile, err)
	}
	return strings.TrimSpace(string(pub)), nil
}

func defaultPasswordGetter(ctx *cli.Context) (string, error) {
	if pwd := ctx.String(flags.Password); pwd != "" {
		return pwd, nil
	}
	return "", fmt.Errorf("password not found — set SERVER_PASSWORD_{ENV} in Bitwarden or use --password flag")
}

func (c *Container) ResolvePassword(ctx *cli.Context) (string, error) {
	if c.UserPassword != "" {
		return c.UserPassword, nil
	}
	return c.GetPassword(ctx)
}

// TrimHttpsPrefix removes http:// or https:// prefix from the url
func TrimHttpsPrefix(url string) string {
	return strings.TrimSpace(strings.TrimPrefix(
		strings.TrimPrefix(url, "http://"),
		"https://",
	))
}

// EnsureHttpsPrefixExists makes sure the url has https:// prefix
func EnsureHttpsPrefixExists(url string) string {
	return "https://" + TrimHttpsPrefix(url)
}

// ValidateHttp validates if given url starts with http:// or https://
func ValidateHttp(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// ValidateWs validates if given url starts with ws:// or wss://
func ValidateWs(url string) bool {
	return strings.HasPrefix(url, "ws://") || strings.HasPrefix(url, "wss://")
}

func readEnvSecret(env, base string) string {
	if env != "" {
		if v := os.Getenv(base + "_" + strings.ToUpper(env)); v != "" {
			return v
		}
	}
	if v := os.Getenv(base); v != "" {
		return v
	}
	return ""
}

func envSuffixedField(env, base string) string {
	if env == "" {
		return base
	}
	return base + "_" + strings.ToUpper(env)
}

func workPath(rel string) string {
	return filepath.Join(os.TempDir(), "d8x-cli", rel)
}

func (c *Container) envWorkDir() string {
	env := c.SelectedEnv
	if env == "" {
		env = "default"
	}
	return filepath.Join(os.TempDir(), "d8x-cli", env)
}

func (c *Container) tfDir() string {
	return filepath.Join(c.envWorkDir(), "terraform")
}

func (c *Container) hostsCfgPath() string {
	return filepath.Join(c.envWorkDir(), "hosts.cfg")
}

func ensureWorkDir(rel string) (string, error) {
	path := workPath(rel)
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		if err := os.Remove(dir); err != nil {
			return "", fmt.Errorf("removing stale file at %s: %w", dir, err)
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return path, nil
}

func (c *Container) loadInfraRepoFile(envRelPath, embeddedSrc string) ([]byte, error) {
	data, _, err := c.loadInfraRepoFileWithSource(envRelPath, embeddedSrc)
	return data, err
}

func (c *Container) loadInfraRepoFileWithSource(envRelPath, embeddedSrc string) ([]byte, bool, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token != "" && c.SelectedEnv != "" {
		f, err := ghReadFile(token, c.SelectedEnv+"/"+envRelPath)
		if err == nil {
			return []byte(f.Content), true, nil
		}
	}
	data, err := configs.EmbededConfigs.ReadFile(embeddedSrc)
	if err != nil {
		return nil, false, fmt.Errorf("reading embedded %s: %w", embeddedSrc, err)
	}
	return data, false, nil
}

func (c *Container) loadOptionalInfraRepoFile(envRelPath string) ([]byte, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" || c.SelectedEnv == "" {
		return nil, nil
	}
	f, err := ghReadFile(token, c.SelectedEnv+"/"+envRelPath)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, nil
		}
		return nil, err
	}
	return []byte(f.Content), nil
}

func (c *Container) stageInfraRepoFile(envRelPath, embeddedSrc, localPath string) error {
	content, err := c.loadInfraRepoFile(envRelPath, embeddedSrc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return err
	}
	return os.WriteFile(localPath, content, 0600)
}

func writeHostsToTempFile(h files.HostsFileInteractor) (string, error) {
	lines, err := h.GetLines()
	if err != nil {
		return "", err
	}
	path, err := ensureWorkDir("hosts.cfg")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func UpdateConfigBytes[Target any](contents []byte, updateFn func(*Target) error) ([]byte, error) {
	target := new(Target)
	if err := json.Unmarshal(contents, &target); err != nil {
		return nil, err
	}
	if err := updateFn(target); err != nil {
		return contents, nil
	}
	return json.MarshalIndent(target, "", "  ")
}
