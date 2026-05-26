package actions

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

// Port that we expose cadvisor on
var CADVISOR_PORT = 4003

// DeployMetrics copies prometheus config and redeploys prometheus service.
// Prometheus deployment is separated from main swarm deployment because we want
// to run it on manager, and it is set to drainer availability by default (in
// ansible setup)
func (c *Container) DeployMetrics(ctx *cli.Context) error {
	fmt.Println("Deploying prometheus and grafana on manager...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureProvisionedEnvironment(cfg); err != nil {
		return err
	}
	if cfg.ServerProvider == "" {
		return fmt.Errorf("server_provider is empty in this env's config.json on the infra repo; set it to \"linode\" or \"aws\" there, or run \"d8x setup provision\" first")
	}
	if err := c.RequireProvisionedHosts("metrics-deploy", "manager"); err != nil {
		return err
	}

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
	defer manager.Close()

	loads := []struct {
		envRelPath, embeddedSrc, remoteDst string
	}{
		{"docker-swarm-metrics.yml", "embedded/docker-swarm-metrics.yml", "./docker-swarm-metrics.yml"},
		{"prometheus.yml", "embedded/prometheus.yml", "./prometheus.yml"},
		{"grafana/datasource-prometheus.yml", "embedded/grafana/datasource-prometheus.yml", "./grafana/datasource-prometheus.yml"},
		{"grafana/chart.json", "embedded/grafana/chart.json", "./grafana/chart.json"},
		{"grafana/chart-cadvisor.json", "embedded/grafana/chart-cadvisor.json", "./grafana/chart-cadvisor.json"},
		{"grafana/dashboards.yml", "embedded/grafana/dashboards.yml", "./grafana/dashboards.yml"},
	}
	contents := make(map[string][]byte, len(loads))
	for _, l := range loads {
		data, err := c.loadInfraRepoFile(l.envRelPath, l.embeddedSrc)
		if err != nil {
			return fmt.Errorf("loading %s: %w", l.envRelPath, err)
		}
		contents[l.envRelPath] = data
	}

	workerIPs, err := c.HostsCfg.GetWorkerPrivateIps()
	if err != nil {
		return err
	}
	prometheusWithTargets, err := c.processPrometheusYaml(contents["prometheus.yml"], workerIPs)
	if err != nil {
		return err
	}
	contents["prometheus.yml"] = prometheusWithTargets

	sftpOps := make([]conn.SftpCopySrcDest, 0, len(loads))
	for _, l := range loads {
		sftpOps = append(sftpOps, conn.SftpCopySrcDest{Content: contents[l.envRelPath], Dst: l.remoteDst})
	}
	if err := manager.CopyFilesOverSftp(sftpOps...); err != nil {
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
	prometheusDeployed := true
	if err := manager.ExecCommandPiped(cmd); err != nil {
		prometheusDeployed = false
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("Deploying metrics: %s", err.Error()),
			),
		)
	} else {
		fmt.Println("Prometheus service deployed")
	}

	// Block access of cadvisor port for public ip servers providers
	switch cfg.ServerProvider {
	case configs.D8XServerProviderLinode:
		workerPrivIps, err := c.HostsCfg.GetWorkerPrivateIps()
		if err != nil {
			return err
		}

		pwd, err := c.ResolvePassword(ctx)
		if err != nil {
			return fmt.Errorf("getting sudo password: %w", err)
		}

		wg := sync.WaitGroup{}
		for _, ip := range workerPrivIps {
			ip := ip
			wg.Add(1)
			go func(workerPrivIp string) {
				defer wg.Done()
				sh, err := conn.NewSSHConnectionWithBastion(manager.GetClient(), workerPrivIp, c.DefaultClusterUserName, c.SshKeyPath)
				if err != nil {
					fmt.Println(
						styles.ErrorText.Render(
							fmt.Sprintf("Connecting to worker %s via manager bastion: %s", workerPrivIp, err.Error()),
						),
					)
					return
				}
				defer sh.Close()
				check := fmt.Sprintf("iptables -L -t raw | grep ':%d'", CADVISOR_PORT)
				inner := fmt.Sprintf("%s || iptables -I PREROUTING 1 -t raw -p tcp -d %s --dport %d -j DROP && iptables-save > /etc/iptables/rules.v4", check, workerPrivIp, CADVISOR_PORT)
				out, err := sh.ExecCommand(
					fmt.Sprintf(`printf '%%s\n' %s | sudo -S bash -c %s`, shQuote(pwd), shQuote(inner)),
				)
				if err != nil {
					fmt.Println(string(out))
					fmt.Println(
						styles.ErrorText.Render("[" + workerPrivIp + "] Error blocking cadvisor port on worker: " + err.Error()),
					)
				}
			}(ip)
		}
		wg.Wait()

	}

	if !prometheusDeployed {
		return fmt.Errorf("metrics deploy aborted: prometheus stack failed to start")
	}
	cfg.MetricsDeployed = true

	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return err
	}
	if err := c.PublishRemoteConfig(cfg); err != nil {
		fmt.Printf("  %s failed to sync remote config: %s\n", notok, err)
	}
	return nil
}

