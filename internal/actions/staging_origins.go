package actions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

const ghRepo = "D8-X/backend-nginx-infra-config"

type ghFileResponse struct {
	Content string `json:"content"`
	SHA     string `json:"sha"`
}

func (c *Container) UpdateStagingOrigins(ctx *cli.Context) error {
	styles.PrintCommandTitle("Manage whitelisted origins")

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN is required in .env file")
	}

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		cfg = &configs.D8XConfig{}
	}

	apiKey := os.Getenv("NGINX_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("NGINX_API_KEY is required in .env file")
	}

	env, err := c.EnsureEnvironment(cfg)
	if err != nil {
		return err
	}

	nginxConfPath := env + "/nginx.conf"
	stagingPath := env + "/staging_origins.map"

	nginxConf, err := ghReadFile(token, nginxConfPath)
	if err != nil {
		fmt.Println(styles.ItalicText.Render("Could not read nginx.conf from GitHub: " + err.Error()))
	} else {
		prodOrigins := parseProductionOrigins(nginxConf.Content)
		if len(prodOrigins) > 0 {
			fmt.Println(styles.ItalicText.Render("\nProduction origins (nginx.conf):"))
			for _, o := range prodOrigins {
				fmt.Printf("  %s\n", o)
			}
		}
	}

	stagingFile, err := ghReadFile(token, stagingPath)
	var current []string
	var currentSHA string
	if err != nil {
		fmt.Println(styles.ItalicText.Render("Could not read staging_origins.map from GitHub, starting fresh"))
	} else {
		current = parseOrigins(stagingFile.Content)
		currentSHA = stagingFile.SHA
	}

	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}

	for {
		if len(current) > 0 {
			fmt.Println(styles.ItalicText.Render("\nStaging origins:"))
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

		sel, err := c.TUI.NewSelection(actions, components.SelectionOptAllowOnlySingleItem(), components.SelectionOptRequireSelection())
		if err != nil {
			return err
		}
		action := sel[0]

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
			stagingContent := buildOriginsFile(current)

			oldContent := ""
			if stagingFile != nil {
				oldContent = stagingFile.Content
			}
			if stagingContent != oldContent {
				newSHA, err := ghWriteFile(token, stagingPath, stagingContent, currentSHA)
				if err != nil {
					return fmt.Errorf("pushing to GitHub: %w", err)
				}
				currentSHA = newSHA
				fmt.Println(styles.SuccessText.Render("Pushed staging_origins.map to GitHub."))
			} else {
				fmt.Println(styles.ItalicText.Render("No changes to staging origins, skipping GitHub push."))
			}

			// Deploy only staging_origins.map and auth_check.conf (don't touch nginx.conf/sites.conf to preserve SSL)
			password := c.UserPassword
			if password == "" {
				password, _ = c.GetPassword(ctx)
			}
			if password == "" {
				fmt.Println("Enter server sudo password:")
				password, err = c.TUI.NewInput(components.TextInputOptPlaceholder("password"))
				if err != nil {
					return err
				}
			}
			sshConn, err := c.CreateSSHConn(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
			if err != nil {
				return fmt.Errorf("SSH connection: %w", err)
			}

			// Deploy staging origins
			if err := sshWriteFileSudo(sshConn, password, "/etc/nginx/conf.d/staging_origins.map", stagingContent); err != nil {
				return err
			}
			fmt.Println("  deployed /etc/nginx/conf.d/staging_origins.map")

			authCheck, err := ghReadFile(token, env+"/auth_check.conf")
			if err != nil {
				return fmt.Errorf("reading auth_check.conf: %w", err)
			}
			authCheckContent := strings.ReplaceAll(authCheck.Content, "API_KEY_HERE", apiKey)
			if err := sshWriteFileSudo(sshConn, password, "/etc/nginx/auth_check.conf", authCheckContent); err != nil {
				return err
			}
			fmt.Println("  deployed /etc/nginx/auth_check.conf")

			// Reload nginx
			sshExecSudo(sshConn, password, "nginx -s reload")
			fmt.Println(styles.SuccessText.Render("Nginx reloaded."))
			return nil

		case "Exit without deploying":
			return nil
		}
	}
}

func ghListDirs(token string) ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/", ghRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(body))
	}

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}

	var dirs []string
	for _, e := range entries {
		if e.Type == "dir" {
			dirs = append(dirs, e.Name)
		}
	}
	return dirs, nil
}

func ghReadFile(token, path string) (*ghFileResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", ghRepo, path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(body))
	}

	var file ghFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("decoding base64: %w", err)
	}
	file.Content = string(decoded)
	return &file, nil
}

func ghWriteFile(token, path, content, sha string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", ghRepo, path)

	payload := map[string]string{
		"message": "update staging origins - committed by d8x-cli",
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	}
	if sha != "" {
		payload["sha"] = sha
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", url, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content struct {
			SHA string `json:"sha"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Content.SHA, nil
}

func parseProductionOrigins(nginxConf string) []string {
	var origins []string
	inMap := false
	for _, line := range strings.Split(nginxConf, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "map $http_origin") {
			inMap = true
			continue
		}
		if inMap {
			if line == "}" {
				break
			}
			if strings.HasPrefix(line, "default") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "include") || line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				origin := strings.Trim(parts[0], "\"")
				origins = append(origins, origin)
			}
		}
	}
	return origins
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
