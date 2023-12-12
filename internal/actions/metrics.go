package actions

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
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
		{Src: "embedded/grafana", Dst: "./grafana", Overwrite: true, Dir: true},
	}
	if err := c.EmbedCopier.Copy(configs.EmbededConfigs, filesToCopy...); err != nil {
		return fmt.Errorf("copying configs to local file system: %w", err)
	}

	// Configure the ip addresses of prometheus targets
	workerIPs, err := c.HostsCfg.GetWorkerPrivateIps()
	if err != nil {
		return err
	}
	prometheusYaml, err := os.ReadFile("./prometheus.yml")
	if err != nil {
		return err
	}
	if prometheusWithTargets, err := c.processPrometheusYaml(prometheusYaml, workerIPs); err != nil {
		return err
	} else {
		if err := os.WriteFile("./prometheus.yml", prometheusWithTargets, 0666); err != nil {
			return err
		}
	}

	if err := manager.CopyFilesOverSftp(
		conn.SftpCopySrcDest{Src: "./prometheus.yml", Dst: "./prometheus.yml"},
		conn.SftpCopySrcDest{Src: "./docker-swarm-metrics.yml", Dst: "./docker-swarm-metrics.yml"},

		conn.SftpCopySrcDest{Src: "./grafana/datasource-prometheus.yml", Dst: "./grafana/datasource-prometheus.yml"},
		conn.SftpCopySrcDest{Src: "./grafana/chart.json", Dst: "./grafana/chart.json"},
		conn.SftpCopySrcDest{Src: "./grafana/dashboards.yml", Dst: "./grafana/dashboards.yml"},
	); err != nil {
		return fmt.Errorf("copying prometheus config to manager: %w", err)
	}

	// Re-Create prometheus_config and deploy metrics compose services
	// (docker-swarm-metrics.yml)
	cmdLines := []string{
		// Create prometheus data volume and don't remove it
		"docker volume create prometheus_data_vol",

		"sleep 5; docker compose -f docker-swarm-metrics.yml up -d",
	}
	cmd := strings.Join(cmdLines, ";")
	if err := manager.ExecCommandPiped(cmd); err != nil {
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("Deploying metrics: %s", err.Error()),
			),
		)
	} else {
		fmt.Println("Prometheus service deployed")
	}

	// Update cfg
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	cfg.MetricsDeployed = true

	return c.ConfigRWriter.Write(cfg)
}

func (c *Container) processPrometheusYaml(promYamlContents []byte, workers []string) ([]byte, error) {
	// Port that we expose cadvisor on
	CADVISOR_PORT := "4003"

	mp := map[any]any{}
	if err := yaml.Unmarshal(promYamlContents, &mp); err != nil {
		return nil, err
	}

	// We want to edit targets and remarshall the yaml
	targets := make([]string, len(workers))
	for i, w := range workers {
		targets[i] = w + ":" + CADVISOR_PORT
	}
	// This is horrible, but it works if we don't change our default prometheus config
	mp["scrape_configs"].([]any)[0].(map[any]any)["static_configs"].([]any)[0].(map[any]any)["targets"] = targets

	return yaml.Marshal(mp)
}

// TunnelGrafana establishes a tunnel to grafana service on manager
func (c *Container) TunnelGrafana(ctx *cli.Context) error {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if !cfg.MetricsDeployed {
		return fmt.Errorf("metrics services are not deployed")
	}
	styles.PrintCommandTitle("Establishing ssh tunnel to grafana on manager node...")

	// Grafana port exposed on swarm node locally
	grafanaPort := 4002
	// UUID of our main chart (from chart.json)
	grafanaD8XServicesDashboardUUID := "e0b3b284-5f62-40f8-9c85-421ef3e1d841"

	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}
	managerConn, err := conn.NewSSHConnection(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return err
	}

	port := ctx.Args().First()
	if len(port) != 0 {
		_, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("port argument must be a number")
		}
	} else {
		//  Default to 8080
		port = "8080"
	}

	addr := fmt.Sprintf("127.0.0.1:%s", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("binding listener on port %s: %w", port, err)
	}

	info := fmt.Sprintf("Grafana is accessible at http://%s", addr)
	fmt.Println(styles.SuccessText.Render(info))
	info = fmt.Sprintf("Main D8X Services dashboard is accessible at http://%s/d/%s", addr, grafanaD8XServicesDashboardUUID)
	fmt.Println(styles.SuccessText.Render(info))
	fmt.Printf("Default username: admin\nDefault password: admin\n")

	cpFn := func(w io.Writer, r io.Reader) error {
		_, err := io.Copy(w, r)
		return err
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		defer conn.Close()

		grafanaConn, err := managerConn.GetClient().Dial("tcp", "127.0.0.1:"+strconv.Itoa(grafanaPort))
		if err != nil {
			return fmt.Errorf("dialing grafana service on manager: %w", err)
		}

		go cpFn(grafanaConn, conn)
		go cpFn(conn, grafanaConn)
	}
}
