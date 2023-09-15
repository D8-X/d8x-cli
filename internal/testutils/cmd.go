package testutils

import (
	"fmt"
	"os/exec"
)

func CheckCmdArgs(cmd *exec.Cmd, wantCmd string, argsList []string) error {
	// cmd.Args includes the command itself as the first arg
	if len(argsList)+1 != len(cmd.Args) {
		return fmt.Errorf("expected %d args, got %d", len(argsList), len(cmd.Args)-1)
	}

	for i, arg := range argsList {
		if arg != cmd.Args[i+1] {
			return fmt.Errorf("expected arg %d to be %s, got %s", i, arg, cmd.Args[i+1])
		}
	}
	return nil
}
