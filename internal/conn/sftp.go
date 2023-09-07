package conn

import (
	"os"
	"path"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SftpCopySrcDest struct {
	// Local source
	Src string
	// Remote destination
	Dst string
}

// CopyFilesOverSftp copies the list of srcDst to remote conn.
func CopyFilesOverSftp(
	conn *ssh.Client,
	srcDst ...SftpCopySrcDest,
) error {
	s, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}

	for _, cp := range srcDst {
		// Endure dir exists on remote
		dir := path.Dir(cp.Dst)
		if err := s.MkdirAll(dir); err != nil {
			return err
		}

		// Open source file
		srcFileContents, err := os.ReadFile(cp.Src)
		if err != nil {
			return err
		}

		// Open remote file
		dstFd, err := s.Create(cp.Dst)
		if err != nil {
			return err
		}

		// Write
		_, err = dstFd.Write(srcFileContents)
		if err != nil {
			return err
		}

		dstFd.Close()
	}

	return nil
}
