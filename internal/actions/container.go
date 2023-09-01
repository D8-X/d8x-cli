package actions

import "strings"

// Container is the cli container which provides all the command and subcommand
// actions
type Container struct {
	// ConfigDir is the configuration directory path, defaults to ~/.config/d8x
	ConfigDir string
}

// expandCMD expands input string to argument slice suitable for exec.Command
// args parameter
func expandCMD(input string) []string {
	return strings.Split(input, " ")
}
