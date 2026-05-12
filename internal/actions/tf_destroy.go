package actions

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) TerraformDestroy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Running terraform destroy...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureEnvironment(cfg); err != nil {
		return err
	}
	if cfg.ServerProvider == "" {
		return fmt.Errorf("server_provider missing from %s/config.json in infra repo. Cannot determine which provider to destroy", c.SelectedEnv)
	}

	fmt.Println(styles.AlertImportant.Render(fmt.Sprintf("You are about to destroy environment %q.", c.SelectedEnv)))
	c.printDestroyTargets(cfg)
	fmt.Println(styles.AlertImportant.Render("All the resources listed above will be destroyed. Irreversible."))

	confirm, err := c.TUI.NewPrompt(fmt.Sprintf("Proceed with destroying %q?", c.SelectedEnv), false)
	if err != nil {
		return err
	}
	if !confirm {
		fmt.Println("Not destroying...")
		return nil
	}

	fmt.Printf("Type the environment name (%s) to confirm:\n", c.SelectedEnv)
	typed, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder(c.SelectedEnv),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return err
	}
	if strings.TrimSpace(typed) != c.SelectedEnv {
		fmt.Printf("Typed %q does not match %q. Not destroying.\n", typed, c.SelectedEnv)
		return nil
	}

	var args []string = []string{
		"destroy", "-auto-approve",
	}

	var env []string = []string{}

	switch cfg.ServerProvider {
	case configs.D8XServerProviderAWS:
		if cfg.AWSConfig == nil {
			return fmt.Errorf("aws config is not defined")
		}
		awsCfg := *cfg.AWSConfig
		awsCfg.AccesKey = readEnvSecret(c.SelectedEnv, "AWS_ACCESS_KEY")
		awsCfg.SecretKey = readEnvSecret(c.SelectedEnv, "AWS_SECRET_KEY")
		if awsCfg.AccesKey == "" || awsCfg.SecretKey == "" {
			return fmt.Errorf("AWS credentials missing: set AWS_ACCESS_KEY_%s and AWS_SECRET_KEY_%s in Bitwarden", strings.ToUpper(c.SelectedEnv), strings.ToUpper(c.SelectedEnv))
		}
		awsConfigurer := &awsConfigurer{D8XAWSConfig: awsCfg, authorizedKey: ""}
		args = append(args, awsConfigurer.generateVariables()...)

	case configs.D8XServerProviderLinode:
		args = append(args, "-var", `authorized_keys=[""]`)
		token := readEnvSecret(c.SelectedEnv, "LINODE_TOKEN")
		if token == "" {
			return fmt.Errorf("LINODE_TOKEN missing: set LINODE_TOKEN_%s in Bitwarden", strings.ToUpper(c.SelectedEnv))
		}
		env = append(env, fmt.Sprintf("LINODE_TOKEN=%s", token))
	}

	cmd := exec.Command("terraform", args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = c.ProvisioningTfDir

	connectCMDToCurrentTerm(cmd)
	if err := c.RunCmd(cmd); err != nil {
		return err
	}

	cfg.ResetDeploymentStatus()

	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return err
	}
	if err := c.PublishRemoteConfig(cfg); err != nil {
		fmt.Printf("%s warning: could not publish reset state to infra repo: %s\n", warning, err)
	}
	c.cleanupHostsAfterDestroy()
	return nil
}

func (c *Container) cleanupHostsAfterDestroy() {
	if err := os.Remove(configs.DEFAULT_HOSTS_FILE); err == nil {
		fmt.Printf("%s removed local %s\n", ok, configs.DEFAULT_HOSTS_FILE)
	} else if !os.IsNotExist(err) {
		fmt.Printf("%s warning: could not remove local %s: %s\n", warning, configs.DEFAULT_HOSTS_FILE, err)
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" || c.SelectedEnv == "" {
		return
	}
	remotePath := c.SelectedEnv + "/hosts.cfg"
	existing, err := ghReadFile(token, remotePath)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return
		}
		fmt.Printf("%s warning: could not look up %s on infra repo: %s\n", warning, remotePath, err)
		return
	}
	if err := ghDeleteFile(token, remotePath, existing.SHA, "delete "+remotePath+" - d8x tf-destroy"); err != nil {
		fmt.Printf("%s warning: could not delete %s on infra repo: %s\n", warning, remotePath, err)
		return
	}
	fmt.Printf("%s removed %s from infra repo\n", ok, remotePath)
}

func (c *Container) printDestroyTargets(cfg *configs.D8XConfig) {
	region, labelPrefix := "", ""
	switch cfg.ServerProvider {
	case configs.D8XServerProviderAWS:
		if cfg.AWSConfig != nil {
			region = cfg.AWSConfig.Region
			labelPrefix = cfg.AWSConfig.LabelPrefix
		}
	case configs.D8XServerProviderLinode:
		if cfg.LinodeConfig != nil {
			region = cfg.LinodeConfig.Region
			labelPrefix = cfg.LinodeConfig.LabelPrefix
		}
	}
	fmt.Printf("  provider:        %s\n", cfg.ServerProvider)
	if region != "" {
		fmt.Printf("  region:          %s\n", region)
	}
	if labelPrefix != "" {
		fmt.Printf("  label prefix:    %s\n", labelPrefix)
	}
	if managerIp, err := c.HostsCfg.GetMangerPublicIp(); err == nil && managerIp != "" {
		fmt.Printf("  manager:         %s\n", managerIp)
	}
	if workerIps, err := c.HostsCfg.GetWorkerIps(); err == nil && len(workerIps) > 0 {
		fmt.Printf("  workers (%d):     %s\n", len(workerIps), strings.Join(workerIps, ", "))
	}
	if brokerIp, err := c.HostsCfg.GetBrokerPublicIp(); err == nil && brokerIp != "" {
		fmt.Printf("  broker:          %s\n", brokerIp)
	}
	var deployed []string
	if cfg.SwarmDeployed {
		deployed = append(deployed, "swarm")
	}
	if cfg.SwarmNginxDeployed {
		deployed = append(deployed, "swarm-nginx")
	}
	if cfg.BrokerDeployed {
		deployed = append(deployed, "broker")
	}
	if cfg.BrokerNginxDeployed {
		deployed = append(deployed, "broker-nginx")
	}
	if cfg.MetricsDeployed {
		deployed = append(deployed, "metrics")
	}
	if len(deployed) > 0 {
		fmt.Printf("  deployed:        %s\n", strings.Join(deployed, ", "))
	}
}
