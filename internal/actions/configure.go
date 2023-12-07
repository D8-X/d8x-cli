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
)

// Configure performs initials hosts setup and configuration with ansible
func (c *Container) Configure(ctx *cli.Context) error {
	styles.PrintCommandTitle("Performing servers setup configuration with ansible...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
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

	// Generate ansible-playbook args
	args := []string{
		"--extra-vars", fmt.Sprintf(`ansible_ssh_private_key_file='%s'`, privKeyPath),
		"--extra-vars", "ansible_host_key_checking=false",
		"--extra-vars", fmt.Sprintf(`user_public_key='%s'`, pubKey),
		"--extra-vars", fmt.Sprintf(`default_user_name=%s`, c.DefaultClusterUserName),
		"--extra-vars", fmt.Sprintf(`default_user_password='%s'`, c.UserPassword),
		"-i", "./hosts.cfg",
		"-u", cfg.GetAnsibleUser(),
		"./playbooks/setup.ansible.yaml",
	}

	// For AWS, we don't want to setup UFW, since firewall is already handled by
	// AWS itself
	if cfg.ServerProvider == configs.D8XServerProviderAWS {
		args = append(args, "--extra-vars", "no_ufw=true")
	}

	cmd := exec.Command("ansible-playbook", args...)
	cmd.Env = os.Environ()
	connectCMDToCurrentTerm(cmd)

	return c.RunCmd(cmd)
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
