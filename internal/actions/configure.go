package actions

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/bcrypt"
)

// Configure performs initials hosts setup and configuration with ansible
func (c *Container) Configure(ctx *cli.Context) error {
	styles.PrintCommandTitle("Performing servers setup configuration with ansible...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureEnvironment(cfg); err != nil {
		return err
	}
	if cfg.ServerProvider == "" {
		return fmt.Errorf("server_provider is empty in this env's config.json on the infra repo; set it to \"linode\" or \"aws\" there, or run \"d8x setup provision\" which sets it for new envs")
	}

	// Update hosts.cfg for linode provider in case d8x config was changed
	// manually
	if cfg.ServerProvider == configs.D8XServerProviderLinode {
		if err := c.LinodeInventorySetUserVar(cfg.ConfigDetails.ConfiguredServers, c.DefaultClusterUserName); err != nil {
			return fmt.Errorf("updating linode inventory file: %w", err)
		}
	}

	playbookPath, err := c.fetchSetupPlaybook()
	if err != nil {
		return err
	}

	pubKey, err := getPublicKey(c.SshKeyPath)
	if err != nil {
		return fmt.Errorf("retrieving public key: %w", err)
	}
	privKeyPath := c.SshKeyPath

	// Generate password when not provided
	if c.UserPassword == "" {
		password, err := generatePassword(16)
		if err != nil {
			return err
		}
		c.UserPassword = password
	}

	if os.Getenv("BW_SESSION") != "" && c.SelectedEnv != "" {
		fieldName := "SERVER_PASSWORD_" + strings.ToUpper(c.SelectedEnv)
		result, _, err := SaveSecretToBitwardenItem(bwItemName, fieldName, c.UserPassword)
		switch {
		case err != nil:
			fmt.Printf("  %s could not save %s to Bitwarden: %s\n", notok, fieldName, err)
			fmt.Printf("  %s server password (capture now, Bitwarden save failed): %s\n", warning, c.UserPassword)
		case result == BwSkippedConflict:
			fmt.Printf("  %s %s already exists in Bitwarden with a different value; not overwriting. This run is using a freshly generated password: %s\n", warning, fieldName, c.UserPassword)
		case result == BwSaved:
			fmt.Printf("  %s server password saved to Bitwarden as %s\n", ok, fieldName)
		case result == BwUnchanged:
			fmt.Printf("  %s server password already in Bitwarden as %s (reused)\n", ok, fieldName)
		}
	} else {
		fmt.Printf("  %s BW_SESSION not set; server password not saved to Bitwarden. Capture it now: %s\n", warning, c.UserPassword)
	}

	configureUser := cfg.GetAnsibleUser()

	// For linode when subsequent configuration is performed, we need to use the
	// cluster user and provide become_pass for old servers, but new servers
	// need root.

	// Hash password for ansible
	h, err := bcrypt.GenerateFromPassword([]byte(c.UserPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("generating hashed password: %w", err)
	}
	hashedPassword := string(h)

	inventoryPath, err := writeHostsToTempFile(c.HostsCfg)
	if err != nil {
		return fmt.Errorf("writing ansible inventory: %w", err)
	}

	args := []string{
		"--extra-vars", fmt.Sprintf(`ansible_ssh_private_key_file='%s'`, privKeyPath),
		"--extra-vars", "ansible_host_key_checking=false",
		"--extra-vars", fmt.Sprintf(`user_public_key='%s'`, pubKey),
		"--extra-vars", fmt.Sprintf(`default_user_name=%s`, c.DefaultClusterUserName),
		"--extra-vars", fmt.Sprintf(`default_user_password='%s'`, hashedPassword),
		"-i", inventoryPath,
		"-u", configureUser,
		playbookPath,
	}

	switch cfg.ServerProvider {
	case configs.D8XServerProviderAWS:
		// For AWS, we don't want to setup UFW, since firewall is already handled by
		// AWS itself
		args = append(args, "--extra-vars", "no_ufw=true")

	case configs.D8XServerProviderLinode:
		// For linode - pass become_pass for subsequent configuration runs.
		if cfg.ConfigDetails.Done {
			args = append(args,
				"--extra-vars", fmt.Sprintf(`ansible_become_pass='%s'`, c.UserPassword),
			)
		}
	}

	cmd := exec.Command("ansible-playbook", args...)
	cmd.Env = os.Environ()
	connectCMDToCurrentTerm(cmd)

	if err := c.RunCmd(cmd); err != nil {
		return err
	}

	// Update configuration details
	cfg.ConfigDetails.Done = true
	cfg.ConfigDetails.ConfiguredServers = c.HostsCfg.GetAllPublicIps()

	// Update hosts.cfg for linode provider
	if cfg.ServerProvider == configs.D8XServerProviderLinode {
		if err := c.LinodeInventorySetUserVar(cfg.ConfigDetails.ConfiguredServers, c.DefaultClusterUserName); err != nil {
			return fmt.Errorf("updating linode inventory file: %w", err)
		}
	}

	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return err
	}
	if err := c.PublishRemoteConfig(cfg); err != nil {
		fmt.Printf("  %s failed to sync remote config: %s\n", notok, err)
	}
	return nil
}

func (c *Container) fetchSetupPlaybook() (string, error) {
	var content []byte
	token := os.Getenv("GITHUB_TOKEN")
	if token != "" && c.SelectedEnv != "" {
		file, err := ghReadFile(token, c.SelectedEnv+"/setup.ansible.yaml")
		if err == nil {
			content = []byte(file.Content)
		} else if !strings.Contains(err.Error(), "404") {
			fmt.Printf("  %s could not fetch %s/setup.ansible.yaml from infra repo (%s); falling back to embedded playbook\n", warning, c.SelectedEnv, err)
		}
	}
	if content == nil {
		embedded, err := configs.GetSetupAnsiblePlaybook()
		if err != nil {
			return "", fmt.Errorf("loading embedded setup.ansible.yaml: %w", err)
		}
		content = embedded
	}
	dir := filepath.Join(os.TempDir(), "d8x-cli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating temp playbook dir: %w", err)
	}
	path := filepath.Join(dir, "setup.ansible.yaml")
	if err := os.WriteFile(path, content, 0644); err != nil {
		return "", fmt.Errorf("writing temp playbook: %w", err)
	}
	return path, nil
}

func generatePassword(n int) (string, error) {
	set := "_1234567890-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	l := int64(len(set))
	var pwd strings.Builder
	pwd.Grow(n)
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(l))
		if err != nil {
			return "", err
		}
		pwd.WriteByte(set[idx.Int64()])
	}
	return pwd.String(), nil
}
