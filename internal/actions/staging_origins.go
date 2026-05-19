package actions

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

const defaultGhRepo = "D8-X/backend-nginx-infra-config"

const cliCommitPrefix = "[D8X CLI]- "

func ghEscapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func getGhRepo() string {
	if repo := os.Getenv("INFRA_REPO"); repo != "" {
		return repo
	}
	return defaultGhRepo
}

func prefixCommitMsg(msg string) string {
	if strings.HasPrefix(msg, cliCommitPrefix) {
		return msg
	}
	return cliCommitPrefix + msg
}

type ghFileResponse struct {
	Content string `json:"content"`
	SHA     string `json:"sha"`
}

func (c *Container) UpdateStagingOrigins(ctx *cli.Context) error {
	styles.PrintCommandTitle("Manage whitelisted origins")

	if err := c.RequireBitwardenField("GITHUB_TOKEN"); err != nil {
		return err
	}
	token := os.Getenv("GITHUB_TOKEN")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		cfg = &configs.D8XConfig{}
	}

	if err := c.RequireBitwardenField("NGINX_API_KEY"); err != nil {
		return err
	}
	apiKey := os.Getenv("NGINX_API_KEY")

	env, err := c.EnsureEnvironment(cfg)
	if err != nil {
		return err
	}
	if err := c.RequireProvisionedHosts("staging-origins", "manager"); err != nil {
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
				if _, err := ghWriteFile(token, stagingPath, stagingContent, currentSHA, "update "+stagingPath); err != nil {
					return fmt.Errorf("pushing to GitHub: %w", err)
				}
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
				password, err = c.TUI.NewInput(
					components.TextInputOptPlaceholder("password"),
					components.TextInputOptMasked(),
					components.TextInputOptDenyEmpty(),
				)
				if err != nil {
					return err
				}
				if os.Getenv("BW_SESSION") != "" && c.SelectedEnv != "" {
					fieldName := "SERVER_PASSWORD_" + strings.ToUpper(c.SelectedEnv)
					if err := saveAndReport(fieldName, password); err != nil {
						fmt.Printf("%s warning: could not save SERVER_PASSWORD to Bitwarden (%s): %s\n", warning, fieldName, err)
					}
				}
				c.UserPassword = password
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
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/", getGhRepo())
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
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", getGhRepo(), ghEscapePath(path))
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

func ghWriteFile(token, path, content, sha, commitMsg string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", getGhRepo(), ghEscapePath(path))

	payload := map[string]string{
		"message": prefixCommitMsg(commitMsg),
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	}
	if sha != "" {
		payload["sha"] = sha
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshalling payload: %w", err)
	}
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

type ghCommitFile struct {
	Path    string
	Content string
}

func ghCommitFiles(token string, files []ghCommitFile, message string) error {
	if len(files) == 0 {
		return nil
	}
	repo := getGhRepo()
	base := fmt.Sprintf("https://api.github.com/repos/%s", repo)

	changedFiles := make([]ghCommitFile, 0, len(files))
	for _, f := range files {
		existing, err := ghReadFile(token, f.Path)
		if err == nil && normalizeContentForCompare(existing.Content) == normalizeContentForCompare(f.Content) {
			continue
		}
		changedFiles = append(changedFiles, f)
	}
	if len(changedFiles) == 0 {
		return nil
	}

	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := ghAPI(token, "GET", base, nil, &repoInfo); err != nil {
		return fmt.Errorf("get repo: %w", err)
	}
	branch := repoInfo.DefaultBranch
	if branch == "" {
		branch = "main"
	}

	var refResp struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := ghAPI(token, "GET", base+"/git/ref/heads/"+branch, nil, &refResp); err != nil {
		return fmt.Errorf("get ref: %w", err)
	}
	headSHA := refResp.Object.SHA

	var commitInfo struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := ghAPI(token, "GET", base+"/git/commits/"+headSHA, nil, &commitInfo); err != nil {
		return fmt.Errorf("get head commit: %w", err)
	}
	baseTreeSHA := commitInfo.Tree.SHA

	treeItems := make([]map[string]any, 0, len(changedFiles))
	for _, f := range changedFiles {
		var blob struct {
			SHA string `json:"sha"`
		}
		if err := ghAPI(token, "POST", base+"/git/blobs", map[string]string{
			"content":  f.Content,
			"encoding": "utf-8",
		}, &blob); err != nil {
			return fmt.Errorf("create blob for %s: %w", f.Path, err)
		}
		treeItems = append(treeItems, map[string]any{
			"path": f.Path,
			"mode": "100644",
			"type": "blob",
			"sha":  blob.SHA,
		})
	}

	var treeResp struct {
		SHA string `json:"sha"`
	}
	if err := ghAPI(token, "POST", base+"/git/trees", map[string]any{
		"base_tree": baseTreeSHA,
		"tree":      treeItems,
	}, &treeResp); err != nil {
		return fmt.Errorf("create tree: %w", err)
	}

	if treeResp.SHA == baseTreeSHA {
		return nil
	}

	var newCommit struct {
		SHA string `json:"sha"`
	}
	if err := ghAPI(token, "POST", base+"/git/commits", map[string]any{
		"message": prefixCommitMsg(message),
		"tree":    treeResp.SHA,
		"parents": []string{headSHA},
	}, &newCommit); err != nil {
		return fmt.Errorf("create commit: %w", err)
	}

	if err := ghAPI(token, "PATCH", base+"/git/refs/heads/"+branch, map[string]string{
		"sha": newCommit.SHA,
	}, nil); err != nil {
		if status := ghStatusCode(err); status == 409 || status == 422 {
			return fmt.Errorf("update ref: branch %q moved during commit (status %d). Re-run to retry on a fresh ref. Underlying: %w", branch, status, err)
		}
		return fmt.Errorf("update ref: %w", err)
	}
	return nil
}

func normalizeContentForCompare(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n")
}

func ghCommitDeletes(token string, paths []string, message string) error {
	if len(paths) == 0 {
		return nil
	}
	repo := getGhRepo()
	base := fmt.Sprintf("https://api.github.com/repos/%s", repo)

	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := ghAPI(token, "GET", base, nil, &repoInfo); err != nil {
		return fmt.Errorf("get repo: %w", err)
	}
	branch := repoInfo.DefaultBranch
	if branch == "" {
		branch = "main"
	}

	var refResp struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := ghAPI(token, "GET", base+"/git/ref/heads/"+branch, nil, &refResp); err != nil {
		return fmt.Errorf("get ref: %w", err)
	}
	headSHA := refResp.Object.SHA

	var commitInfo struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := ghAPI(token, "GET", base+"/git/commits/"+headSHA, nil, &commitInfo); err != nil {
		return fmt.Errorf("get head commit: %w", err)
	}
	baseTreeSHA := commitInfo.Tree.SHA

	treeItems := make([]map[string]any, 0, len(paths))
	for _, p := range paths {
		treeItems = append(treeItems, map[string]any{
			"path": p,
			"mode": "100644",
			"type": "blob",
			"sha":  nil,
		})
	}

	var treeResp struct {
		SHA string `json:"sha"`
	}
	if err := ghAPI(token, "POST", base+"/git/trees", map[string]any{
		"base_tree": baseTreeSHA,
		"tree":      treeItems,
	}, &treeResp); err != nil {
		return fmt.Errorf("create tree: %w", err)
	}
	if treeResp.SHA == baseTreeSHA {
		return nil
	}

	var newCommit struct {
		SHA string `json:"sha"`
	}
	if err := ghAPI(token, "POST", base+"/git/commits", map[string]any{
		"message": prefixCommitMsg(message),
		"tree":    treeResp.SHA,
		"parents": []string{headSHA},
	}, &newCommit); err != nil {
		return fmt.Errorf("create commit: %w", err)
	}
	if err := ghAPI(token, "PATCH", base+"/git/refs/heads/"+branch, map[string]string{
		"sha": newCommit.SHA,
	}, nil); err != nil {
		if status := ghStatusCode(err); status == 409 || status == 422 {
			return fmt.Errorf("update ref: branch %q moved during commit (status %d). Re-run to retry on a fresh ref. Underlying: %w", branch, status, err)
		}
		return fmt.Errorf("update ref: %w", err)
	}
	return nil
}

func ghListEnvFiles(token, env string) ([]string, error) {
	repo := getGhRepo()
	var treeResp struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/HEAD?recursive=1", repo)
	if err := ghAPI(token, "GET", url, nil, &treeResp); err != nil {
		return nil, err
	}
	prefix := env + "/"
	var paths []string
	for _, e := range treeResp.Tree {
		if e.Type != "blob" {
			continue
		}
		if strings.HasPrefix(e.Path, prefix) {
			paths = append(paths, e.Path)
		}
	}
	return paths, nil
}

type ghAPIError struct {
	StatusCode int
	Body       string
}

func (e *ghAPIError) Error() string {
	return fmt.Sprintf("GitHub API %d: %s", e.StatusCode, e.Body)
}

func ghAPI(token, method, url string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return &ghAPIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return err
		}
	}
	return nil
}

func ghStatusCode(err error) int {
	var apiErr *ghAPIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}

func ghDeleteFile(token, path, sha, commitMsg string) error {
	if sha == "" {
		return fmt.Errorf("ghDeleteFile requires a sha")
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", getGhRepo(), ghEscapePath(path))

	payload := map[string]string{
		"message": prefixCommitMsg(commitMsg),
		"sha":     sha,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}
	req, err := http.NewRequest("DELETE", url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
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
	return slices.Contains(list, item)
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
