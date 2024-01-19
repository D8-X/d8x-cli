package actions

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/files"
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

	// Update hosts.cfg for linode provider in case d8x config was changed
	// manually
	if cfg.ServerProvider == configs.D8XServerProviderLinode {
		if err := c.LinodeInventorySetUserVar(cfg.ConfigDetails.ConfiguredServers, c.DefaultClusterUserName); err != nil {
			return fmt.Errorf("updating linode inventory file: %w", err)
		}
	}

	// Copy the playbooks file
	if err := c.EmbedCopier.Copy(
		configs.EmbededConfigs,
		files.EmbedCopierOp{Src: "embedded/playbooks/setup.ansible.yaml", Dst: "./playbooks/setup.ansible.yaml", Overwrite: true},
	); err != nil {
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

	// Prompt to save password
	c.DisplayPasswordAlert()
	// Legacy functionality to store password in txt
	if err := c.FS.WriteFile("./password.txt", []byte(c.UserPassword)); err != nil {
		return fmt.Errorf("storing password in ./password.txt file: %w", err)
	}
	fmt.Println(
		styles.SuccessText.Render("Password was stored in ./password.txt file"),
	)

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
	fmt.Printf("hashed user password: %s\n", hashedPassword)

	// Generate ansible-playbook args
	args := []string{
		"--extra-vars", fmt.Sprintf(`ansible_ssh_private_key_file='%s'`, privKeyPath),
		"--extra-vars", "ansible_host_key_checking=false",
		"--extra-vars", fmt.Sprintf(`user_public_key='%s'`, pubKey),
		"--extra-vars", fmt.Sprintf(`default_user_name=%s`, c.DefaultClusterUserName),
		"--extra-vars", fmt.Sprintf(`default_user_password='%s'`, hashedPassword),
		"-i", "./hosts.cfg",
		"-u", configureUser,
		"./playbooks/setup.ansible.yaml",
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

	return c.ConfigRWriter.Write(cfg)
}

func generatePassword(n int) (string, error) {
	set := "_1234567890-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	l := len(set)
	pwd := ""

	for i := 0; i < n; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(l)))
		if err != nil {
			return "", err
		}
		pwd += string(set[n.Int64()])
	}

	return pwd, nil

}
