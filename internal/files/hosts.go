package files

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// HostsFileInteractor interacts with hosts.cfg file
type HostsFileInteractor interface {
	GetBrokerPublicIp() (string, error)
	GetMangerPublicIp() (string, error)
	GetMangerPrivateIp() (string, error)
	GetWorkerIps() ([]string, error)
	GetWorkerPrivateIps() ([]string, error)
}

func NewFSHostsFileInteractor(filePath string) HostsFileInteractor {
	return &fsHostFileInteractor{
		filePath: filePath,
	}
}

var _ (HostsFileInteractor) = (*fsHostFileInteractor)(nil)

type fsHostFileInteractor struct {
	filePath string
	cached   *HostsFile
}

func (f *fsHostFileInteractor) ensureFileLoaded() error {
	if f.cached == nil {
		h, err := LoadHostsFileFromFS(f.filePath)
		if err != nil {
			return err
		}
		f.cached = h
	}
	return nil
}

func (f *fsHostFileInteractor) GetBrokerPublicIp() (string, error) {
	if err := f.ensureFileLoaded(); err != nil {
		return "", err
	}
	return f.cached.GetBrokerPublicIp()
}
func (f *fsHostFileInteractor) GetMangerPublicIp() (string, error) {
	if err := f.ensureFileLoaded(); err != nil {
		return "", err
	}
	return f.cached.GetMangerPublicIp()
}
func (f *fsHostFileInteractor) GetMangerPrivateIp() (string, error) {
	if err := f.ensureFileLoaded(); err != nil {
		return "", err
	}
	return f.cached.GetMangerPrivateIp()
}
func (f *fsHostFileInteractor) GetWorkerIps() ([]string, error) {
	if err := f.ensureFileLoaded(); err != nil {
		return nil, err
	}
	return f.cached.GetWorkerIps()
}
func (f *fsHostFileInteractor) GetWorkerPrivateIps() ([]string, error) {
	if err := f.ensureFileLoaded(); err != nil {
		return nil, err
	}
	return f.cached.GetWorkerPrivateIps()
}

// HostsFileLoader attempts to load and parse give file as HostsFile (hosts.cfg)
type HostsFileLoader func(file string) (*HostsFile, error)

var _ (HostsFileLoader) = (LoadHostsFileFromFS)

func LoadHostsFileFromFS(file string) (*HostsFile, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	contents, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("could not read the contents of %s file: %w", file, err)
	}

	lines := strings.Split(string(contents), "\n")

	return &HostsFile{
		lines:    lines,
		numLines: len(lines),
	}, nil
}

// HostsFile provides utility functions for hosts.cfg file
type HostsFile struct {
	lines    []string
	numLines int
}

// GetBrokerPublicIp gets the first broker server entry from hosts.cfg and
// returns its public ip address
func (h *HostsFile) GetBrokerPublicIp() (string, error) {
	ip, err := h.FindFirstIp("[broker]")
	if err != nil {
		return "", fmt.Errorf("broker ip was not found in hosts file: %w", err)
	}
	return ip, nil

}

func (h *HostsFile) GetMangerPrivateIp() (string, error) {
	ip, err := h.FindPrivateIps("manager")
	if err != nil {
		return "", fmt.Errorf("manager private ip was not found in hosts file: %w", err)
	}
	return ip[0], nil
}
func (h *HostsFile) GetMangerPublicIp() (string, error) {
	ip, err := h.FindFirstIp("[managers]")
	if err != nil {
		return "", fmt.Errorf("manager ip was not found in hosts file: %w", err)
	}
	return ip, nil
}

func (h *HostsFile) GetWorkerIps() ([]string, error) {
	ip, err := h.FindAllIps("[workers]")
	if err != nil {
		return nil, fmt.Errorf("worker ip was not found in hosts file: %w", err)
	}
	return ip, nil
}

func (h *HostsFile) GetWorkerPrivateIps() ([]string, error) {
	ip, err := h.FindPrivateIps("worker")
	if err != nil {
		return nil, fmt.Errorf("worker private ip was not found in hosts file: %w", err)
	}
	return ip, nil
}

// FindFirstIp returns the first item in the next line matching of
func (h *HostsFile) FindFirstIp(of string) (string, error) {
	for i, l := range h.lines {
		if strings.Contains(l, of) {
			if i+1 < h.numLines {
				// Ip address is the first entry
				return strings.Split(h.lines[i+1], " ")[0], nil
			}
		}
	}
	return "", nil
}

// FindPrivateIps returns the private ips of either nodeType=manager
// or nodeType=worker
func (h *HostsFile) FindPrivateIps(nodeType string) ([]string, error) {
	var ret = []string{}
	for _, l := range h.lines {
		if strings.Contains(l, nodeType+"_private_ip") {
			v := strings.Split(l, nodeType+"_private_ip=")[1]
			v = strings.Split(v, " ")[0]
			ret = append(ret, v)
		}
	}
	return ret, nil
}

// FindAllIps returns all items in the next line matching of
func (h *HostsFile) FindAllIps(of string) ([]string, error) {
	ret := []string{}

	runLoop := false
	for i, l := range h.lines {
		// Find first occurence of "of"
		if !runLoop && strings.Contains(l, of) {
			runLoop = true
			continue
		}

		if runLoop {
			// runLoop until we find the next group
			if strings.HasPrefix(l, "[") {
				break
			}

			// Ip address is the first entry
			ip := strings.Split(h.lines[i], " ")[0]
			if len(ip) > 0 {
				ret = append(ret, ip)
			}
		}
	}
	return ret, nil
}
