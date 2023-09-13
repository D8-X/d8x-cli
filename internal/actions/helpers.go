package actions

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/flags"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

// ensureSSHKeyPresent prompts user to create or override new ssh key pair in
// default c.SshKeyPair location
func (c *Container) ensureSSHKeyPresent() error {
	// By default, we assume key exists, if it doesn't - we will create it
	// without prompting for users's constent, otherwise we prompt for consent.
	createKey := false
	_, err := os.Stat(c.SshKeyPath)
	if err != nil {
		fmt.Printf("SSH key %s was not found, creating new one...\n", c.SshKeyPath)
		createKey = true
	} else {
		ok, err := c.TUI.NewPrompt(
			fmt.Sprintf("SSH key %s was found, do you want to overwrite it with a new one?", c.SshKeyPath),
			true,
		)
		if err != nil {
			return err
		}

		if ok {
			createKey = true
		}
	}

	if createKey {
		fmt.Println(
			"Executing:",
			styles.ItalicText.Render(
				fmt.Sprintf("ssh-keygen -t ed25519 -f %s", c.SshKeyPath),
			),
		)
		cmd := exec.Command("ssh-keygen", "-N", "", "-t", "ed25519", "-f", c.SshKeyPath, "-C", "")
		connectCMDToCurrentTerm(cmd)
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

// getPublicKey returns the public key contents
func (c *Container) getPublicKey() (string, error) {
	pubkeyfile := fmt.Sprintf("%s.pub", c.SshKeyPath)
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

	fmt.Println(styles.AlertImportant.Render(`Make sure to securely store default user password! This password will be
	created for default user on each provisioned server.`))
	fmt.Printf("User: %s\n", c.DefaultClusterUserName)
	fmt.Printf("Password: %s\n", c.UserPassword)

	c.TUI.NewConfirmation("Please confirm that you have stored the password!")
}

// Get password gets the password with the following precedence:
// 1. --password flag
// 2. ./password.txt file in cwd
func defaultPasswordGetter(ctx *cli.Context) (string, error) {
	if pwd := ctx.String(flags.Password); pwd != "" {
		return pwd, nil
	}
	if pwd, err := os.ReadFile(configs.DEFAULT_PASSWORD_FILE); err != nil {
		return "", fmt.Errorf("could not retrieve the password: %w", err)
	} else {
		return string(pwd), nil
	}
}
