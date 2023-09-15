package files

import (
	"embed"
	"io"
	"os"
	"path/filepath"
)

//go:generate mockgen -package mocks -destination ../mocks/files.go . EmbedFileCopier,HostsFileInteractor,FSInteractor

type EmbedCopierOp struct {
	// Src is the path to the file in embed.FS
	Src string
	// Dst is the path to the file in local fs
	Dst string
	// Overwrite determines whether existing file should be overwritten
	Overwrite bool
}

// EmbedFileCopier handles copying of embed files to local fs
type EmbedFileCopier interface {
	// CopyMultiToDest appends (concats) contents of all embedPaths (in the provided
	// order) to dest. Filepaths embedPaths are searched in given fs. If file in
	// destPath does not exist, it is created and truncated. If destPath
	// contains a nested dir which is not available - it will be created too.
	CopyMultiToDest(fs embed.FS, destPath string, embedPaths ...string) error

	// Copy simply performs copy operations
	Copy(fs embed.FS, operations ...EmbedCopierOp) error
}

func NewEmbedFileCopier() EmbedFileCopier {
	return &embedFileCopier{}
}

var _ EmbedFileCopier = (*embedFileCopier)(nil)

type embedFileCopier struct{}

func (e *embedFileCopier) CopyMultiToDest(efs embed.FS, destPath string, embedPaths ...string) error {
	e.ensureNestedDirsPresent(destPath)

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

func (e *embedFileCopier) Copy(fs embed.FS, operations ...EmbedCopierOp) error {
	for _, op := range operations {
		if !op.Overwrite {
			if _, err := os.Stat(op.Dst); err == nil {
				// File exists, skip
				continue
			}
		}

		if err := e.ensureNestedDirsPresent(op.Dst); err != nil {
			return err
		}

		outFile, err := os.OpenFile(op.Dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			return err
		}
		if err := copyEmbedFilesToDest(outFile, fs, op.Src); err != nil {
			return err
		}
	}

	return nil
}

func (e *embedFileCopier) ensureNestedDirsPresent(destPath string) error {
	dir := filepath.Dir(destPath)
	if _, err := os.Stat(dir); err != nil {
		// Attempt to create dir
		if err := os.MkdirAll(dir, 0766); err != nil {
			return err
		}
	}
	return nil
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
