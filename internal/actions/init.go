package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) Init(_ *cli.Context) error {
	if err := warnOrAbortIfRunningAsRoot(c); err != nil {
		return err
	}

	tfFound := true
	ansibleFound := true

	fmt.Println("Searching for required dependencies on this system...")

	if err := c.findInPath("terraform"); err != nil {
		tfFound = false
		fmt.Println(styles.ErrorText.Render("Terraform was not found!"))
	} else {
		fmt.Println(styles.SuccessText.Render("Terraform found!"))
	}

	if err := c.findInPath("ansible", "ansible-playbook", "ansible-galaxy"); err != nil {
		ansibleFound = false
		fmt.Println(styles.ErrorText.Render("Ansible was not found!"))
	} else {
		fmt.Println(styles.SuccessText.Render("Ansible found!"))
	}

	if runtime.GOOS == "darwin" {
		if !tfFound || !ansibleFound {
			missing := []string{}
			if !tfFound {
				missing = append(missing, "terraform")
			}
			if !ansibleFound {
				missing = append(missing, "ansible")
			}
			return fmt.Errorf("missing on macOS: %s. Install via Homebrew (e.g. \"brew install %s\") and retry", strings.Join(missing, ", "), strings.Join(missing, " "))
		}
		return nil
	}

	install := []string{}
	if !tfFound {
		install = append(install, "terraform")
	}
	if !ansibleFound {
		install = append(install, "ansible")
	}
	if len(install) == 0 {
		return nil
	}

	fmt.Println(styles.SuccessText.Italic(true).MarginTop(1).Render("Select which dependencies you wish to install automatically:"))
	selected, err := c.TUI.NewSelection(install)
	if err != nil {
		return err
	}

	if len(selected) == 0 {
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf("No dependency selected for automatic install. Install manually before running other commands: %s.", strings.Join(install, ", "))))
		return fmt.Errorf("missing dependencies: %s", strings.Join(install, ", "))
	}

	fmt.Printf("Attempting to install: %v\n", strings.Join(selected, ", "))
	for _, dep := range selected {
		switch dep {
		case "terraform":
			if err := c.installTerraform(); err != nil {
				return fmt.Errorf("installing terraform: %w. See https://developer.hashicorp.com/terraform/downloads for manual install", err)
			}
		case "ansible":
			if err := c.installAnsible(); err != nil {
				return fmt.Errorf("installing ansible: %w. See https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html for manual install", err)
			}
		}
	}

	ensureLocalBinOnPath()

	stillMissing := []string{}
	if c.findInPath("terraform") != nil {
		stillMissing = append(stillMissing, "terraform")
	}
	if c.findInPath("ansible", "ansible-playbook", "ansible-galaxy") != nil {
		stillMissing = append(stillMissing, "ansible")
	}
	if len(stillMissing) > 0 {
		return fmt.Errorf("still missing after install attempt: %s. Install manually and retry", strings.Join(stillMissing, ", "))
	}

	return nil
}

func warnOrAbortIfRunningAsRoot(c *Container) error {
	if os.Geteuid() != 0 {
		return nil
	}
	sudoUser := os.Getenv("SUDO_USER")
	switch {
	case sudoUser != "" && sudoUser != "root":
		fmt.Println(styles.AlertImportant.Render(fmt.Sprintf(
			"Detected sudo invocation (SUDO_USER=%s). pipx, ansible, and ansible-galaxy install per-user; running them under root will land them in /root and your subsequent commands as %s will not find them.",
			sudoUser, sudoUser,
		)))
		proceed, err := c.TUI.NewPrompt(fmt.Sprintf("Continue installing as root anyway? (recommended: rerun as %s)", sudoUser), false)
		if err != nil {
			return err
		}
		if !proceed {
			return fmt.Errorf("aborted to avoid installing user-scoped tools under root. Rerun without sudo, as %s", sudoUser)
		}
	default:
		fmt.Println(styles.AlertImportant.Render(
			"Running as root. pipx, ansible, and ansible-galaxy install per-user. If you intend to run later d8x commands as a non-root user, they will not find these tools. Run \"d8x init\" as the same user that will run subsequent commands.",
		))
		proceed, err := c.TUI.NewPrompt("Continue installing as root anyway?", false)
		if err != nil {
			return err
		}
		if !proceed {
			return fmt.Errorf("aborted to avoid installing user-scoped tools under root. Rerun as your normal user")
		}
	}
	return nil
}

