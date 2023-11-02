package actions

import (
	"context"
	"fmt"
	"net/http"
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
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Establish manager node ssh connection
	ip, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}
	managerConn, err := conn.NewSSHConnection(ip, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return fmt.Errorf("establishing ssh connection to manager node: %w", err)
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

	// Once http endpoint checks are done - run docker services check
	dockerSwarmInfoString, err := healthChecksSwarmServices(managerConn)
	if err != nil {
		return fmt.Errorf("retrieving docker swarm info: %w", err)
	}
	// Print the docker services info outside the bubbletea program
	fmt.Printf("\nDocker swarm services status:%s\n", dockerSwarmInfoString)

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

	// Build the output
	fullOutput := strings.Builder{}
	for _, svcName := range svcNames {
		out := strings.Builder{}
		v := svcs[svcName]
		out.WriteByte('\n')

		// Name and instances
		nameAndInstances := fmt.Sprintf(
			"%s\n  instances: %s",
			svcName, v.replicasString,
		)
		out.WriteString(nameAndInstances)

		// Instances info

		for _, psInfo := range v.psInfo {
			out.WriteString("\n  \\_ ")
			out.WriteString(psInfo.name)
			out.WriteString(" on ")
			out.WriteString(psInfo.node)
			out.WriteString(" status ")
			out.WriteString(psInfo.currentState)

			if psInfo.err != "" {
				out.WriteString(" ")
				out.WriteString(psInfo.err)
			}
		}

		if v.running < v.total {
			fullOutput.WriteString(styles.ErrorText.Render(
				out.String(),
			))
		} else {
			fullOutput.WriteString(out.String())
		}

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

	for _, svc := range h.services {
		spinner := ""
		sendingRequestTime := ""
		retry := ""
		responseStatus := ""
		reachable := ""
		if svc.done {
			if svc.success {
				spinner = ok
				reachable = "service was reached"
			} else {
				spinner = notok
				reachable = "service unreachable"
			}

			responseStatus = "HTTP Status (" + strconv.Itoa(svc.responseStatus) + ")"

			if svc.responseStatus >= 200 && svc.responseStatus < 500 {
				responseStatus = styles.SuccessText.Render(responseStatus)
			}
			if svc.responseStatus >= 500 {
				responseStatus = styles.ErrorText.Render(responseStatus)
				spinner = warning
			}
			spinner += " "

		} else {
			spinner = h.spinner.View()

			if time.Now().Before(svc.currentCtxDeadline) {
				// display how many seconds left till request deadline
				sendingRequestTime = "next request timeout in " + strconv.Itoa(int(svc.currentCtxDeadline.Unix()-time.Now().Unix())) + "s"
			}

			retry = strconv.Itoa(int(svc.currentRetry))
			retry = "(" + retry + ")"
		}

		httpHealthChecks.WriteString(
			fmt.Sprintf(
				"%s %s %s %s %s %s %s",
				spinner,
				svc.service,
				svc.hostname,
				retry,
				sendingRequestTime,
				reachable,
				responseStatus,
			),
		)

		httpHealthChecks.WriteByte('\n')
	}

	title := "Performing health checks"
	dockerSwarmInfo := "\n" + h.spinner.View() + "Loading Docker Swarm Services info\n"
	if h.allDone() {
		title = "Health checks done"
		dockerSwarmInfo = ""
	}

	return title + "\n\nHTTP Endpoints:\n" + httpHealthChecks.String() + dockerSwarmInfo
}
