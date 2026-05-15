package actions

import (
	"bytes"
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
		c.BitwardenStatus = "bw CLI not found in PATH (install with 'brew install bitwarden-cli')"
		fmt.Println(styles.ErrorText.Render("Bitwarden CLI ('bw') not found in PATH. Install it with 'brew install bitwarden-cli' so secrets can load automatically."))
		return nil
	}

	session := os.Getenv("BW_SESSION")
	if session == "" {
		if cached := readCachedBWSession(); cached != "" {
			session = cached
			os.Setenv("BW_SESSION", session)
		}
	}
	needUnlock := session == ""
	if !needUnlock {
		probe, err := exec.Command("bw", "get", "item", bwItemName, "--session", session).CombinedOutput()
		if err != nil {
			fmt.Println(styles.ItalicText.Render("Existing BW_SESSION appears stale; re-authenticating..."))
			_ = probe
			needUnlock = true
			session = ""
			clearCachedBWSession()
		}
	}
	if needUnlock {
		out, err := exec.Command("bw", "status").Output()
		if err != nil {
			c.BitwardenStatus = fmt.Sprintf("bw status check failed: %s", err)
			fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Bitwarden status check failed: %s. Run 'bw login' or 'bw unlock' manually.", err)))
			return nil
		}
		var status struct {
			Status string `json:"status"`
		}
		json.Unmarshal(out, &status)

		if status.Status == "unauthenticated" {
			c.BitwardenStatus = "bw vault is not logged in (run 'bw login')"
			fmt.Println(styles.ErrorText.Render("Bitwarden: not logged in. Run 'bw login' first."))
			return nil
		}

		fmt.Println("Enter Bitwarden master password:")
		masterPwd, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("master password"),
			components.TextInputOptMasked(),
		)
		if err != nil || masterPwd == "" {
			c.BitwardenStatus = "master password prompt cancelled or empty"
			fmt.Println(styles.ErrorText.Render("No master password entered; Bitwarden secrets will NOT be loaded."))
			return nil
		}

		os.Setenv("BW_TMP_PWD", masterPwd)
		unlockOut, err := exec.Command("bw", "unlock", "--raw", "--passwordenv", "BW_TMP_PWD").CombinedOutput()
		os.Unsetenv("BW_TMP_PWD")
		if err != nil {
			c.BitwardenStatus = fmt.Sprintf("bw unlock failed: %s", err)
			fmt.Println(styles.ErrorText.Render("Bitwarden unlock failed. Check your master password."))
			return nil
		}
		session = strings.TrimSpace(string(unlockOut))
		os.Setenv("BW_SESSION", session)
		if err := writeCachedBWSession(session); err != nil {
			fmt.Printf("  %s could not cache BW_SESSION to disk (%s); will re-prompt next run\n", styles.ItalicText.Render("notok"), err)
		} else {
			fmt.Println(styles.ItalicText.Render("BW_SESSION cached for subsequent runs; no master password needed until the vault re-locks."))
		}
	}

	if c.BitwardenFields == nil {
		c.BitwardenFields = make(map[string]string)
	}
	count := 0
	itemsSeen := 0
	var fetchErr error
	for _, itemName := range []string{bwPersonalItemName, bwItemName} {
		out, err := exec.Command("bw", "get", "item", itemName, "--session", session).Output()
		if err != nil {
			if itemName == bwItemName {
				fetchErr = err
				fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Bitwarden: could not fetch shared item '%s'. Error: %s", itemName, err)))
			}
			continue
		}
		if len(bytes.TrimSpace(out)) == 0 {
			continue
		}
		itemsSeen++

		var item bwItem
		if err := json.Unmarshal(out, &item); err != nil {
			if itemName == bwItemName {
				fmt.Printf("%s Bitwarden item '%s' returned unparseable JSON (%s); skipping\n", notok, itemName, err)
			}
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

	if itemsSeen == 0 {
		if fetchErr != nil {
			c.BitwardenStatus = fmt.Sprintf("could not fetch d8x-cli item (%s)", fetchErr)
		} else {
			c.BitwardenStatus = "no d8x-cli items found in vault"
		}
		fmt.Println(styles.ErrorText.Render("Bitwarden: no d8x-cli items could be loaded. Secrets will not be available."))
	} else {
		c.BitwardenStatus = fmt.Sprintf("loaded %d field(s) from %d item(s)", len(c.BitwardenFields), itemsSeen)
		if count > 0 {
			fmt.Printf("%s Loaded %d secret(s) from Bitwarden\n", ok, count)
		} else {
			fmt.Printf("%s Bitwarden items loaded; %d field(s) already matched existing env vars\n", ok, len(c.BitwardenFields))
		}
	}

	return nil
}

type BwSaveResult int

const (
	BwSaved BwSaveResult = iota
	BwUnchanged
	BwSkippedConflict
)

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

	syncCmd := exec.Command("bw", "sync", "--session", session)
	var syncStderr strings.Builder
	syncCmd.Stderr = &syncStderr
	if _, err := syncCmd.Output(); err != nil {
		return BwSkippedConflict, "", fmt.Errorf("bw sync failed: %w (stderr: %s)", err, strings.TrimSpace(syncStderr.String()))
	}

	out, err := exec.Command("bw", "get", "item", itemName, "--session", session).Output()
	if err != nil {
		return BwSkippedConflict, "", fmt.Errorf("bitwarden item '%s' not found", itemName)
	}

	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return BwSkippedConflict, "", err
	}

	fields, ok := raw["fields"].([]any)
	if !ok {
		fields = []any{}
	}
	existingIdx := -1
	existingValue := ""
	for i, f := range fields {
		field, ok := f.(map[string]any)
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
	newField := map[string]any{
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

	for _, k := range []string{
		"revisionDate", "creationDate", "deletedDate",
		"object", "attachments", "passwordHistory",
	} {
		delete(raw, k)
	}

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
	var encodeStderr strings.Builder
	cmd.Stderr = &encodeStderr
	encodedOut, err := cmd.Output()
	if err != nil {
		return BwSkippedConflict, existingValue, fmt.Errorf("bw encode failed: %w (stderr: %s)", err, strings.TrimSpace(encodeStderr.String()))
	}

	editCmd := exec.Command("bw", "edit", "item", itemID, "--session", session)
	editCmd.Stdin = strings.NewReader(string(encodedOut))
	var editStderr strings.Builder
	editCmd.Stderr = &editStderr
	if _, err := editCmd.Output(); err != nil {
		return BwSkippedConflict, existingValue, fmt.Errorf("bw edit failed: %w (stderr: %s)", err, strings.TrimSpace(editStderr.String()))
	}

	return BwSaved, existingValue, nil
}

func (c *Container) RequireBitwardenField(fieldName string) error {
	if v := os.Getenv(fieldName); v != "" {
		return nil
	}
	if c.BitwardenFields != nil {
		if v := c.BitwardenFields[fieldName]; v != "" {
			os.Setenv(fieldName, v)
			return nil
		}
	}
	status := c.BitwardenStatus
	if status == "" {
		status = "Bitwarden was not consulted"
	}
	return fmt.Errorf("%s is missing. Bitwarden status: %s. Add a '%s' field to the d8x-cli Bitwarden item (or its d8x-cli-personal counterpart) and re-run", fieldName, status, fieldName)
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
	if out, err := exec.Command("ssh-keygen", "-y", "-f", keyPath).Output(); err == nil {
		_ = os.WriteFile(keyPath+".pub", out, 0644)
	} else {
		fmt.Printf("  %s could not derive public key for %s: %s\n", notok, name, err)
	}
	return keyPath, nil
}

func bwSessionCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".d8x-cli", "session"), nil
}

func readCachedBWSession() string {
	p, err := bwSessionCachePath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeCachedBWSession(session string) error {
	p, err := bwSessionCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(session), 0600)
}

func clearCachedBWSession() {
	if p, err := bwSessionCachePath(); err == nil {
		_ = os.Remove(p)
	}
}
