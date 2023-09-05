package actions

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/configs"
)

// Container is the cli container which provides all the command and subcommand
// actions
type Container struct {
	// ConfigDir is the configuration directory path, defaults to ~/.config/d8x
	ConfigDir string

	// Default ssh key pathname. Defaults to ./id_ed25519 for private key. For
	// public key same name is used + .pub
	SshKeyPath string

	// Default user that will be created on each cluster node. This user will
	// have ssh key provided via SshKeyPath added to authorized_keys
	DefaultClusterUserName string
	// Password of DefaultClusterUserName. If not provided, attempt to read
	// password from ./password.txt will be made. For configuraton
	UserPassword string

	EmbedCopier configs.EmbedMultiFileToDestCopier

	// Time when provisioning was done. Used ot calculate cooldown for ansible
	// configuration. If provisioning was not done in current cli session, this
	// will not be set.
	provisioningTime time.Time
}

func NewDefaultContainer() *Container {
	return &Container{
		EmbedCopier: configs.NewEmbedFileCopier(),
	}
}

// expandCMD expands input string to argument slice suitable for exec.Command
// args parameter
func expandCMD(input string) []string {
	return strings.Split(input, " ")
}

// connectCMDToCurrentTerm connects std{in,out,err} to current terminal
func connectCMDToCurrentTerm(c *exec.Cmd) {
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
}
