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

// Init performs initialization of configuration files, installation of
// dependencies.
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

	// MACOS
	if strings.Contains(runtime.GOOS, "darwin") {
		if !tfFound || !ansibleFound {
			return fmt.Errorf("ansible or terraform is not installed on the system")
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

		fmt.Printf("Attempting to install: %v\n", strings.Join(selected, ", "))
		for _, dep := range selected {
			switch dep {
			case "terraform":
				if err := c.installTerraform(); err != nil {
					return fmt.Errorf("Installing terraform: %w\n please refer to official guides on manual terraform installation: https://developer.hashicorp.com/terraform/downloads", err)
				}
			case "ansible":
				if err := c.installAnsible(); err != nil {
					return fmt.Errorf("Installing ansible: %w\n please refer to official guides on manual ansible installation: https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html", err)
				}
			}
		}
	}

	c.MakeConfigDir()

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

	// Assume we have apt by default
	sh := `
wget -O- https://apt.releases.hashicorp.com/gpg | gpg --batch --yes --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | tee /etc/apt/sources.list.d/hashicorp.list
apt update && apt install terraform
`
	// Yum/Centos
	if _, err := exec.LookPath("yum"); err == nil {
		sh = `
yum install -y yum-utils
yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
yum -y install terraform
		`
	}
	// DNF/fedora
	if _, err := exec.LookPath("dnf"); err == nil {
		sh = `
yum install -y yum-utils
yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
yum -y install terraform
		`
	}

	// Make temp script file
	f, err := os.CreateTemp("", "d8x-installation-XXXX.sh")
	if err != nil {
		return fmt.Errorf("creating temporary file for terraform script: %w", err)
	}
	defer f.Close()
	defer func() {
		os.Remove(f.Name())
	}()
	if err := f.Chmod(0700); err != nil {
		return fmt.Errorf("creating temporary file for terraform script: %w", err)
	}
	_, err = f.Write([]byte(sh))
	if err != nil {
		return fmt.Errorf("could not create installation script: %w", err)
	}

	cmd := exec.Command("sudo", "bash", f.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Connect stdin for sudo pass
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// installAnsible attempts to install ansible
func (c *Container) installAnsible() error {
	// Ansible galaxy collections which should be installed
	ansibleCollections := []string{
		"community.docker",
		"ansible.posix",
		"community.general",
	}

	// Check for python3
	if c.findInPath("python3") != nil {
		return fmt.Errorf("python3 was not found in path")
	}

	// Attempt to install pipx
	if c.findInPath("pipx") != nil {
		fmt.Println(styles.ItalicText.Render("Installing pipx"))
		cmd := exec.Command("python3", expandCMD("-m pip install pipx passlib")...)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("installing pipx: %w", err)
		} else {
			fmt.Println(styles.SuccessText.Render("pipx was installed"))
		}
	}

	// Install ansible
	if c.findInPath("ansible", "ansible-playbook", "ansible-galaxy") != nil {
		fmt.Println(styles.ItalicText.Render("Installing ansible"))
		cmd := exec.Command("pipx", expandCMD("install --include-deps ansible")...)
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("installing ansible: %w", err)
		} else {
			fmt.Println(styles.SuccessText.Render("ansible was installed"))
		}
	}

	// Install ansible galaxy collections
	fmt.Println(styles.ItalicText.Render("Installing ansible-galaxy collections"))
	collectionsArgs := append([]string{"collection", "install"}, ansibleCollections...)
	if err := exec.Command("ansible-galaxy", collectionsArgs...).Run(); err != nil {
		return fmt.Errorf("installing ansible galaxy collections: %w", err)
	} else {
		fmt.Println(styles.SuccessText.Render("ansible galaxy collections were installed"))
	}

	return nil
}

// MakeConfigDir creates configuration directory if it does not exist yet
func (c *Container) MakeConfigDir() error {
	_, err := os.Stat(c.ConfigDir)
	if err != nil {
		if err := os.MkdirAll(c.ConfigDir, 0776); err != nil {
			return err
		}
		fmt.Println(styles.SuccessText.Render(
			fmt.Sprintf("Configuration directory was created at %s\n", c.ConfigDir),
		))
	}

	return nil
}
