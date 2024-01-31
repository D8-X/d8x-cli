package actions

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// SSH establishes ssh connection to manager or broker servers and attaches ssh
// session to current terminal
func (c *Container) SSH(ctx *cli.Context) error {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	serverName := ctx.Args().First()
	ip := ""
	isWorker := false
	switch serverName {
	case "manager":
		ip, err = c.HostsCfg.GetMangerPublicIp()
	case "broker":
		ip, err = c.HostsCfg.GetBrokerPublicIp()
	default:
		// Parse workers
		if strings.HasPrefix(serverName, "worker-") {
			ips, err := c.HostsCfg.GetWorkerIps()
			if err != nil {
				return err
			}
			parsedWorkerNum, err := strconv.Atoi(strings.Split(serverName, "worker-")[1])
			if err != nil {
				return fmt.Errorf("Incorrect worker name was passed. Accepted values are worker-1, worker-2, worker-3, worker-*...")
			}

			isWorker = true

			if parsedWorkerNum <= len(ips) {
				ip = ips[parsedWorkerNum-1]
				break
			}
		}

		return fmt.Errorf("Incorrect server name was passed. Accepted values are manager, broker, worker-* (where * is a digit)")
	}

	if err != nil {
		return err
	}

	fmt.Println(styles.ItalicText.Render(
		fmt.Sprintf("Establishing ssh connection to %s at %s\n", serverName, ip),
	))

	// Get the connection
	var (
		cn      conn.SSHConnection
		connErr error
	)
	if isWorker {
		cn, connErr = c.GetWorkerConnection(ip, cfg)
	} else {
		cn, connErr = conn.NewSSHConnection(ip, c.DefaultClusterUserName, c.SshKeyPath)
	}
	if connErr != nil {
		return connErr
	}

	sshClient := cn.GetClient()

	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	fileDescriptor := int(os.Stdin.Fd())
	originalState, err := term.MakeRaw(fileDescriptor)
	if err != nil {
		return err
	}

	w, h, err := term.GetSize(fileDescriptor)
	if err != nil {
		return err
	}
	defer term.Restore(fileDescriptor, originalState)

	if err := session.RequestPty("xterm", h, w, ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}); err != nil {
		return err
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Shell(); err != nil {
		return err
	}

	if err := session.Wait(); err != nil {
		return err
	}

	return nil
}

// GetWorkerConnection establishes a connection to given worker. If Server
// provider is AWS, we use manager as a bastion server
func (c *Container) GetWorkerConnection(workerIp string, cfg *configs.D8XConfig) (conn.SSHConnection, error) {
	var (
		cn      conn.SSHConnection
		connErr error
	)

	if cfg.ServerProvider == configs.D8XServerProviderLinode {
		cn, connErr = conn.NewSSHConnection(workerIp, c.DefaultClusterUserName, c.SshKeyPath)
	} else {
		managerIp, err := c.HostsCfg.GetMangerPublicIp()
		if err != nil {
			return nil, err
		}

		// Workers are accessible through manager for AWS
		managerConn, errMngr := conn.NewSSHConnection(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
		if errMngr != nil {
			return nil, errMngr
		}
		cn, connErr = conn.NewSSHConnectionWithBastion(managerConn.GetClient(), workerIp, c.DefaultClusterUserName, c.SshKeyPath)
	}

	return cn, connErr
}
