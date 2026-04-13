package actions

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/urfave/cli/v2"
)

const MaxRequestWaitTime = time.Second * 60

func (c *Container) HealthCheck(ctx *cli.Context) error {
	styles.PrintCommandTitle("Performing health checks...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureEnvironment(cfg); err != nil {
		return err
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token != "" && c.SelectedEnv != "" {
		sites, err := ghReadFile(token, c.SelectedEnv+"/sites.conf")
		if err == nil {
			for _, host := range extractAllServerNames(sites.Content) {
				name := strings.Split(host, ".")[0]
				if _, exists := cfg.Services[configs.D8XServiceName(name)]; !exists {
					cfg.Services[configs.D8XServiceName(name)] = configs.D8XService{
						Name:      configs.D8XServiceName(name),
						HostName:  host,
						UsesHTTPS: true,
					}
				}
			}
		}
	}

	svcsForModel := []*serviceHostnameStatus{}
	for _, svc := range cfg.Services {
		prefix := "http://"
		if svc.UsesHTTPS {
			prefix = "https://"
		}
		shs := &serviceHostnameStatus{
			hostname: prefix + svc.HostName,
			service:  string(svc.Name),
		}
		svcsForModel = append(svcsForModel, shs)

		ch := make(chan healthCheckMsg)

		// Run health check requests for each svc
		go c.healthCheckWithBackoff(
			ch,
			svc,
			// Start off with 2 second deadline
			time.Second*2,
		)

		// Listen for updates from health checker
		go func(ch chan healthCheckMsg, s *serviceHostnameStatus) {
			for info := range ch {

				if info.done {
					s.responseStatus = info.responseStatus
					s.done = true
					s.success = info.success
					return
				}
				s.currentCtxDeadline = info.nextTimeout
				s.currentRetry++

			}
		}(ch, shs)
	}

	_, err = tea.NewProgram(initHealthCheckModel(healthCheckModel{
		services: svcsForModel,
	})).Run()

	if err != nil {
		return err
	}

	if cfg.SwarmDeployed {
		// Establish manager node ssh connection
		ip, err := c.HostsCfg.GetMangerPublicIp()
		if err != nil {
			return err
		}
		managerConn, err := conn.NewSSHConnection(ip, c.DefaultClusterUserName, c.SshKeyPath)
		if err != nil {
			return fmt.Errorf("establishing ssh connection to manager node: %w", err)
		}
		// Once http endpoint checks are done - run docker services check
		dockerSwarmInfoString, err := healthChecksSwarmServices(managerConn)
		if err != nil {
			return fmt.Errorf("retrieving docker swarm info: %w", err)
		}
		// Print the docker services info outside the bubbletea program
		fmt.Printf("\nDocker swarm services status:\n%s", dockerSwarmInfoString)
	}

	brokerIp, err := c.HostsCfg.GetBrokerPublicIp()
	if err == nil {
		brokerConn, err := conn.NewSSHConnection(brokerIp, c.DefaultClusterUserName, c.SshKeyPath)
		if err == nil {
			fmt.Printf("\nBroker services (docker compose):\n")
			out, err := brokerConn.ExecCommand("docker ps --format '{{.Names}} {{.Status}}'")
			if err == nil {
				for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
					if line == "" {
						continue
					}
					parts := strings.SplitN(line, " ", 2)
					name := strings.TrimPrefix(parts[0], "broker-")
					name = strings.TrimSuffix(name, "-1")
					status := ""
					if len(parts) > 1 {
						status = parts[1]
					}
					icon := ok
					if !strings.Contains(strings.ToLower(status), "up") {
						icon = notok
						status = styles.ErrorText.Render(status)
					}
					fmt.Printf("  %s %-20s %s\n", icon, name, status)
				}
			}

			brokerHostname := ""
			if svc, ok := cfg.Services[configs.D8XServiceBrokerServer]; ok {
				brokerHostname = svc.HostName
			}
			if brokerHostname == "" {
				// Try from infra repo
				token := os.Getenv("GITHUB_TOKEN")
				if token != "" && c.SelectedEnv != "" {
					brokerNginx, err := ghReadFile(token, c.SelectedEnv+"/broker-nginx.conf")
					if err == nil {
						names := extractAllServerNames(brokerNginx.Content)
						if len(names) > 0 {
							brokerHostname = names[0]
						}
					}
				}
			}
			if brokerHostname != "" {
				apiKey := os.Getenv("NGINX_API_KEY")
				rpcURL := fmt.Sprintf("https://%s/rpc", brokerHostname)
				curlCmd := fmt.Sprintf(`curl -s -o /dev/null -w '%%{http_code}' -X POST -H 'Content-Type: application/json' -H 'X-Api-Key: %s' -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' %s`, apiKey, rpcURL)
				out, err = brokerConn.ExecCommand(curlCmd)
				code := strings.TrimSpace(string(out))
				icon := notok
				codeDisplay := styles.ErrorText.Render("unreachable")
				if err == nil && code == "200" {
					icon = ok
					codeDisplay = styles.SuccessText.Render(code)
				} else if err == nil {
					codeDisplay = styles.ErrorText.Render(code)
				}
				fmt.Printf("  %s %-20s %s  %s\n", icon, "rpc-proxy", codeDisplay, rpcURL)
			}
		}
	}

	return nil

}

