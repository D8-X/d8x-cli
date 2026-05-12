package cmd

import "testing"

func TestIsHelpOrVersionInvocation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", []string{"d8x"}, true},
		{"help subcommand", []string{"d8x", "help"}, true},
		{"help with target", []string{"d8x", "help", "setup"}, true},
		{"h alias", []string{"d8x", "h", "setup"}, true},
		{"--help flag at root", []string{"d8x", "--help"}, true},
		{"-h flag at root", []string{"d8x", "-h"}, true},
		{"--help on subcommand", []string{"d8x", "setup", "--help"}, true},
		{"-h on subcommand", []string{"d8x", "setup", "-h"}, true},
		{"--help on nested subcommand", []string{"d8x", "setup", "rpc", "--help"}, true},
		{"--version", []string{"d8x", "--version"}, true},
		{"-v", []string{"d8x", "-v"}, true},

		{"plain init", []string{"d8x", "init"}, false},
		{"plain setup", []string{"d8x", "setup", "rpc"}, false},
		{"flag with literal help as value (--password help)", []string{"d8x", "--password", "help", "setup"}, false},
		{"flag with literal h as value (--password h)", []string{"d8x", "--password", "h", "setup"}, false},
		{"flag with literal -h as value (--password -h)", []string{"d8x", "--password", "-h", "setup"}, false},
		{"--chdir followed by help is value", []string{"d8x", "--chdir", "help", "init"}, false},
		{"--github-token followed by --version is value", []string{"d8x", "--github-token", "--version", "init"}, false},
		{"--quiet then plain subcommand stays false", []string{"d8x", "--quiet", "setup", "rpc"}, false},
		{"--quiet then --help still help", []string{"d8x", "--quiet", "--help"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHelpOrVersionInvocation(tc.args); got != tc.want {
				t.Fatalf("args=%v: got %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
