package files

import (
	"bytes"
	"io"
	"os"
)

type ReplacementTuple struct {
	Find    string
	Replace string
}

// FSInteractor provides abstraction for interacting with file system
type FSInteractor interface {
	// WriteFile creates fileName if it does not exist and writes contents to
	// it
	WriteFile(fileName string, contents []byte) error

	ReplaceAndCopy(inFile, outFile string, replacements []ReplacementTuple) error
}

func NewFileSystemInteractor() FSInteractor {
	return &fsInteractor{}
}

type fsInteractor struct {
}

func (f *fsInteractor) WriteFile(fileName string, contents []byte) error {
	fd, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer fd.Close()
	_, err = io.Copy(fd, bytes.NewReader(contents))
	return err
}

func (f *fsInteractor) ReplaceAndCopy(inFile, outFile string, replacements []ReplacementTuple) error {
	return nil
}
