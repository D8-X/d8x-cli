package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/styles"
)

const bwItemName = "d8x-cli"
const bwPersonalItemName = "d8x-cli-personal"

type bwItem struct {
	Fields []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"fields"`
}

func (c *Container) LoadSecretsFromBitwarden() error {
	if _, err := exec.LookPath("bw"); err != nil {
		return nil
	}

	session := os.Getenv("BW_SESSION")
	if session == "" {
		out, err := exec.Command("bw", "status").Output()
		if err != nil {
			return nil
		}
		var status struct {
			Status string `json:"status"`
		}
		json.Unmarshal(out, &status)

		if status.Status == "unauthenticated" {
			fmt.Println(styles.ItalicText.Render("Bitwarden: not logged in. Run 'bw login' first."))
			return nil
		}

		fmt.Println("Enter Bitwarden master password:")
		masterPwd, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("master password"),
			components.TextInputOptMasked(),
		)
		if err != nil || masterPwd == "" {
			return nil
		}

		os.Setenv("BW_TMP_PWD", masterPwd)
		out, err = exec.Command("bw", "unlock", "--raw", "--passwordenv", "BW_TMP_PWD").CombinedOutput()
		os.Unsetenv("BW_TMP_PWD")
		if err != nil {
			fmt.Println(styles.ErrorText.Render("Bitwarden unlock failed. Check your master password."))
			return nil
		}
		session = strings.TrimSpace(string(out))
		os.Setenv("BW_SESSION", session)
	}

	count := 0
	for _, itemName := range []string{bwPersonalItemName, bwItemName} {
		out, err := exec.Command("bw", "get", "item", itemName, "--session", session).Output()
		if err != nil {
			continue
		}

		var item bwItem
		if err := json.Unmarshal(out, &item); err != nil {
			continue
		}

		for _, field := range item.Fields {
			if field.Name == "" || field.Value == "" || os.Getenv(field.Name) != "" {
				continue
			}

			if strings.HasPrefix(field.Name, "SSH_KEY_") {
				keyPath, err := writeSSHKeyToTempFile(field.Name, field.Value)
				if err != nil {
					fmt.Printf("  %s failed to write SSH key %s: %s\n", notok, field.Name, err)
					continue
				}
				os.Setenv(field.Name, keyPath)
				count++
				continue
			}

			os.Setenv(field.Name, field.Value)
			count++
		}
	}

	if count > 0 {
		fmt.Printf("%s Loaded %d secret(s) from Bitwarden\n", ok, count)
	}

	return nil
}

func SaveSecretToBitwarden(fieldName, fieldValue string) error {
	session := os.Getenv("BW_SESSION")
	if session == "" {
		return nil
	}

	out, err := exec.Command("bw", "get", "item", bwItemName, "--session", session).Output()
	if err != nil {
		return fmt.Errorf("bitwarden item '%s' not found", bwItemName)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(out, &raw); err != nil {
		return err
	}

	fields, _ := raw["fields"].([]interface{})
	for _, f := range fields {
		field, _ := f.(map[string]interface{})
		if field["name"] == fieldName {
			existing, _ := field["value"].(string)
			if existing != "" {
				fmt.Printf("  %s Bitwarden field '%s' already exists, skipping\n", warning, fieldName)
				return nil
			}
		}
	}
	fields = append(fields, map[string]interface{}{
		"name":  fieldName,
		"value": fieldValue,
		"type":  1,
	})
	raw["fields"] = fields

	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	itemID, _ := raw["id"].(string)
	cmd := exec.Command("bw", "encode")
	cmd.Stdin = strings.NewReader(string(encoded))
	encodedOut, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("bw encode failed: %w", err)
	}

	editCmd := exec.Command("bw", "edit", "item", itemID, "--session", session)
	editCmd.Stdin = strings.NewReader(string(encodedOut))
	if _, err := editCmd.Output(); err != nil {
		return fmt.Errorf("bw edit failed: %w", err)
	}

	return nil
}

func writeSSHKeyToTempFile(name, content string) (string, error) {
	dir := filepath.Join(os.TempDir(), "d8x-cli")
	os.MkdirAll(dir, 0700)

	keyPath := filepath.Join(dir, name)
	if err := os.WriteFile(keyPath, []byte(content), 0600); err != nil {
		return "", err
	}
	return keyPath, nil
}
