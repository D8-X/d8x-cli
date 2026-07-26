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
	styles.PrintCommandTitle("Remove an environment from the infra repo")

	if err := c.RequireBitwardenField("GITHUB_TOKEN"); err != nil {
		return err
	}
	token := os.Getenv("GITHUB_TOKEN")

	allDirs, err := ghListDirs(token)
	if err != nil {
		return fmt.Errorf("cannot list directories on infra repo: %w", err)
	}

	type envInfo struct {
		name        string
		provisioned bool
	}
	var labels []string
	labelToInfo := map[string]envInfo{}
	envToInfo := map[string]envInfo{}
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
		hostsFile, hostsErr := ghReadFile(token, d+"/hosts.cfg")
		provisioned := hostsErr == nil && strings.TrimSpace(hostsFile.Content) != ""

		state := "not provisioned"
		if provisioned {
			state = "provisioned"
		}
		label := fmt.Sprintf("%s  [%s]", d, state)
		if ec.ChainId > 0 {
			label = fmt.Sprintf("%s  (chain %d)  [%s]", d, ec.ChainId, state)
		}
		labels = append(labels, label)
		info := envInfo{name: d, provisioned: provisioned}
		labelToInfo[label] = info
		envToInfo[d] = info
	}

	if len(labels) == 0 {
		fmt.Println(styles.ItalicText.Render("No environments found on the infra repo. Nothing to remove."))
		return nil
	}

	var info envInfo
	if name := strings.TrimSpace(ctx.String("env")); name != "" {
		got, found := envToInfo[name]
		if !found {
			return fmt.Errorf("env %q not found on the infra repo", name)
		}
		info = got
		fmt.Printf("%s env %q selected via --env flag\n", ok, info.name)
	} else {
		fmt.Println(styles.ItalicText.Render("Select the environment to remove from the infra repo:"))
		picked, err := c.TUI.NewSelection(
			labels,
			components.SelectionOptAllowOnlySingleItem(),
			components.SelectionOptRequireSelection(),
		)
		if err != nil {
			return err
		}
		info = labelToInfo[picked[0]]
	}

	paths, err := ghListEnvFiles(token, info.name)
	if err != nil {
		return fmt.Errorf("listing files under %s/: %w", info.name, err)
	}
	if len(paths) == 0 {
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf("Environment %q has no files on the infra repo (already empty).", info.name)))
		return nil
	}

	fmt.Println(styles.ItalicText.Render(fmt.Sprintf("\nWill delete %d file(s) under %s/ on the infra repo:", len(paths), info.name)))
	for _, p := range paths {
		fmt.Printf("  - %s\n", p)
	}

	if info.provisioned {
		fmt.Println(styles.AlertImportant.Render(fmt.Sprintf("Warning: %q still has hosts.cfg on the infra repo (provisioned).", info.name)))
		fmt.Println(styles.AlertImportant.Render("Deleting files here will NOT tear down cloud infra. Run \"d8x tf-destroy\" first if you want the servers gone."))
	}

	confirm, err := c.TUI.NewPrompt(fmt.Sprintf("Remove the %q environment from the infra repo?", info.name), false)
	if err != nil {
		return err
	}
	if !confirm {
		fmt.Println(styles.ItalicText.Render("Aborted; nothing was deleted."))
		return nil
	}

	fmt.Printf("Type the environment name (%s) to confirm:\n", info.name)
	typed, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder(info.name),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return err
	}
	if strings.TrimSpace(typed) != info.name {
		return fmt.Errorf("typed name %q does not match %q; aborted", typed, info.name)
	}

	if err := ghCommitDeletes(token, paths, fmt.Sprintf("remove %s/ from infra repo", info.name)); err != nil {
		return fmt.Errorf("removing environment %s: %w", info.name, err)
	}
	fmt.Println(styles.SuccessText.Render(fmt.Sprintf("Removed environment %q (%d files) from the infra repo.", info.name, len(paths))))
	return nil
}