func ensureLocalBinOnPath() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	localBin := filepath.Join(home, ".local", "bin")
	if _, err := os.Stat(localBin); err != nil {
		return
	}
	path := os.Getenv("PATH")
	if slices.Contains(filepath.SplitList(path), localBin) {
		return
	}
	os.Setenv("PATH", localBin+string(os.PathListSeparator)+path)
}

// findInPath searches for executables in PATH
func (c *Container) findInPath(executable ...string) error {
	for _, exe := range executable {
		_, err := exec.LookPath(exe)
		if err != nil {
			return err
		}
	}
	return nil
}

// installTerraform attempts to install
func (c *Container) installTerraform() error {
	fmt.Println(styles.ItalicText.Render("Installing terraform..."))

	if !lookPathOk("sudo") {
		return fmt.Errorf("sudo not found in PATH. The install script requires sudo to write apt/dnf/yum repo files. Install sudo or run terraform install manually from https://developer.hashicorp.com/terraform/downloads")
	}

	sh := ""
	switch {
	case lookPathOk("dnf"):
		sh = `
set -eo pipefail
dnf install -y dnf-plugins-core
if [ -f /etc/yum.repos.d/hashicorp.repo ]; then
  echo "hashicorp.repo already exists at /etc/yum.repos.d/hashicorp.repo, skipping add-repo to avoid overwriting"
else
  dnf config-manager --add-repo https://rpm.releases.hashicorp.com/fedora/hashicorp.repo
fi
dnf -y install terraform
`
	case lookPathOk("yum"):
		sh = `
set -eo pipefail
yum install -y yum-utils
if [ -f /etc/yum.repos.d/hashicorp.repo ]; then
  echo "hashicorp.repo already exists at /etc/yum.repos.d/hashicorp.repo, skipping add-repo to avoid overwriting"
else
  yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
fi
yum -y install terraform
`
	case lookPathOk("apt"):
		sh = `
set -eo pipefail
if ! command -v lsb_release >/dev/null 2>&1; then
  apt update && apt install -y lsb-release
fi
if ! command -v wget >/dev/null 2>&1; then
  apt install -y wget
fi
if ! command -v gpg >/dev/null 2>&1; then
  apt install -y gnupg
fi
KEYRING=/usr/share/keyrings/hashicorp-archive-keyring.gpg
SOURCES_LIST=/etc/apt/sources.list.d/hashicorp.list
if [ -f "$KEYRING" ]; then
  echo "$KEYRING already exists, skipping download to avoid overwriting"
else
  wget -O- https://apt.releases.hashicorp.com/gpg | gpg --batch --dearmor -o "$KEYRING"
fi
if [ -f "$SOURCES_LIST" ]; then
  echo "$SOURCES_LIST already exists, skipping repo line write to avoid overwriting"
else
  echo "deb [signed-by=$KEYRING] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | tee "$SOURCES_LIST"
fi
apt update && apt install -y terraform
`
	default:
		return fmt.Errorf("no supported package manager found (looked for dnf, yum, apt). On Windows or other systems install terraform manually from https://developer.hashicorp.com/terraform/downloads")
	}

	f, err := os.CreateTemp("", "d8x-installation-*.sh")
	if err != nil {
		return fmt.Errorf("creating temp install script: %w", err)
	}
	defer f.Close()
	defer func() {
		os.Remove(f.Name())
	}()
	if err := f.Chmod(0600); err != nil {
		return fmt.Errorf("chmod on temp install script %s: %w", f.Name(), err)
	}
	if _, err := f.Write([]byte(sh)); err != nil {
		return fmt.Errorf("writing temp install script: %w", err)
	}

	cmd := exec.Command("sudo", "bash", f.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return c.RunCmd(cmd)
}

func lookPathOk(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// installAnsible attempts to install ansible
func (c *Container) installAnsible() error {
	ansibleCollections := []string{
		"community.docker",
		"ansible.posix",
		"community.general",
	}

	if c.findInPath("python3") != nil {
		return fmt.Errorf("python3 was not found in PATH. Install python3 first")
	}

	if c.findInPath("pipx") != nil {
		if err := c.installPipx(); err != nil {
			return err
		}
	}
	ensureLocalBinOnPath()

	if c.findInPath("ansible", "ansible-playbook", "ansible-galaxy") != nil {
		fmt.Println(styles.ItalicText.Render("Installing ansible..."))
		cmd := exec.Command("pipx", expandCMD("install --include-deps ansible")...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := c.RunCmd(cmd); err != nil {
			return fmt.Errorf("installing ansible: %w", err)
		}
		fmt.Println(styles.SuccessText.Render("ansible was installed"))
	}
	ensureLocalBinOnPath()

	fmt.Println(styles.ItalicText.Render("Injecting passlib into the ansible pipx venv (needed for the password_hash filter)..."))
	injectCmd := exec.Command("pipx", "inject", "ansible", "passlib")
	injectCmd.Stdin = os.Stdin
	injectCmd.Stdout = os.Stdout
	injectCmd.Stderr = os.Stderr
	if err := c.RunCmd(injectCmd); err != nil {
		return fmt.Errorf("injecting passlib into ansible pipx venv: %w", err)
	}

	fmt.Println(styles.ItalicText.Render("Ensuring ansible-galaxy collections are installed..."))
	collectionsArgs := append([]string{"collection", "install"}, ansibleCollections...)
	cmd := exec.Command("ansible-galaxy", collectionsArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := c.RunCmd(cmd); err != nil {
		return fmt.Errorf("installing ansible galaxy collections: %w", err)
	}
	fmt.Println(styles.SuccessText.Render("ansible-galaxy collections are up to date"))

	return nil
}

func (c *Container) installPipx() error {
	fmt.Println(styles.ItalicText.Render("Installing pipx..."))

	if lookPathOk("sudo") {
		var distroCmd *exec.Cmd
		switch {
		case lookPathOk("apt"):
			distroCmd = exec.Command("sudo", "bash", "-c", "apt update && apt install -y pipx")
		case lookPathOk("dnf"):
			distroCmd = exec.Command("sudo", "dnf", "install", "-y", "pipx")
		case lookPathOk("yum"):
			distroCmd = exec.Command("sudo", "yum", "install", "-y", "pipx")
		}
		if distroCmd != nil {
			distroCmd.Stdin = os.Stdin
			distroCmd.Stdout = os.Stdout
			distroCmd.Stderr = os.Stderr
			if err := c.RunCmd(distroCmd); err == nil {
				fmt.Println(styles.SuccessText.Render("pipx was installed via the system package manager"))
				return nil
			} else {
				fmt.Println(styles.ItalicText.Render(fmt.Sprintf("System package install failed (%s), falling back to pip user install...", err)))
			}
		}
	}

	cmd := exec.Command("python3", expandCMD("-m pip install --user --break-system-packages pipx")...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := c.RunCmd(cmd); err != nil {
		return fmt.Errorf("installing pipx via pip: %w. If you saw \"No module named pip\", install python3-pip via your distro package manager first", err)
	}
	fmt.Println(styles.SuccessText.Render("pipx was installed via pip --user"))
	return nil
}

