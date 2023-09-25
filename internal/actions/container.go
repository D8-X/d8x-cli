package actions

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/urfave/cli/v2"
)

type SSHConnectionMaker func()

// Container is the cli container which provides all the command and subcommand
// actions
type Container struct {
	// ConfigDir is the configuration directory path
	ConfigDir string

	// Default ssh key pathname. Defaults to ./id_ed25519 for private key. For
	// public key same name is used + .pub
	SshKeyPath string

	// Default user that will be created on each cluster node. This user will
	// have ssh key provided via SshKeyPath added to authorized_keys
	DefaultClusterUserName string

	// Password of DefaultClusterUserName. If not provided, attempt to read
	// password from ./password.txt will be made in Before action. If
	// Configuration action was executed, the password value will be set.
	UserPassword string

	EmbedCopier files.EmbedFileCopier

	FS files.FSInteractor

	// Time when provisioning was done. Used ot calculate cooldown for ansible
	// configuration. If provisioning was not done in current cli session, this
	// will not be set.
	provisioningTime time.Time

	// Whether broker server should be provisioned and d8x-broker-server
	// deployed.
	CreateBrokerServer bool

	HostsCfg files.HostsFileInteractor

	// pg.crt path, defaults to ./pg.crt
	PgCrtPath string

	// Default http client use for http interactions
	HttpClient *http.Client

	ConfigRWriter configs.D8XConfigReadWriter

	// terminal ui runner
	TUI components.ComponentsRunner

	// Retrieve the servers default user sudo password
	GetPassword func(ctx *cli.Context) (string, error)

	CreateSSHConn conn.SSHConnectionEstablisher

	// RunCmd runs the provided command
	RunCmd func(*exec.Cmd) error
}

func NewDefaultContainer() *Container {

	httpClient := http.DefaultClient
	return &Container{
		EmbedCopier:   files.NewEmbedFileCopier(),
		FS:            files.NewFileSystemInteractor(),
		HostsCfg:      files.NewFSHostsFileInteractor(configs.DEFAULT_HOSTS_FILE),
		HttpClient:    httpClient,
		TUI:           components.InteractiveRunner{},
		GetPassword:   defaultPasswordGetter,
		CreateSSHConn: conn.NewSSHConnection,
		RunCmd: func(c *exec.Cmd) error {
			return c.Run()
		},
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
