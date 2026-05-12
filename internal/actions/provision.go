package actions

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

type SupportedServerProvider string

const (
	ServerProviderLinode SupportedServerProvider = "linode"
	ServerProviderAws    SupportedServerProvider = "aws"
)

// Default terraform files directory without trailing slash
const TF_FILES_DIR = "./terraform"

func (c *Container) Provision(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting provisioning...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	env, err := c.EnsureEnvironment(cfg)
	if err != nil {
		return err
	}
	if err := c.ensureSSHKey(env); err != nil {
		return err
	}

	if err := c.Input.CollectProvisioningData(ctx); err != nil {
		return err
	}
	providerConfigurer := c.Input.GetServerProviderConfigurer()
	if providerConfigurer == nil {
		return fmt.Errorf("misconfigured server provider details")
	}

	// Terraform apply for selected server provider
	tfCmd, err := providerConfigurer.BuildTerraformCMD(c)
	if err != nil {
		return err
	}

	// Terraform init must run after we copy all the terraform files via
	// BuildTerraformCMD
	tfInit := exec.Command("terraform", "init")
	tfInit.Dir = c.ProvisioningTfDir
	connectCMDToCurrentTerm(tfInit)
	if err := c.RunCmd(tfInit); err != nil {
		return err
	}

	if tfCmd != nil {
		// Set the tf dir
		tfCmd.Dir = c.ProvisioningTfDir

		connectCMDToCurrentTerm(tfCmd)
		err := c.RunCmd(tfCmd)
		if err != nil {
			fmt.Println(styles.ErrorText.Render("Terraform apply failed, please check the output above for more details.\nPossible issues:\n\tDuplicate server label\n\tIncorrect server provider credentials\n\tSelected region was used first time"))
			return err
		}
	}

	// Set the provisioning time
	c.provisioningTime = time.Now()

	if hostsContent, herr := os.ReadFile(configs.DEFAULT_HOSTS_FILE); herr == nil {
		c.HostsCfg = files.NewMemHostsFileInteractor(hostsContent, hostsCfgGitHubPusher(c.SelectedEnv))
	}

	// Perform provider dependent actions
	if err := providerConfigurer.PostProvisioningAction(c); err != nil {
		return err
	}

	// Update the input
	if err := c.Input.PostProvisioningHook(); err != nil {
		return err
	}

	if token := os.Getenv("GITHUB_TOKEN"); token != "" && c.SelectedEnv != "" {
		hostsContent, err := os.ReadFile(configs.DEFAULT_HOSTS_FILE)
		if err == nil {
			path := c.SelectedEnv + "/hosts.cfg"
			existing, _ := ghReadFile(token, path)
			sha := ""
			if existing != nil {
				sha = existing.SHA
				if existing.Content == string(hostsContent) {
					fmt.Printf("  %s hosts.cfg already up to date on GitHub (%s)\n", ok, path)
					return nil
				}
			}
			_, err := ghWriteFile(token, path, string(hostsContent), sha, "update "+path)
			if err != nil {
				fmt.Printf("  %s Could not push hosts.cfg to GitHub: %s\n", notok, err)
			} else {
				fmt.Printf("  %s hosts.cfg pushed to GitHub (%s)\n", ok, path)
			}
		}
	}

	return nil
}

func hostsCfgGitHubPusher(env string) func(content string) error {
	return func(content string) error {
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" || env == "" {
			return nil
		}
		path := env + "/hosts.cfg"
		sha := ""
		if existing, _ := ghReadFile(token, path); existing != nil {
			sha = existing.SHA
			if existing.Content == content {
				fmt.Printf("%s %s already up to date in infra repo\n", ok, path)
				return nil
			}
		}
		fmt.Printf("%s overwriting %s in infra repo\n", warning, path)
		if _, err := ghWriteFile(token, path, content, sha, "update "+path); err != nil {
			return fmt.Errorf("pushing hosts.cfg to infra repo: %w", err)
		}
		fmt.Printf("%s pushed %s to infra repo\n", ok, path)
		return nil
	}
}

// ServerProviderConfigurer
type ServerProviderConfigurer interface {
	//  BuildTerraformCMD generates neccessary files and configs to start
	// terraform provisioning. Returned exec.Cmd can be used to execute
	// terraform apply
	BuildTerraformCMD(*Container) (*exec.Cmd, error)

	// PostProvisioningAction is called once BuildTerraformCMD Cmd is executed
	// successfuly. This method is used to perform provider specific actions
	// after the provisioning.
	PostProvisioningAction(*Container) error
}

// CollectNumberOfWorkers collects number of workers input from user
func (c *InputCollector) CollectNumberOfWorkers(defaultNum string) (int, error) {
	fmt.Println("Enter number of worker servers to create: ")
	numWorkers, err := c.TUI.NewInput(
		components.TextInputOptValue(defaultNum),
		components.TextInputOptPlaceholder("4"),
		components.TextInputOptValidation(func(s string) bool {
			_, err := strconv.Atoi(s)
			return err == nil
		}, "please provide a valid number"),
	)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(numWorkers)
}
