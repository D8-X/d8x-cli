package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

var nonEnvDirs = map[string]struct{}{
	"terraform": {},
}

func (c *Container) RemoveEnvironment(ctx *cli.Context) error {
	styles.PrintCommandTitle("Remove a non-provisioned environment from the infra repo")

	if err := c.RequireBitwardenField("GITHUB_TOKEN"); err != nil {
		return err
	}
	token := os.Getenv("GITHUB_TOKEN")

	allDirs, err := ghListDirs(token)
	if err != nil {
		return fmt.Errorf("cannot list directories on infra repo: %w", err)
	}

	var envs []string
	var labels []string
	for _, d := range allDirs {
		if strings.HasPrefix(d, ".") {
			continue
		}
		if _, ok := nonEnvDirs[d]; ok {
			continue
		}
		cfgFile, err := ghReadFile(token, d+"/config.json")
		if err != nil {
			continue
		}
		var ec configs.D8XConfig
		if err := json.Unmarshal([]byte(cfgFile.Content), &ec); err != nil {
			continue
		}
		if _, hostsErr := ghReadFile(token, d+"/hosts.cfg"); hostsErr == nil {
			continue
		}
		envs = append(envs, d)
		label := d
		if ec.ChainId > 0 {
			label = fmt.Sprintf("%s  (chain %d)", d, ec.ChainId)
		}
		labels = append(labels, label)
	}

	if len(envs) == 0 {
		fmt.Println(styles.ItalicText.Render("No non-provisioned environments found on the infra repo. Nothing to remove."))
		fmt.Println(styles.ItalicText.Render("(\"setup rm-env\" only handles never-provisioned envs; for a provisioned one, run \"d8x tf-destroy\" first.)"))
		return nil
	}

	fmt.Println(styles.ItalicText.Render("Select the environment to remove (only non-provisioned envs are shown):"))
	picked, err := c.TUI.NewSelection(
		labels,
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)
	if err != nil {
		return err
	}
	env := envs[indexOf(labels, picked[0])]

	paths, err := ghListEnvFiles(token, env)
	if err != nil {
		return fmt.Errorf("listing files under %s/: %w", env, err)
	}
	if len(paths) == 0 {
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf("Environment %q has no files on the infra repo (already empty).", env)))
		return nil
	}

	fmt.Println(styles.ItalicText.Render(fmt.Sprintf("\nWill delete %d file(s) under %s/ on the infra repo:", len(paths), env)))
	for _, p := range paths {
		fmt.Printf("  - %s\n", p)
	}

	confirm, err := c.TUI.NewPrompt(fmt.Sprintf("Remove the %q environment from the infra repo?", env), false)
	if err != nil {
		return err
	}
	if !confirm {
		fmt.Println(styles.ItalicText.Render("Aborted; nothing was deleted."))
		return nil
	}

	if err := ghCommitDeletes(token, paths, fmt.Sprintf("remove %s environment", env)); err != nil {
		return fmt.Errorf("removing environment %s: %w", env, err)
	}
	fmt.Println(styles.SuccessText.Render(fmt.Sprintf("Removed environment %q (%d files) from the infra repo.", env, len(paths))))
	return nil
}

