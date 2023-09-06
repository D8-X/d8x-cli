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
