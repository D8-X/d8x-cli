package conn

import (
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

//go:generate mockgen -package mocks -destination ../mocks/conn.go . SSHConnection

type SSHConnection interface {
	// Execute cmd on remote server
	ExecCommand(cmd string) ([]byte, error)

	// ExecCommandPiped works exactly like ExecCommand but connects
	// stdin/out/err
	ExecCommandPiped(cmd string) error

	CopyFilesOverSftp(srcDst ...SftpCopySrcDest) error

	GetClient() *ssh.Client
}

type SSHConnectionEstablisher func(serverIp, user, idFilePath string) (SSHConnection, error)

var _ (SSHConnectionEstablisher) = NewSSHConnection

// NewSSHClient attempts to connect to server via ssh on default port 22
func NewSSHConnection(serverIp, user, idFilePath string) (SSHConnection, error) {
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

	// TODO pass port as parameter
	c, err := ssh.Dial("tcp", serverIp+":22", config)
	if err != nil {
		return nil, err
	}
	return &sshConnection{c: c}, nil
}

func NewSSHConnectionWithBastion(bastion *ssh.Client, serverIp, user, idFilePath string) (SSHConnection, error) {
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

	targetConn, err := bastion.Dial("tcp", serverIp+":22")
	if err != nil {
		return nil, fmt.Errorf("dialing to target via bastion: %w", err)
	}
	a, b, c, err := ssh.NewClientConn(targetConn, ":22", config)
	if err != nil {
		return nil, err
	}

	return &sshConnection{c: ssh.NewClient(a, b, c)}, nil
}

var _ (SSHConnection) = (*sshConnection)(nil)

type sshConnection struct {
	c *ssh.Client
}

func (s *sshConnection) GetClient() *ssh.Client {
	return s.c
}

func (conn *sshConnection) ExecCommand(cmd string) ([]byte, error) {
	// Print out the cmd for debugging
	if _, ok := os.LookupEnv("DEBUG"); ok {
		fmt.Printf("[CMD]: %s\n", cmd)
	}

	s, err := conn.c.NewSession()
	if err != nil {
		return nil, err
	}
	return s.CombinedOutput(cmd)
}

// SSHExecCommandPiped connects stdin/out/err
func (conn *sshConnection) ExecCommandPiped(cmd string) error {
	// Print out the cmd for debugging
	if _, ok := os.LookupEnv("DEBUG"); ok {
		fmt.Printf("[CMD]: %s\n", cmd)
	}

	s, err := conn.c.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.RequestPty("xterm", 80, 80,
		ssh.TerminalModes{
			ssh.ECHO:          0,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		},
	); err != nil {
		return err
	}

	out, err := s.StdoutPipe()
	if err != nil {
		return fmt.Errorf("retrieving remote stdout: %w", err)
	}

	go func() {
		for {
			n, err := io.Copy(os.Stdout, out)
			if err != nil || n == 0 {
				return
			}
		}
	}()

	return s.Run(cmd)

}

func (conn *sshConnection) CopyFilesOverSftp(srcDst ...SftpCopySrcDest) error {
	return CopyFilesOverSftp(conn.c, srcDst...)
}
