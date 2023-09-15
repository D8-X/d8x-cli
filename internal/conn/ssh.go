package conn

import (
	"fmt"
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

var _ (SSHConnection) = (*sshConnection)(nil)

type sshConnection struct {
	c *ssh.Client
}

func (conn *sshConnection) ExecCommand(cmd string) ([]byte, error) {
	s, err := conn.c.NewSession()
	if err != nil {
		return nil, err
	}
	return s.CombinedOutput(cmd)
}

// SSHExecCommandPiped connects stdin/out/err
func (conn *sshConnection) ExecCommandPiped(cmd string) error {
	s, err := conn.c.NewSession()
	if err != nil {
		return err
	}

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

func (conn *sshConnection) CopyFilesOverSftp(srcDst ...SftpCopySrcDest) error {
	return CopyFilesOverSftp(conn.c, srcDst...)
}