type healthCheckMsg struct {
	done           bool
	success        bool
	nextTimeout    time.Time
	responseStatus int
}

func (c *Container) healthCheckWithBackoff(ch chan healthCheckMsg, svc configs.D8XService, timeout time.Duration) error {
	if timeout > MaxRequestWaitTime {
		ch <- healthCheckMsg{
			done:    true,
			success: false,
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	nextTimeout, _ := ctx.Deadline()
	ch <- healthCheckMsg{nextTimeout: nextTimeout}
	ctx.Deadline()
	prefix := "http://"
	if svc.UsesHTTPS {
		prefix = "https://"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, prefix+svc.HostName, nil)
	if err != nil {
		return err
	}
	if apiKey := os.Getenv("NGINX_API_KEY"); apiKey != "" {
		req.Header.Set("X-Api-Key", apiKey)
	}
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return c.healthCheckWithBackoff(ch, svc, timeout*2)
	} else {

		ch <- healthCheckMsg{done: true, success: true, responseStatus: resp.StatusCode}
	}

	return nil
}

// healthChecksSwarmServices parses services statuses from manager node
func healthChecksSwarmServices(managerConn conn.SSHConnection) (string, error) {

	cmd := `docker service ls | awk 'NR > 1' | awk  '{print $2}' | xargs docker service ps --format 'table {{.Node}}[##]{{.Name}}[##]{{.CurrentState}}[##]{{.Error}}[##]' --no-trunc`

	psOutput, err := managerConn.ExecCommand(cmd)
	if err != nil {
		return "", err
	}

	lsOutput, err := managerConn.ExecCommand("docker service ls")
	if err != nil {
		return "", err
	}

	psLines := strings.Split(string(psOutput), "\n")[1:]
	lsLines := strings.Split(string(lsOutput), "\n")[1:]

	// docker ps output info
	type svcPsInfo struct {
		node         string
		currentState string
		err          string
		// svc task name (appened with .<int>)
		name string
	}

	type svcInfo struct {
		name string
		// Number of running replicas
		running int
		// Number of total replicas defined to run
		total          int
		replicasString string
		psInfo         []svcPsInfo
	}

	// Parse `docker ls` info
	//Fields: ID,NAME,MODE,REPLICAS,IMAGE,PORTS
	svcNames := []string{}
	svcs := map[string]svcInfo{}
	for _, line := range lsLines {
		fields := strings.Fields(line)

		if len(fields) >= 3 {
			replicas := strings.Split(fields[3], "/")
			running, _ := strconv.Atoi(replicas[0])
			total, _ := strconv.Atoi(replicas[1])
			info := svcInfo{
				name:           fields[1],
				running:        running,
				total:          total,
				psInfo:         []svcPsInfo{},
				replicasString: fields[3],
			}
			svcs[fields[1]] = info
			svcNames = append(svcNames, fields[1])
		}
	}
	sort.StringSlice(svcNames).Sort()

	// Parse `docker ps` info
	// NODE, NAME, CURRENT STATE, ERROR
	for _, line := range psLines {
		// See the cmd for separator
		fields := strings.Split(line, "[##]")
		for i, f := range fields {
			fields[i] = strings.TrimSpace(f)
		}

		if len(fields) >= 3 {
			name := strings.Split(fields[1], ".")[0]
			err := fields[3]

			if v, ok := svcs[name]; ok {
				v.psInfo = append(v.psInfo, svcPsInfo{
					node:         fields[0],
					currentState: fields[2],
					err:          err,
					name:         fields[1],
				})
				svcs[name] = v
			}
		}
	}

	maxName := 0
	for _, name := range svcNames {
		short := strings.TrimPrefix(name, "stack_")
		if len(short) > maxName {
			maxName = len(short)
		}
	}

	fullOutput := strings.Builder{}
	for _, svcName := range svcNames {
		v := svcs[svcName]
		short := strings.TrimPrefix(svcName, "stack_")
		padding := strings.Repeat(" ", maxName-len(short)+1)

		icon := ok
		if v.running < v.total {
			icon = notok
		}

		replicas := fmt.Sprintf("%d/%d", v.running, v.total)
		line := fmt.Sprintf("  %s %s%s%s", icon, short, padding, replicas)

		if len(v.psInfo) == 1 {
			ps := v.psInfo[0]
			line += fmt.Sprintf("  %s  %s", ps.node, ps.currentState)
			if ps.err != "" {
				line += "  " + ps.err
			}
		} else if len(v.psInfo) > 1 {
			nodes := []string{}
			for _, ps := range v.psInfo {
				nodes = append(nodes, ps.node)
			}
			line += fmt.Sprintf("  [%s]", strings.Join(nodes, ", "))
		}

		if v.running < v.total {
			fullOutput.WriteString(styles.ErrorText.Render(line))
		} else {
			fullOutput.WriteString(line)
		}
		fullOutput.WriteByte('\n')
	}

	return fullOutput.String(), nil
}

const (
	notok   = "❌"
	ok      = "✅"
	warning = "⚠️"
)

type serviceHostnameStatus struct {
	hostname           string
	service            string
	currentRetry       uint
	currentCtxDeadline time.Time
	done               bool
	success            bool
	responseStatus     int
}

var _ (tea.Model) = (*healthCheckModel)(nil)

type healthCheckModel struct {
	services []*serviceHostnameStatus
	spinner  spinner.Model
}

func initHealthCheckModel(h healthCheckModel) healthCheckModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.D8XPurple)
	h.spinner = s
	return h
}

