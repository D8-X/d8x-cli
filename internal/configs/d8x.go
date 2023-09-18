package configs

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/D8-X/d8x-cli/internal/styles"
)

//go:generate mockgen -package mocks -destination ../mocks/configs.go . D8XConfigReadWriter

// D8XServiceName is the name of the service that is deployed by d8x-cli and
// exposed to the public internet via subdomain.
type D8XServiceName string

const (
	D8XServiceBrokerServer D8XServiceName = "broker_server"

	D8XServiceMainHTTP D8XServiceName = "main_http"
	D8XServiceMainWS   D8XServiceName = "main_ws"

	D8XServiceHistory D8XServiceName = "history"

	D8XServiceReferral D8XServiceName = "referral"

	D8XServiceCandlesWs D8XServiceName = "candles_ws"
)

type D8XConfig struct {
	Services       map[D8XServiceName]D8XService `json:"services"`
	ServerProvider D8XServerProvider             `json:"server_provider"`

	LinodeConfig *D8XLinodeConfig `json:"linode_config"`
	AWSConfig    *D8XAWSConfig    `json:"aws_config"`
}

type D8XServerProvider string

const (
	D8XServerProviderLinode D8XServerProvider = "linode"
	D8XServerProviderAWS    D8XServerProvider = "aws"
)

type D8XLinodeConfig struct {
	Token       string `json:"linode_token"`
	DbId        string `json:"db_id"`
	Region      string `json:"region"`
	LabelPrefix string `json:"label_prefix"`
}

type D8XAWSConfig struct {
	AccesKey    string `json:"access_key"`
	SecretKey   string `json:"secret_key"`
	Region      string `json:"region"`
	LabelPrefix string `json:"label_prefix"`
}

type D8XService struct {
	// Name of the service
	Name D8XServiceName `json:"name"`
	// Whether site should be set up with https
	UsesHTTPS bool `json:"https"`
	// User specified domain name
	HostName string `json:"hostname"`
}

func NewD8XConfig() *D8XConfig {
	return &D8XConfig{
		Services: make(map[D8XServiceName]D8XService),
	}
}

// D8XConfigReadWriter reads and writes D8X config to storage system. If file
// does not exists it is created automatically.
type D8XConfigReadWriter interface {
	// Read reads the config from underlying storagesystem. If config is not
	// found, an empty D8XConfig is returned
	Read() (*D8XConfig, error)
	Write(*D8XConfig) error
}

func NewFileBasedD8XConfigRW(filePath string) D8XConfigReadWriter {
	return &d8xConfigFileReadWriter{filePath: filePath}
}

var _ (D8XConfigReadWriter) = (*d8xConfigFileReadWriter)(nil)

type d8xConfigFileReadWriter struct {
	filePath string
}

func (d *d8xConfigFileReadWriter) Read() (*D8XConfig, error) {
	cfg := NewD8XConfig()
	if contents, err := os.ReadFile(d.filePath); err != nil {
		// Print error message to indicate empty config when not intended
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("Config file was not found: %s", d.filePath),
			),
		)
		return cfg, nil
	} else {
		if err := json.Unmarshal(contents, cfg); err != nil {
			return nil, err
		}
	}

	// Make sure we initialize nil-able fields
	if cfg.Services == nil {
		cfg.Services = make(map[D8XServiceName]D8XService)
	}

	return cfg, nil
}

func (d *d8xConfigFileReadWriter) Write(cfg *D8XConfig) error {
	if buf, err := json.Marshal(cfg); err != nil {
		return err
	} else {
		if err := os.WriteFile(d.filePath, buf, 0666); err != nil {
			return err
		}
	}
	return nil
}
