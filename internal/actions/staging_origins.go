package actions

import (
	"fmt"
	"os"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

const originsFilePath = "./nginx/staging_origins.map"

func (c *Container) UpdateStagingOrigins(ctx *cli.Context) error {
	styles.PrintCommandTitle("Manage whitelisted origins")

	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}

	current, err := fetchOriginsFromServer(c, managerIp)
	if err != nil {
		fmt.Println(styles.ItalicText.Render("Could not fetch origins from server, using local file"))
		current = loadOrigins()
	}

	for {
		if len(current) > 0 {
			fmt.Println(styles.ItalicText.Render("\nCurrent whitelisted staging origins:"))
			for i, o := range current {
				fmt.Printf("  %d. %s\n", i+1, o)
			}
		} else {
			fmt.Println(styles.ItalicText.Render("\nNo staging origins configured."))
		}

		actions := []string{"Add origin", "Remove origin", "Deploy and exit", "Exit without deploying"}
		if len(current) == 0 {
			actions = []string{"Add origin", "Deploy and exit", "Exit without deploying"}
		}

		selected, err := c.TUI.NewSelection(actions, components.SelectionOptAllowOnlySingleItem(), components.SelectionOptRequireSelection())
		if err != nil {
			return err
		}
		action := selected[0]

		switch action {
		case "Add origin":
			input, err := c.TUI.NewInput(
				components.TextInputOptPlaceholder("https://staging.example.com"),
			)
			if err != nil {
				return err
			}
			for _, o := range strings.Split(input, ",") {
				o = strings.TrimSpace(o)
				o = strings.TrimRight(o, "/")
				if o != "" && !contains(current, o) {
					current = append(current, o)
					fmt.Println(styles.SuccessText.Render("  + " + o))
				}
			}

		case "Remove origin":
			if len(current) == 0 {
				continue
			}
			toRemove, err := c.TUI.NewSelection(current)
			if err != nil {
				return err
			}
			for _, r := range toRemove {
				current = remove(current, r)
				fmt.Println(styles.ErrorText.Render("  - " + r))
			}

		case "Deploy and exit":
			content := buildOriginsFile(current)
			if err := writeAndDeploy(c, managerIp, content); err != nil {
				return err
			}
			fmt.Println(styles.SuccessText.Render("Origins deployed and nginx reloaded."))
			return nil

		case "Exit without deploying":
			return nil
		}
	}
}

func fetchOriginsFromServer(c *Container, managerIp string) ([]string, error) {
	sshConn, err := c.CreateSSHConn(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("SSH connection: %w", err)
	}
	output, err := sshConn.ExecCommand("cat /etc/nginx/conf.d/staging_origins.map 2>/dev/null || echo ''")
	if err != nil {
		return nil, fmt.Errorf("reading remote file: %w", err)
	}
	return parseOrigins(string(output)), nil
}

func loadOrigins() []string {
	data, err := os.ReadFile(originsFilePath)
	if err != nil {
		return nil
	}
	return parseOrigins(string(data))
}

func parseOrigins(content string) []string {
	var origins []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			origin := strings.Trim(parts[0], "\"")
			if origin != "" {
				origins = append(origins, origin)
			}
		}
	}
	return origins
}

func buildOriginsFile(origins []string) string {
	var lines []string
	lines = append(lines, "# Staging origins > managed by: ./d8x staging-origins")
	for _, o := range origins {
		lines = append(lines, fmt.Sprintf("%q 1;", o))
	}
	return strings.Join(lines, "\n") + "\n"
}

func writeAndDeploy(c *Container, managerIp, content string) error {
	os.MkdirAll("./nginx", 0755)
	if err := os.WriteFile(originsFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	sshConn, err := c.CreateSSHConn(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return fmt.Errorf("SSH connection: %w", err)
	}

	escaped := strings.ReplaceAll(content, "'", "'\\''")
	cmd := fmt.Sprintf("echo '%s' | sudo tee /etc/nginx/conf.d/staging_origins.map > /dev/null && sudo nginx -s reload", escaped)
	if _, err := sshConn.ExecCommand(cmd); err != nil {
		return fmt.Errorf("updating nginx: %w", err)
	}

	return nil
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func remove(list []string, item string) []string {
	var result []string
	for _, v := range list {
		if v != item {
			result = append(result, v)
		}
	}
	return result
}
