package conn

import (
	"fmt"
	"path"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SftpCopySrcDest struct {
	Content []byte
	Dst     string
}

func CopyFilesOverSftp(
	conn *ssh.Client,
	srcDst ...SftpCopySrcDest,
) error {
	s, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}

	for _, cp := range srcDst {
		dir := path.Dir(cp.Dst)
		if err := s.MkdirAll(dir); err != nil {
			return err
		}

		content := cp.Content
		if content == nil {
			return fmt.Errorf("sftp copy: Dst=%q has no Content", cp.Dst)
		}

		dstFd, err := s.Create(cp.Dst)
		if err != nil {
			return err
		}
		if _, err := dstFd.Write(content); err != nil {
			dstFd.Close()
			return err
		}
		dstFd.Close()
	}

	return nil
}
