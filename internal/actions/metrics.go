package actions

import (
	"fmt"
	"strings"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

// Stack name for metrics services deployed on manager
var dockerMetricsStackName = "metrics"

// DeployMetrics copies prometheus config and redeploys prometheus service.
// Prometheus deployment is separated from main swarm deployment because we want
// to run it on manager, and it is set to drainer availability by default (in
// ansible setup)
func (c *Container) DeployMetrics(ctx *cli.Context) error {
	fmt.Println("Deploying prometheus and grafana on manager...")

	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return fmt.Errorf("finding manager ip address: %w", err)
	}

	manager, err := c.CreateSSHConn(
		managerIp,
		c.DefaultClusterUserName,
		c.SshKeyPath,
	)
	if err != nil {
		return err
	}

	filesToCopy := []files.EmbedCopierOp{
		// Metrics (grafana/prometheus) stack
		{Src: "embedded/docker-swarm-metrics.yml", Dst: "./docker-swarm-metrics.yml", Overwrite: true},
		// Prometheus config
		{Src: "embedded/prometheus.yml", Dst: "./prometheus.yml", Overwrite: true},

		// All things grafana
		{Src: "embedded/grafana/datasource-prometheus.yml", Dst: "./grafana/datasource-prometheus.yml", Overwrite: true},
	}
	if err := c.EmbedCopier.Copy(configs.EmbededConfigs, filesToCopy...); err != nil {
		return fmt.Errorf("copying configs to local file system: %w", err)
	}

	if err := manager.CopyFilesOverSftp(
		conn.SftpCopySrcDest{Src: "./prometheus.yml", Dst: "./prometheus.yml"},
		conn.SftpCopySrcDest{Src: "./docker-swarm-metrics.yml", Dst: "./docker-swarm-metrics.yml"},

		conn.SftpCopySrcDest{Src: "./grafana/datasource-prometheus.yml", Dst: "./grafana/datasource-prometheus.yml"},
	); err != nil {
		return fmt.Errorf("copying prometheus config to manager: %w", err)
	}

	// Toggle manager availability for this deployment
	if out, err := manager.ExecCommand("docker node update --availability active manager-1"); err != nil {
		fmt.Println(string(out))
		return fmt.Errorf("setting manager-1 availability to active: %w", err)
	}
	defer func() {
		if out, err := manager.ExecCommand("docker node update --availability pause manager-1"); err != nil {
			fmt.Println(string(out))
			fmt.Println(
				styles.ErrorText.Render(
					fmt.Sprintf("setting manager-1 availability to active: %s", err.Error()),
				),
			)
		}
	}()

	// Re-Create prometheus_config and deploy metrics stack (docker-swarm-metrics.yml)
	cmdLines := []string{
		// Create prometheus data volume and don't remove it
		"docker volume create prometheus_data_vol",

		"docker stack rm " + dockerMetricsStackName,
		"docker config rm prometheus_config",
		"docker config create prometheus_config ./prometheus.yml >/dev/null 2>&1",
		"sleep 5; docker stack deploy -c docker-swarm-metrics.yml " + dockerMetricsStackName,
	}
	cmd := strings.Join(cmdLines, ";")
	// And grafana
	// cmd += ";docker service rm grafana; docker service create --name grafana --publish 8080:8080 grafana/grafana"
	if err := manager.ExecCommandPiped(cmd); err != nil {
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("setting manager-1 availability to active: %s", err.Error()),
			),
		)
	} else {
		fmt.Println("Prometheus service deployed")
	}

	return nil
}
