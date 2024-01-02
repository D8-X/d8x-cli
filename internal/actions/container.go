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

type ChainInfo struct {
	ChainId   string
	ChainName string
}

// ALLOWED_CHAINS_STRINGS is used for selection component
var ALLOWED_CHAINS_STRINGS = []string{
	"zkEVM testnet (1442)",
	"zkEVM mainnet (1101)",
	"x1 testnet (195)",
}

// Chain ids which are allowed to enter
var ALLOWED_CHAINS_MAP = map[string]string{
	"zkEVM testnet (1442)": "1442",
	"zkEVM mainnet (1101)": "1101",
	"x1 testnet (195)":     "195",
}

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

	// Directory to the terraform files. Defaults to ./terraform but can be
	// overriden by --tf-dir flag
	ProvisioningTfDir string

	EmbedCopier files.EmbedFileCopier

	FS files.FSInteractor

	// Time when provisioning was done. Used ot calculate cooldown for ansible
	// configuration. If provisioning was not done in current cli session, this
	// will not be set.
	provisioningTime time.Time

	HostsCfg files.HostsFileInteractor

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

	// Cached parsed chain.json contents
	cachedChainJson ChainJson

	// Global input state
	Input *InputCollector
}

func NewDefaultContainer() (*Container, error) {

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
		ProvisioningTfDir: TF_FILES_DIR,
	}, nil
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
