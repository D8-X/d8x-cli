package conn

import (
	"fmt"
	"os"
	"path"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SftpCopySrcDest struct {
	Src     string
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
			if cp.Src == "" {
				return fmt.Errorf("sftp copy: Dst=%q has neither Content nor Src", cp.Dst)
			}
			content, err = os.ReadFile(cp.Src)
			if err != nil {
				return err
			}
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
