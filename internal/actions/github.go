package actions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func ghFetchDir(token, repoPath, localDir string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", getGhRepo(), repoPath)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(body))
	}

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return err
	}

	os.MkdirAll(localDir, 0755)

	for _, e := range entries {
		localPath := filepath.Join(localDir, e.Name)
		if e.Type == "dir" {
			if err := ghFetchDir(token, e.Path, localPath); err != nil {
				return err
			}
		} else {
			file, err := ghReadFile(token, e.Path)
			if err != nil {
				return fmt.Errorf("fetching %s: %w", e.Path, err)
			}
			if err := os.WriteFile(localPath, []byte(file.Content), 0600); err != nil {
				return fmt.Errorf("writing %s: %w", localPath, err)
			}
		}
	}
	return nil
}
