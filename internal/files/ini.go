package files

import (
	"fmt"
	"io"
	"os"
	"strings"
)

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

func (h *HostsFile) GetMangerPublicIp() (string, error) {
	ip, err := h.FindFirstIp("[managers]")
	if err != nil {
		return "", fmt.Errorf("manager ip was not found in hosts file: %w", err)
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
