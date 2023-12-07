package actions

import (
	"fmt"
	"os"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/flags"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

// getPublicKey returns the public key contents
func getPublicKey(sshKeyPath string) (string, error) {
	pubkeyfile := fmt.Sprintf("%s.pub", sshKeyPath)
	pub, err := os.ReadFile(pubkeyfile)
	if err != nil {
		return "", fmt.Errorf("reading public key %s: %w", pubkeyfile, err)
	}
	return strings.TrimSpace(string(pub)), nil
}

func (c *Container) DisplayPasswordAlert() {
	if len(c.UserPassword) == 0 {
		return
	}
	fmt.Printf("User: %s\n", c.DefaultClusterUserName)
	fmt.Printf("Password: %s\n", c.UserPassword)
}

// Get password gets the password with the following precedence:
// 1. --password flag
// 2. ./password.txt file in cwd
func defaultPasswordGetter(ctx *cli.Context) (string, error) {
	if pwd := ctx.String(flags.Password); pwd != "" {
		return pwd, nil
	}
	if pwd, err := os.ReadFile(configs.DEFAULT_PASSWORD_FILE); err != nil {
		return "", fmt.Errorf("could not retrieve the password: %w", err)
	} else {
		return string(pwd), nil
	}
}

// TrimHttpsPrefix removes http:// or https:// prefix from the url
func TrimHttpsPrefix(url string) string {
	return strings.TrimSpace(strings.TrimPrefix(
		strings.TrimPrefix(url, "http://"),
		"https://",
	))
}

// EnsureHttpsPrefixExists makes sure the url has https:// prefix
func EnsureHttpsPrefixExists(url string) string {
	return "https://" + TrimHttpsPrefix(url)
}

// ValidateHttp validates if given url starts with http:// or https://
func ValidateHttp(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// CollectAndValidatePrivateKey prompts user to enter a private key, validates
// it, displays the address of entered key and prompts user to confirm that
// entered key's address is correct. If any of the validation or
// confirmation steps fail, it will restart the collection process. Returned
// values are private key without 0x prefix and its address.
func (c *Container) CollectAndValidatePrivateKey(title string) (string, string, error) {
	fmt.Println(title)
	pk, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
		components.TextInputOptMasked(),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return "", "", err
	}
	pk = strings.TrimPrefix(pk, "0x")
	addr, err := PrivateKeyToAddress(pk)
	if err != nil {
		info := fmt.Sprintf("Invalid private key, please try again...\n - %s\n", err.Error())
		fmt.Println(styles.ErrorText.Render(info))
		return c.CollectAndValidatePrivateKey(title)
	}

	fmt.Printf("Wallet address of entered private key: %s\n", addr.Hex())

	ok, err := c.TUI.NewPrompt("Is this the correct address?", true)
	if err != nil {
		return "", "", err
	}

	if !ok {
		return c.CollectAndValidatePrivateKey(title)
	}

	return pk, addr.Hex(), nil
}
