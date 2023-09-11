package actions

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/configs"
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

		// Run health check requests for
		go c.healthCheckWithBackoff(
			ch,
			svc,
			// Start off with 2 second deadline
			time.Second*2,
		)

		// Listen for updates from health cheker
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
	if _, err := tea.NewProgram(initHealthCheckModel(svcsForModel)).Run(); err != nil {
		return err
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
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return c.healthCheckWithBackoff(ch, svc, timeout*2)
	} else {

		ch <- healthCheckMsg{done: true, success: true, responseStatus: resp.StatusCode}
	}

	return nil
}

const (
	notok = "❌"
	ok    = "✅"
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

func initHealthCheckModel(svcs []*serviceHostnameStatus) healthCheckModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.D8XPurple)
	return healthCheckModel{
		spinner:  s,
		services: svcs,
	}
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
	if numDone == len(m.services) {
		return true
	}

	return false
}

func (h healthCheckModel) View() string {
	b := strings.Builder{}

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
			}

		} else {
			spinner = h.spinner.View()

			if time.Now().Before(svc.currentCtxDeadline) {
				// display how many seconds left till request deadline
				sendingRequestTime = "next request timeout in " + strconv.Itoa(int(svc.currentCtxDeadline.Unix()-time.Now().Unix())) + "s"
			}

			retry = strconv.Itoa(int(svc.currentRetry))
			retry = "(" + retry + ")"
		}

		b.WriteString(
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

		b.WriteByte('\n')
	}

	title := "Performing health checks"
	if h.allDone() {
		title = "Health checks done"
	}

	return title + "\n\n" + b.String()
}
