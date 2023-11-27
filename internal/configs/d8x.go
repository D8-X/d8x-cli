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

	// Whether broker server is deployed
	BrokerDeployed bool `json:"broker_deployed"`

	BrokerServerConfig D8XBrokerServerConfig `json:"broker_server_config"`

	ReferralConfig ReferralConfig `json:"referral_config"`

	// Chain id of all services
	ChainId uint `json:"chain_id"`

	// List of user provided http rpc endpoints for chainId
	HttpRpcList map[string][]string `json:"http_rpc_list"`
	// List of user provided ws rpc endpoints for chainId
	WsRpcList map[string][]string `json:"ws_rpc_list"`

	SwarmRedisPassword string `json:"swarm_redis_password"`

	// Value which will be used for REMOTE_BROKER_HTTP variable in .env file
	SwarmRemoteBrokerHTTPUrl string `json:"swarm_remote_broker_http_url"`

	// Database dsn string
	DatabaseDSN string `json:"database_dsn"`
}

type ReferralConfig struct {
	// ExecutorAddress     string `json:"executor_address"`
	BrokerPayoutAddress string `json:"broker_payout_address"`
}

func (d *D8XConfig) IsEmpty() bool {
	return d.ServerProvider == ""
}

// GetAnsibleUser returns the default sudo user for initial ansible
// configuration step
func (d *D8XConfig) GetAnsibleUser() string {
	if d.ServerProvider == D8XServerProviderLinode {
		return "root"
	} else if d.ServerProvider == D8XServerProviderAWS {
		// In case used image changes - we should also chane the user!
		return "ubuntu"
	}
	return ""
}

type D8XServerProvider string

const (
	D8XServerProviderLinode D8XServerProvider = "linode"
	D8XServerProviderAWS    D8XServerProvider = "aws"
)

type D8XLinodeConfig struct {
	Token              string `json:"linode_token"`
	DbId               string `json:"db_id"`
	Region             string `json:"region"`
	LabelPrefix        string `json:"label_prefix"`
	SwarmWorkerSize    string `json:"swarm_worker_size"`
	SwarmNodeSize      string `json:"swarm_node_size"`
	BrokerServerSize   string `json:"broker_server_size"`
	CreateBrokerServer bool   `json:"create_broker_server"`
}

type D8XAWSConfig struct {
	AccesKey               string `json:"access_key"`
	SecretKey              string `json:"secret_key"`
	Region                 string `json:"region"`
	LabelPrefix            string `json:"label_prefix"`
	RDSInstanceClass       string `json:"rds_instance_class"`
	CreateBrokerServer     bool   `json:"create_broker_server"`
	RDSCredentialsFilePath string `json:"rds_credentials_file_path"`
}

type D8XService struct {
	// Name of the service
	Name D8XServiceName `json:"name"`
	// Whether site should be set up with https
	UsesHTTPS bool `json:"https"`
	// User specified domain name
	HostName string `json:"hostname"`
}

type D8XBrokerServerConfig struct {
	FeeTBPS       string `json:"fee_tbps"`
	RedisPassword string `json:"redis_password"`
	// Executor address must match the provided Executor private key in swarm setup
	ExecutorAddress string `json:"executor_address"`
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

	// Write writes given D8XConfig to underlying storage system
	Write(*D8XConfig) error
}

func NewFileBasedD8XConfigRW(filePath string) D8XConfigReadWriter {
	return &d8xConfigFileReadWriter{filePath: filePath}
}

var _ (D8XConfigReadWriter) = (*d8xConfigFileReadWriter)(nil)

type d8xConfigFileReadWriter struct {
	filePath string

	warningShown bool
}

func (d *d8xConfigFileReadWriter) Read() (*D8XConfig, error) {
	cfg := NewD8XConfig()
	if contents, err := os.ReadFile(d.filePath); err != nil {
		// Print error message to indicate empty config when not intended. Only
		// once in current session!
		if !d.warningShown {
			fmt.Println(
				styles.ErrorText.Render(
					fmt.Sprintf("Config file was not found: %s", d.filePath),
				),
			)
			d.warningShown = true
		}
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
	if cfg.HttpRpcList == nil {
		cfg.HttpRpcList = make(map[string][]string)
	}
	if cfg.WsRpcList == nil {
		cfg.WsRpcList = make(map[string][]string)
	}

	return cfg, nil
}

func (d *d8xConfigFileReadWriter) Write(cfg *D8XConfig) error {
	if buf, err := json.MarshalIndent(cfg, "", "\t"); err != nil {
		return err
	} else {
		if err := os.WriteFile(d.filePath, buf, 0666); err != nil {
			return err
		}
	}
	return nil
}
