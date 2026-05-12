package actions

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) Init(ctx *cli.Context) error {
	tfFound := true
	ansibleFound := true

	fmt.Println("Searching for required dependencies on this system...")

	if err := c.findInPath("terraform"); err != nil {
		tfFound = false
		fmt.Println(styles.ErrorText.Render("Terraform was not found!"))
	} else {
		fmt.Println(styles.SuccessText.Render("Terraform found!"))
	}

	if err := c.findInPath("ansible", "ansible-playbook"); err != nil {
		ansibleFound = false
		fmt.Println(styles.ErrorText.Render("Ansible was not found!"))
	} else {
		fmt.Println(styles.SuccessText.Render("Ansible found!"))
	}

	if strings.Contains(runtime.GOOS, "darwin") {
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

	if !tfFound || !ansibleFound {
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
	}

	return nil
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

	sh := ""
	switch {
	case lookPathOk("dnf"):
		sh = `
dnf install -y dnf-plugins-core
dnf config-manager --add-repo https://rpm.releases.hashicorp.com/fedora/hashicorp.repo
dnf -y install terraform
`
	case lookPathOk("yum"):
		sh = `
yum install -y yum-utils
yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
yum -y install terraform
`
	case lookPathOk("apt"):
		sh = `
set -e
if ! command -v lsb_release >/dev/null 2>&1; then
  apt update && apt install -y lsb-release
fi
wget -O- https://apt.releases.hashicorp.com/gpg | gpg --batch --yes --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | tee /etc/apt/sources.list.d/hashicorp.list
apt update && apt install -y terraform
`
	default:
		return fmt.Errorf("no supported package manager found (looked for dnf, yum, apt). Install terraform manually from https://developer.hashicorp.com/terraform/downloads")
	}

	f, err := os.CreateTemp("", "d8x-installation-*.sh")
	if err != nil {
		return fmt.Errorf("creating temp install script: %w", err)
	}
	defer f.Close()
	defer func() {
		os.Remove(f.Name())
	}()
	if err := f.Chmod(0700); err != nil {
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
		fmt.Println(styles.ItalicText.Render("Installing pipx..."))
		cmd := exec.Command("python3", expandCMD("-m pip install pipx passlib")...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := c.RunCmd(cmd); err != nil {
			return fmt.Errorf("installing pipx: %w", err)
		}
		fmt.Println(styles.SuccessText.Render("pipx was installed"))
	}

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

	fmt.Println(styles.ItalicText.Render("Installing ansible-galaxy collections..."))
	collectionsArgs := append([]string{"collection", "install"}, ansibleCollections...)
	cmd := exec.Command("ansible-galaxy", collectionsArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := c.RunCmd(cmd); err != nil {
		return fmt.Errorf("installing ansible galaxy collections: %w", err)
	}
	fmt.Println(styles.SuccessText.Render("ansible galaxy collections were installed"))

	return nil
}

