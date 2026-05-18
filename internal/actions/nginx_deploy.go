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

	if _, err := sshExecSudo(sshConn, password, "mkdir -p /etc/systemd/system/nginx.service.d"); err != nil {
		return fmt.Errorf("mkdir nginx.service.d: %w", err)
	}
	if err := sshWriteFileSudo(sshConn, password, "/etc/systemd/system/nginx.service.d/nofiles.conf", "[Service]\nLimitNOFILE=700000\n"); err != nil {
		return fmt.Errorf("writing nofiles.conf: %w", err)
	}
	if _, err := sshExecSudo(sshConn, password, "systemctl daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	fmt.Println("Testing nginx config...")
	if out, err := sshExecSudo(sshConn, password, "nginx -t 2>&1"); err != nil {
		return fmt.Errorf("nginx config test failed:\n%s", string(out))
	}

	fmt.Println("Reloading nginx...")
	if out, err := sshExecSudo(sshConn, password, "systemctl reload nginx"); err != nil {
		return fmt.Errorf("nginx reload failed:\n%s", string(out))
	}

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

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func sshExecSudo(sshConn conn.SSHConnection, password, cmd string) ([]byte, error) {
	full := fmt.Sprintf("printf '%%s\\n' %s | sudo -S %s", shQuote(password), cmd)
	return sshConn.ExecCommand(full)
}

func sshWriteFileSudo(sshConn conn.SSHConnection, password, dest, content string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	tmpCmd := fmt.Sprintf("printf '%%s' %s | base64 -d > /tmp/d8x_deploy_tmp", shQuote(encoded))
	if _, err := sshConn.ExecCommand(tmpCmd); err != nil {
		return fmt.Errorf("writing tmp file for %s: %w", dest, err)
	}
	mvCmd := fmt.Sprintf("printf '%%s\\n' %s | sudo -S mv /tmp/d8x_deploy_tmp %s", shQuote(password), shQuote(dest))
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
		if !strings.HasPrefix(line, "server_name ") {
			continue
		}
		rest := strings.TrimPrefix(line, "server_name ")
		rest = strings.TrimSuffix(strings.TrimSpace(rest), ";")
		for _, name := range strings.Fields(rest) {
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}
