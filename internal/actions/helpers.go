package actions

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/styles"
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
		ok, err := components.NewPrompt(
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
