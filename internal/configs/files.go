package configs

import (
	"embed"
	"io"
	"os"
	"path/filepath"
)

func NewEmbedFileCopier() EmbedMultiFileToDestCopier {
	return &embedFileCopier{}
}

var _ EmbedMultiFileToDestCopier = (*embedFileCopier)(nil)

type embedFileCopier struct{}

func (e *embedFileCopier) Copy(efs embed.FS, destPath string, embedPaths ...string) error {
	// Ensure nested dirs are present
	dir := filepath.Dir(destPath)
	if _, err := os.Stat(dir); err != nil {
		// Attempt to create dir
		if err := os.MkdirAll(dir, 0766); err != nil {
			return err
		}
	}

	outFile, err := os.OpenFile(destPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	return copyEmbedFilesToDest(
		outFile,
		efs,
		embedPaths...,
	)
}

// copyEmbedFilesToDest copies embedFiles from embedFS into dest in the order
// that embedFS are provided
func copyEmbedFilesToDest(dest *os.File, embedFS embed.FS, embedFiles ...string) error {
	for _, embedFile := range embedFiles {
		f, err := embedFS.Open(embedFile)
		if err != nil {
			return err
		}

		_, err = io.Copy(dest, f)
		if err != nil {
			return err
		}
	}

	return nil
}
