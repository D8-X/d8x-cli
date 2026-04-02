package actions

import (
	"fmt"
	"os"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) UpdateStagingOrigins(ctx *cli.Context) error {
	styles.PrintCommandTitle("Updating staging origins...")

	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}

	input, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("https://staging.predictex.io,https://dev.predictex.io"),
	)
	if err != nil {
		return err
	}

	var lines []string
	lines = append(lines, "# Staging origins - managed by: ./cli staging-origins")
	lines = append(lines, "# After editing, run: nginx -s reload")
	for _, o := range strings.Split(input, ",") {
		if o = strings.TrimSpace(o); o != "" {
			lines = append(lines, fmt.Sprintf("%q 1;", o))
		}
	}
	content := strings.Join(lines, "\n") + "\n"

	localPath := "./nginx/staging_origins.map"
	os.MkdirAll("./nginx", 0755)
	if err := os.WriteFile(localPath, []byte(content), 0644); err != nil {
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

	fmt.Println(styles.SuccessText.Render("Staging origins updated! Nginx reloaded (no service restart)."))
	return nil
}
