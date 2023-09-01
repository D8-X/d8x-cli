package version

import "fmt"

// Injected at build-time
var (
	// commit might be a hash or tag
	commit,

	// buildTime is the time when
	buildTime string
)

// Get builds the version string
func Get() string {
	if commit == "" {
		return "development-build"
	}

	return fmt.Sprintf("%s built at %s", commit, buildTime)
}
