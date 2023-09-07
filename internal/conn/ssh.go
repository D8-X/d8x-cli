package conn

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

// NewSSHClient attempts to connect to server via ssh on default 22 port
func NewSSHClient(serverIp, user, idFilePath string) (*ssh.Client, error) {
	pk, err := os.ReadFile(idFilePath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(pk)
	if err != nil {
		return nil, fmt.Errorf("parsing private key %s: %v", idFilePath, err)
	}

	config := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		Timeout:         time.Second * 10,
	}

	return ssh.Dial("tcp", serverIp+":22", config)
}

func SSHExecCommand(c *ssh.Client, cmd string) ([]byte, error) {
	s, err := c.NewSession()
	if err != nil {
		return nil, err
	}
	return s.CombinedOutput(cmd)
}

// SSHExecCommandPiped connects stdin/out/err
func SSHExecCommandPiped(c *ssh.Client, cmd string) error {
	s, err := c.NewSession()
	if err != nil {
		return err
	}
	// if err := s.RequestSubsystem("bash"); err != nil {
	// 	return err
	// }

	if err := s.RequestPty("xterm", 80, 80,
		ssh.TerminalModes{
			ssh.ECHO:          0,     // disable echoing
			ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
			ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
		},
	); err != nil {
		return err
	}

	s.Stdin = os.Stdin
	s.Stdout = os.Stdout
	s.Stderr = os.Stderr
	if err := s.Shell(); err != nil {
		return err
	}

	if _, err := os.Stdin.Read([]byte(cmd)); err != nil {
		return err
	}

	return s.Close()
}
