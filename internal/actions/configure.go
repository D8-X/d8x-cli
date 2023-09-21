package actions

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os/exec"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

// Configure performs initials hosts setup and configuration with ansible
func (c *Container) Configure(ctx *cli.Context) error {
	styles.PrintCommandTitle("Performing servers setup configuration with ansible...")

	// Copy the playbooks file
	if err := c.EmbedCopier.Copy(
		configs.EmbededConfigs,
		files.EmbedCopierOp{Src: "embedded/playbooks/setup.ansible.yaml", Dst: "./playbooks/setup.ansible.yaml", Overwrite: true},
	); err != nil {
		return err
	}

	pubKey, err := c.getPublicKey()
	if err != nil {
		return fmt.Errorf("retrieving public key: %w", err)
	}
	privKeyPath := c.SshKeyPath

	// Generate password when not provided
	if c.UserPassword == "" {
		password, err := c.generatePassword(16)
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
		"-u", "root",
		"./playbooks/setup.ansible.yaml",
	}

	cmd := exec.Command("ansible-playbook", args...)
	connectCMDToCurrentTerm(cmd)

	return cmd.Run()
}

func (c *Container) generatePassword(n int) (string, error) {
	set := "@#%^*_+1234567890-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
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