func (c *Container) processPrometheusYaml(promYamlContents []byte, workers []string) ([]byte, error) {

	mp := map[any]any{}
	if err := yaml.Unmarshal(promYamlContents, &mp); err != nil {
		return nil, err
	}

	targets := make([]string, len(workers))
	for i, w := range workers {
		targets[i] = w + ":" + strconv.Itoa(CADVISOR_PORT)
	}
	scrapes, ok := mp["scrape_configs"].([]any)
	if !ok || len(scrapes) == 0 {
		return nil, fmt.Errorf("prometheus.yml: missing scrape_configs[]")
	}
	job, ok := scrapes[0].(map[any]any)
	if !ok {
		return nil, fmt.Errorf("prometheus.yml: scrape_configs[0] not a map")
	}
	statics, ok := job["static_configs"].([]any)
	if !ok || len(statics) == 0 {
		return nil, fmt.Errorf("prometheus.yml: missing static_configs[]")
	}
	entry, ok := statics[0].(map[any]any)
	if !ok {
		return nil, fmt.Errorf("prometheus.yml: static_configs[0] not a map")
	}
	entry["targets"] = targets

	return yaml.Marshal(mp)
}

// TunnelGrafana establishes a tunnel to grafana service on manager
func (c *Container) TunnelGrafana(ctx *cli.Context) error {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureProvisionedEnvironment(cfg); err != nil {
		return err
	}
	if err := c.RequireProvisionedHosts("grafana-tunnel", "manager"); err != nil {
		return err
	}
	if !cfg.MetricsDeployed {
		return fmt.Errorf("metrics services are not deployed")
	}
	styles.PrintCommandTitle("Establishing ssh tunnel to grafana on manager node...")

	// Grafana port exposed on swarm node locally
	grafanaPort := 4002
	// UUID of our main chart (from chart.json, chart-cadvisor.json)
	grafanaD8XServicesDashboardUUID := "e0b3b284-5f62-40f8-9c85-421ef3e1d841"
	grafanaCadvisorMetricsDashboardUUID := "e0b3b284-5f62-40f8-9c85-421ef3e1d842"

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

	fmt.Println("Grafana is accessible at:")
	info := fmt.Sprintf("\thttp://%s", addr)
	fmt.Println(styles.SuccessText.Render(info))
	info = fmt.Sprintf("\thttp://%[1]s/d/%[2]s \n\thttp://%[1]s/d/%[3]s", addr, grafanaD8XServicesDashboardUUID, grafanaCadvisorMetricsDashboardUUID)
	fmt.Println("Main D8X Services dashboards are accessible at:")
	fmt.Println(styles.SuccessText.Render(info))
	fmt.Printf("Default username: admin\nDefault password: admin\n\n")

	fmt.Println(styles.GrayText.Render("Press Ctrl+C to exit"))

	cpFn := func(w io.Writer, r io.Reader) error {
		_, err := io.Copy(w, r)
		return err
	}

	for {
		clientConn, err := l.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer clientConn.Close()
			grafanaConn, err := managerConn.GetClient().Dial("tcp", "127.0.0.1:"+strconv.Itoa(grafanaPort))
			if err != nil {
				fmt.Println(styles.ErrorText.Render(fmt.Sprintf("dialing grafana service on manager: %s", err)))
				return
			}
			defer grafanaConn.Close()
			done := make(chan struct{}, 2)
			go func() { cpFn(grafanaConn, clientConn); done <- struct{}{} }()
			go func() { cpFn(clientConn, grafanaConn); done <- struct{}{} }()
			<-done
		}()
	}
}
