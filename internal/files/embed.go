package files

import (
	"embed"
	"io"
	"os"
	"path/filepath"
)

// EmbedMultiFileToDestCopier appends contents of all embedPaths (in the
// provided order) to dest. Filepaths embedPaths are searched in given fs. If
// file in destPath does not exist, it is created and truncated. If destPath
// contains a nested dir which is not available - it will be created too.
type EmbedMultiFileToDestCopier interface {
	Copy(fs embed.FS, destPath string, embedPaths ...string) error
}

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
	defer dest.Close()
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
