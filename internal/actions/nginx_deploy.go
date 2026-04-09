package actions

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
)

// nginxDeployConfig holds everything needed to deploy nginx to a server
type nginxDeployConfig struct {
	sshConn  conn.SSHConnection
	password string
	apiKey   string

	nginxConfContent      string
	sitesConfContent      string
	authCheckContent      string
	stagingOriginsContent string
}

// deployNginxFull deploys all nginx configs, cleans up, tests, and reloads.
// Matches the order from the Ansible playbook.
func deployNginxFull(cfg nginxDeployConfig) error {
	sshConn := cfg.sshConn
	password := cfg.password

	authCheck := strings.ReplaceAll(cfg.authCheckContent, "API_KEY_HERE", cfg.apiKey)

	fmt.Println("Preparing nginx...")
	sshExecSudo(sshConn, password, "rm -f /etc/nginx/sites-enabled/default")
	sshExecSudo(sshConn, password, "rm -f /etc/nginx/conf.d/auth_check.conf")
	sshExecSudo(sshConn, password, "mkdir -p /etc/nginx/conf.d")

	fmt.Println("Deploying nginx configs...")
	files := []struct {
		dest    string
		content string
	}{
		{"/etc/nginx/nginx.conf", cfg.nginxConfContent},
		{"/etc/nginx/sites-enabled/d8x", cfg.sitesConfContent},
		{"/etc/nginx/auth_check.conf", authCheck},
		{"/etc/nginx/conf.d/staging_origins.map", cfg.stagingOriginsContent},
	}

	for _, f := range files {
		if err := sshWriteFileSudo(sshConn, password, f.dest, f.content); err != nil {
			return err
		}
		fmt.Printf("  %s\n", f.dest)
	}

	// 3. Set nginx file limits
	sshExecSudo(sshConn, password, "mkdir -p /etc/systemd/system/nginx.service.d")
	sshWriteFileSudo(sshConn, password, "/etc/systemd/system/nginx.service.d/nofiles.conf", "[Service]\nLimitNOFILE=700000\n")
	sshExecSudo(sshConn, password, "systemctl daemon-reload")

	fmt.Println("Testing nginx config...")
	if out, err := sshConn.ExecCommand(fmt.Sprintf("echo '%s' | sudo -S nginx -t 2>&1", password)); err != nil {
		return fmt.Errorf("nginx config test failed:\n%s", string(out))
	}

	fmt.Println("Reloading nginx...")
	sshExecSudo(sshConn, password, "systemctl reload nginx")

	fmt.Println(styles.SuccessText.Render("Nginx deployed and reloaded."))
	return nil
}

// fetchAndBuildNginxConfig fetches all nginx config files from GitHub for the given environment
func fetchAndBuildNginxConfig(token, env string) (*nginxDeployConfig, error) {
	nginxConf, err := ghReadFile(token, env+"/nginx.conf")
	if err != nil {
		return nil, fmt.Errorf("reading nginx.conf: %w", err)
	}
	sitesConf, err := ghReadFile(token, env+"/sites.conf")
	if err != nil {
		return nil, fmt.Errorf("reading sites.conf: %w", err)
	}
	authCheck, err := ghReadFile(token, env+"/auth_check.conf")
	if err != nil {
		return nil, fmt.Errorf("reading auth_check.conf: %w", err)
	}
	stagingOrigins, err := ghReadFile(token, env+"/staging_origins.map")
	if err != nil {
		return nil, fmt.Errorf("reading staging_origins.map: %w", err)
	}

	return &nginxDeployConfig{
		nginxConfContent:      nginxConf.Content,
		sitesConfContent:      sitesConf.Content,
		authCheckContent:      authCheck.Content,
		stagingOriginsContent: stagingOrigins.Content,
	}, nil
}

func sshExecSudo(sshConn conn.SSHConnection, password, cmd string) {
	sshConn.ExecCommand(fmt.Sprintf("echo '%s' | sudo -S %s", password, cmd))
}

func sshWriteFileSudo(sshConn conn.SSHConnection, password, dest, content string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	tmpCmd := fmt.Sprintf("echo '%s' | base64 -d > /tmp/d8x_deploy_tmp", encoded)
	if _, err := sshConn.ExecCommand(tmpCmd); err != nil {
		return fmt.Errorf("writing tmp file for %s: %w", dest, err)
	}
	mvCmd := fmt.Sprintf("echo '%s' | sudo -S mv /tmp/d8x_deploy_tmp %s", password, dest)
	if _, err := sshConn.ExecCommand(mvCmd); err != nil {
		return fmt.Errorf("moving to %s: %w", dest, err)
	}
	return nil
}

func extractAllServerNames(sitesConf string) []string {
	seen := map[string]bool{}
	var names []string
	for _, line := range strings.Split(sitesConf, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "server_name ") {
			name := strings.TrimPrefix(line, "server_name ")
			name = strings.TrimSuffix(name, ";")
			name = strings.TrimSpace(name)
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}
