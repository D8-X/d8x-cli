package files

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func LoadHostsFile(file string) (*HostsFile, error) {
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
	for i, l := range h.lines {
		if strings.Contains(l, "[broker]") {
			if i+1 < h.numLines {
				// Ip address is the first entry
				return strings.Split(h.lines[i+1], " ")[0], nil
			}
		}
	}
	return "", nil
}
