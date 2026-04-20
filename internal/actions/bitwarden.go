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

	if c.BitwardenFields == nil {
		c.BitwardenFields = make(map[string]string)
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
			if field.Name == "" || field.Value == "" {
				continue
			}
			if _, exists := c.BitwardenFields[field.Name]; !exists {
				c.BitwardenFields[field.Name] = field.Value
			}

			if os.Getenv(field.Name) != "" {
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

type BwSaveResult int

const (
	BwSaved BwSaveResult = iota
	BwUnchanged
	BwSkippedConflict
)

func SaveSecretToBitwarden(fieldName, fieldValue string) (BwSaveResult, string, error) {
	return saveBitwardenField(bwItemName, fieldName, fieldValue, false)
}

func SaveSecretToBitwardenPersonal(fieldName, fieldValue string) (BwSaveResult, string, error) {
	return saveBitwardenField(bwPersonalItemName, fieldName, fieldValue, false)
}

func SaveSecretToBitwardenItem(itemName, fieldName, fieldValue string) (BwSaveResult, string, error) {
	return saveBitwardenField(itemName, fieldName, fieldValue, false)
}

func ForceOverwriteBitwarden(fieldName, fieldValue string) (BwSaveResult, string, error) {
	return saveBitwardenField(bwItemName, fieldName, fieldValue, true)
}

func saveBitwardenField(itemName, fieldName, fieldValue string, overwrite bool) (BwSaveResult, string, error) {
	session := os.Getenv("BW_SESSION")
	if session == "" {
		return BwSkippedConflict, "", fmt.Errorf("BW_SESSION is not set")
	}

	out, err := exec.Command("bw", "get", "item", itemName, "--session", session).Output()
	if err != nil {
		return BwSkippedConflict, "", fmt.Errorf("bitwarden item '%s' not found", itemName)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(out, &raw); err != nil {
		return BwSkippedConflict, "", err
	}

	fields, ok := raw["fields"].([]interface{})
	if !ok {
		fields = []interface{}{}
	}
	existingIdx := -1
	existingValue := ""
	for i, f := range fields {
		field, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		if field["name"] == fieldName {
			existingIdx = i
			if v, ok := field["value"].(string); ok {
				existingValue = v
			}
			break
		}
	}
	if existingIdx >= 0 && existingValue == fieldValue {
		return BwUnchanged, existingValue, nil
	}
	if !overwrite && existingIdx >= 0 && existingValue != "" && existingValue != fieldValue {
		return BwSkippedConflict, existingValue, nil
	}
	newField := map[string]interface{}{
		"name":  fieldName,
		"value": fieldValue,
		"type":  1,
	}
	if existingIdx >= 0 {
		fields[existingIdx] = newField
	} else {
		fields = append(fields, newField)
	}
	raw["fields"] = fields

	encoded, err := json.Marshal(raw)
	if err != nil {
		return BwSkippedConflict, existingValue, err
	}

	itemID, ok := raw["id"].(string)
	if !ok || itemID == "" {
		return BwSkippedConflict, existingValue, fmt.Errorf("bitwarden item has no ID")
	}
	cmd := exec.Command("bw", "encode")
	cmd.Stdin = strings.NewReader(string(encoded))
	encodedOut, err := cmd.Output()
	if err != nil {
		return BwSkippedConflict, existingValue, fmt.Errorf("bw encode failed: %w", err)
	}

	editCmd := exec.Command("bw", "edit", "item", itemID, "--session", session)
	editCmd.Stdin = strings.NewReader(string(encodedOut))
	if _, err := editCmd.Output(); err != nil {
		return BwSkippedConflict, existingValue, fmt.Errorf("bw edit failed: %w", err)
	}

	return BwSaved, existingValue, nil
}

func saveAndReport(fieldName, value string) error {
	return saveAndReportTo(bwItemName, fieldName, value)
}

func saveAndReportPersonal(fieldName, value string) error {
	return saveAndReportTo(bwPersonalItemName, fieldName, value)
}

func saveAndReportTo(itemName, fieldName, value string) error {
	result, _, err := SaveSecretToBitwardenItem(itemName, fieldName, value)
	if err != nil {
		fmt.Printf("  %s Could not save %s to Bitwarden item '%s': %s\n", notok, fieldName, itemName, err)
		return err
	}
	switch result {
	case BwSaved:
		fmt.Printf("  %s Saved to Bitwarden item '%s' as %s\n", ok, itemName, fieldName)
	case BwUnchanged:
		fmt.Printf("  %s %s already up to date in Bitwarden item '%s'\n", ok, fieldName, itemName)
	case BwSkippedConflict:
		fmt.Printf("  %s %s exists in Bitwarden item '%s' with a different value; NOT overwriting.\n", notok, fieldName, itemName)
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
