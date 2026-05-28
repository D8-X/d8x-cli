package files

import (
	"strings"
)

func NewMemHostsFileInteractor(content []byte, onWrite func(content string) error) HostsFileInteractor {
	return &memHostFileInteractor{
		hosts:   parseHostsBytes(content),
		onWrite: onWrite,
	}
}

func parseHostsBytes(content []byte) *HostsFile {
	if len(content) == 0 {
		return &HostsFile{lines: []string{}, numLines: 0}
	}
	lines := strings.Split(string(content), "\n")
	return &HostsFile{
		lines:    lines,
		numLines: len(lines),
		fileName: "<memory>",
	}
}

var _ (HostsFileInteractor) = (*memHostFileInteractor)(nil)

type memHostFileInteractor struct {
	hosts   *HostsFile
	onWrite func(content string) error
}

func (m *memHostFileInteractor) GetAllPublicIps() []string {
	ret := []string{}
	if ip, err := m.GetBrokerPublicIp(); err == nil {
		ret = append(ret, ip)
	}
	if ip, err := m.GetMangerPublicIp(); err == nil {
		ret = append(ret, ip)
	}
	if ips, err := m.GetWorkerIps(); err == nil {
		ret = append(ret, ips...)
	}
	return ret
}

func (m *memHostFileInteractor) GetBrokerPublicIp() (string, error) {
	return m.hosts.GetBrokerPublicIp()
}
func (m *memHostFileInteractor) GetBrokerPrivateIp() (string, error) {
	return m.hosts.GetBrokerPrivateIp()
}
func (m *memHostFileInteractor) GetMangerPublicIp() (string, error) {
	return m.hosts.GetMangerPublicIp()
}
func (m *memHostFileInteractor) GetMangerPrivateIp() (string, error) {
	return m.hosts.GetMangerPrivateIp()
}
func (m *memHostFileInteractor) GetWorkerIps() ([]string, error) {
	return m.hosts.GetWorkerIps()
}
func (m *memHostFileInteractor) GetWorkerPrivateIps() ([]string, error) {
	return m.hosts.GetWorkerPrivateIps()
}
func (m *memHostFileInteractor) GetLines() ([]string, error) {
	return m.hosts.lines, nil
}

func (m *memHostFileInteractor) GetPath() string {
	return "<memory>"
}

func (m *memHostFileInteractor) WriteLines(lines []string) error {
	m.hosts = &HostsFile{
		lines:    lines,
		numLines: len(lines),
		fileName: "<memory>",
	}
	if m.onWrite != nil {
		return m.onWrite(strings.Join(lines, "\n"))
	}
	return nil
}
