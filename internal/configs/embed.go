package configs

import (
	"embed"
)

//go:generate mockgen -p mocks

//go:embed trader-backend/*
var TraderBackendConfigs embed.FS

//go:embed playbooks/*
var AnsiblePlaybooks embed.FS

// EmbedMultiFileToDestCopier appends contents of all embedPaths (in the
// provided order) to dest. Filepaths embedPaths are searched in given fs. If
// file in destPath does not exist, it is created and truncated. If destPath
// contains a nested dir which is not available - it will be created too.
type EmbedMultiFileToDestCopier interface {
	Copy(fs embed.FS, destPath string, embedPaths ...string) error
}
