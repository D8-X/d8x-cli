package files

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path"
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
	// Whether the src and dst is a directory and should copy all contents
	Dir bool
}

// EmbedFileCopier handles copying of embed files to local fs
type EmbedFileCopier interface {
	// CopyMultiToDest appends (concats) contents of all embedPaths (in the provided
	// order) to dest. Filepaths embedPaths are searched in given fs. If file in
	// destPath does not exist, it is created and truncated. If destPath
	// contains a nested dir which is not available - it will be created too.
	CopyMultiToDest(fs embed.FS, destPath string, embedPaths ...string) error

	// Copy simply performs copy operations. If Any of given operations include
	// Dir=true, all files from given operation's  Src will be copied to Dst
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
	copyFunc := func(src, dst string, overWrite bool, fs embed.FS) error {
		if !overWrite {
			if _, err := os.Stat(dst); err == nil {
				// File exists, skip
				return nil
			}
		}

		if err := e.ensureNestedDirsPresent(dst); err != nil {
			return err
		}

		outFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			return err
		}
		if err := copyEmbedFilesToDest(outFile, fs, src); err != nil {
			return err
		}
		return nil
	}

	for _, op := range operations {
		if op.Dir {
			entries, err := fs.ReadDir(op.Src)
			if err != nil {
				return fmt.Errorf("reading embedded fs: %w", err)
			}

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				src := path.Join(op.Src, entry.Name())
				dst := path.Join(op.Dst, entry.Name())
				if err := copyFunc(src, dst, op.Overwrite, fs); err != nil {
					return err
				}
			}

		} else {
			if err := copyFunc(op.Src, op.Dst, op.Overwrite, fs); err != nil {
				return err
			}
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