func (h healthCheckModel) Init() tea.Cmd {
	return h.spinner.Tick
}
func (m healthCheckModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		default:
			return m, nil
		}
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)

		// Check if all service checks are done
		if m.allDone() {
			return m, tea.Quit
		}

		return m, cmd
	}
}
func (m healthCheckModel) allDone() bool {
	numDone := 0
	for _, svc := range m.services {
		if svc.done {
			numDone++
		}
	}
	return numDone == len(m.services)
}

func (h healthCheckModel) View() string {
	httpHealthChecks := strings.Builder{}

	// Find max service name length for alignment
	maxName := 0
	for _, svc := range h.services {
		if len(svc.service) > maxName {
			maxName = len(svc.service)
		}
	}

	for _, svc := range h.services {
		icon := ""
		status := ""
		if svc.done {
			code := svc.responseStatus
			codeStr := strconv.Itoa(code)
			if svc.success {
				if code >= 200 && code < 500 {
					icon = ok
					status = styles.SuccessText.Render(codeStr)
				} else if code >= 500 {
					icon = warning
					status = styles.ErrorText.Render(codeStr)
				} else {
					icon = ok
					status = codeStr
				}
			} else {
				icon = notok
				status = styles.ErrorText.Render("unreachable")
			}
		} else {
			icon = h.spinner.View()
			if time.Now().Before(svc.currentCtxDeadline) {
				status = fmt.Sprintf("retry #%d", svc.currentRetry)
			}
		}

		padding := strings.Repeat(" ", maxName-len(svc.service)+1)
		httpHealthChecks.WriteString(
			fmt.Sprintf("  %s %s%s%s  %s\n", icon, svc.service, padding, status, svc.hostname),
		)
	}

	title := "Performing health checks"
	dockerSwarmInfo := "\n" + h.spinner.View() + " Loading Docker swarm services...\n"
	if h.allDone() {
		title = "Health checks done"
		dockerSwarmInfo = ""
	}

	return title + "\n\nHTTP Endpoints:\n" + httpHealthChecks.String() + dockerSwarmInfo
}
