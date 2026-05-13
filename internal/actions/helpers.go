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
	"github.com/D8-X/d8x-cli/internal/flags"
	"github.com/D8-X/d8x-cli/internal/styles"
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

func (c *Container) DisplayPasswordAlert() {
	if len(c.UserPassword) == 0 {
		return
	}
	fmt.Printf("User: %s\n", c.DefaultClusterUserName)
	fmt.Printf("Password: %s\n", c.UserPassword)
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

func ensureWorkDir(rel string) (string, error) {
	path := workPath(rel)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	return path, nil
}

type infraRepoFile struct {
	envRelPath  string
	embeddedSrc string
}

var infraRepoManagedFiles = []infraRepoFile{
	{"trader-backend/rpc.main.json", "embedded/trader-backend/rpc.main.json"},
	{"trader-backend/rpc.history.json", "embedded/trader-backend/rpc.history.json"},
	{"candles/prices.config.json", "embedded/candles/prices.config.json"},
	{"candles/rpc_conf.json", "embedded/candles/rpc_conf.json"},
	{"docker-swarm-stack.yml", "embedded/docker-swarm-stack.yml"},
	{"docker-swarm-metrics.yml", "embedded/docker-swarm-metrics.yml"},
	{"prometheus.yml", "embedded/prometheus.yml"},
	{"grafana/datasource-prometheus.yml", "embedded/grafana/datasource-prometheus.yml"},
	{"grafana/chart.json", "embedded/grafana/chart.json"},
	{"grafana/chart-cadvisor.json", "embedded/grafana/chart-cadvisor.json"},
	{"grafana/dashboards.yml", "embedded/grafana/dashboards.yml"},
	{"broker-server/rpc.json", "embedded/broker-server/rpc.json"},
	{"broker-server/chainConfig.json", "embedded/broker-server/chainConfig.json"},
	{"broker-server/docker-compose.yml", "embedded/broker-server/docker-compose.yml"},
}

func (c *Container) loadInfraRepoFile(envRelPath, embeddedSrc string) ([]byte, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token != "" && c.SelectedEnv != "" {
		f, err := ghReadFile(token, c.SelectedEnv+"/"+envRelPath)
		if err == nil {
			return []byte(f.Content), nil
		}
		if !strings.Contains(err.Error(), "404") {
			fmt.Printf("  %s could not fetch %s/%s from infra repo (%s); using embedded fallback\n", warning, c.SelectedEnv, envRelPath, err)
		}
	}
	data, err := configs.EmbededConfigs.ReadFile(embeddedSrc)
	if err != nil {
		return nil, fmt.Errorf("reading embedded %s: %w", embeddedSrc, err)
	}
	return data, nil
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
	return os.WriteFile(localPath, content, 0644)
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
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// CollectAndValidatePrivateKey prompts user to enter a private key, validates
// it, displays the address of entered key and prompts user to confirm that
// entered key's address is correct. If any of the validation or
// confirmation steps fail, it will restart the collection process. Returned
// values are private key without 0x prefix and its address.
func (c *Container) CollectAndValidatePrivateKey(title string) (string, string, error) {
	fmt.Println(title)
	pk, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
		components.TextInputOptMasked(),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return "", "", err
	}
	pk = strings.TrimPrefix(pk, "0x")
	addr, err := PrivateKeyToAddress(pk)
	if err != nil {
		info := fmt.Sprintf("Invalid private key, please try again...\n - %s\n", err.Error())
		fmt.Println(styles.ErrorText.Render(info))
		return c.CollectAndValidatePrivateKey(title)
	}

	fmt.Printf("Wallet address of entered private key: %s\n", addr.Hex())

	ok, err := c.TUI.NewPrompt("Is this the correct address?", true)
	if err != nil {
		return "", "", err
	}

	if !ok {
		return c.CollectAndValidatePrivateKey(title)
	}

	return pk, addr.Hex(), nil
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

func UpdateConfig[Target any](configFilePath string, updateFn func(*Target) error) error {
	contents, err := os.ReadFile(configFilePath)
	if err != nil {
		return err
	}
	out, err := UpdateConfigBytes(contents, updateFn)
	if err != nil {
		return err
	}
	return os.WriteFile(configFilePath, out, 0644)
}
