package configs

import (
	"encoding/json"
	"fmt"
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

	D8XServiceCandlesWs D8XServiceName = "candles_ws"
)

var SuggestedSubdomains = map[D8XServiceName]string{
	D8XServiceBrokerServer: "broker",

	D8XServiceMainHTTP:  "api",
	D8XServiceMainWS:    "ws",
	D8XServiceHistory:   "history",
	D8XServiceCandlesWs: "candles",
}

type D8XConfig struct {
	Services       map[D8XServiceName]D8XService `json:"services"`
	ServerProvider D8XServerProvider             `json:"server_provider"`

	LinodeConfig *D8XLinodeConfig `json:"linode_config"`
	AWSConfig    *D8XAWSConfig    `json:"aws_config"`

	BrokerServerConfig D8XBrokerServerConfig `json:"broker_server_config"`

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

	CertbotEmail string `json:"certbot_email"`

	// Setup domain entered by user. Used to suggest subdomain names for
	// services
	SetupDomain string `json:"setup_domain"`

	// Whether metrics services were deployed
	MetricsDeployed bool `json:"metrics_deployed"`
	// Whether broker server is deployed
	BrokerDeployed        bool `json:"broker_deployed"`
	BrokerNginxDeployed   bool `json:"broker_nginx_deployed"`
	BrokerCertbotDeployed bool `json:"broker_certbot_deployed"`

	// Whether swarm is deployed
	SwarmDeployed        bool `json:"swarm_deployed"`
	SwarmNginxDeployed   bool `json:"swarm_nginx_deployed"`
	SwarmCertbotDeployed bool `json:"swarm_certbot_deployed"`

	// Ansible related configuration details
	ConfigDetails ConfigurationDetails `json:"configuration_details"`

	// Pyth/triton, etc. User supplied price feed endpoints which will be added
	// to prices.config.json
	UserSuppliedPriceFeedEndpoints []string `json:"user_supplied_price_feed_endpoints"`
}

func (c *D8XConfig) GetServersLabel() string {
	switch c.ServerProvider {
	case D8XServerProviderAWS:
		if c.AWSConfig != nil {
			return c.AWSConfig.LabelPrefix
		}
	case D8XServerProviderLinode:
		if c.LinodeConfig != nil {
			return c.LinodeConfig.LabelPrefix
		}
	}
	return "d8x-cluster"
}

type ConfigurationDetails struct {
	// Whether at least 1 time configuration was done successfully
	Done bool `json:"done"`

	// List of IP addresses of servers which were configured previously. This is
	// important for linode configuration step when non-first time setup is
	// performed. We use this list to mark which servers should use cluster user
	// instead of root for ssh access in configure action.
	ConfiguredServers []string `json:"configured_server_ip_addresses"`
}

// ResetDeploymentStatus cleans up deployment status of all services/servers,
// etc. Should be called and stored after tf-destroy.
func (d *D8XConfig) ResetDeploymentStatus() {
	d.MetricsDeployed = false
	d.BrokerDeployed = false
	d.BrokerNginxDeployed = false
	d.BrokerCertbotDeployed = false
	d.SwarmDeployed = false
	d.SwarmNginxDeployed = false
	d.SwarmCertbotDeployed = false
	d.ConfigDetails = ConfigurationDetails{
		Done:              false,
		ConfiguredServers: []string{},
	}
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
		// In case used image changes - we should also change the user!
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
	DeploySwarm        bool   `json:"deploy_swarm"`
	// Number of worker servers to deploy in swarm
	NumWorker int `json:"num_worker"`
}

type D8XAWSConfig struct {
	AccesKey               string `json:"access_key"`
	SecretKey              string `json:"secret_key"`
	Region                 string `json:"region"`
	LabelPrefix            string `json:"label_prefix"`
	RDSInstanceClass       string `json:"rds_instance_class"`
	CreateBrokerServer     bool   `json:"create_broker_server"`
	RDSCredentialsFilePath string `json:"rds_credentials_file_path"`
	DeploySwarm            bool   `json:"deploy_swarm"`
	// Number of worker servers to deploy in swarm
	NumWorker int `json:"num_worker"`
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
	FeeTBPS string `json:"fee_tbps"`
	// User supplied Fee value in percent
	FeeInputPercent string `json:"fee_input_percent"`

	RedisPassword string `json:"redis_password"`

}

func NewD8XConfig() *D8XConfig {
	return &D8XConfig{
		Services: make(map[D8XServiceName]D8XService),
	}
}

// D8XConfigReadWriter is the in-process state store the CLI uses while it
// reconciles remote config (infra repo + Bitwarden) with user edits. No file
// I/O — Read/Write operate on a deep-copied in-memory D8XConfig.
type D8XConfigReadWriter interface {
	// Read returns a copy of the current state. If nothing has been written
	// yet, an empty D8XConfig is returned.
	Read() (*D8XConfig, error)

	// Write replaces the current state with cfg.
	Write(*D8XConfig) error
}

// NewInMemoryD8XConfigRW returns a read-writer that holds D8XConfig entirely
// in memory.
func NewInMemoryD8XConfigRW(initial *D8XConfig) D8XConfigReadWriter {
	if initial == nil {
		initial = NewD8XConfig()
	}
	if initial.Services == nil {
		initial.Services = make(map[D8XServiceName]D8XService)
	}
	if initial.HttpRpcList == nil {
		initial.HttpRpcList = make(map[string][]string)
	}
	if initial.WsRpcList == nil {
		initial.WsRpcList = make(map[string][]string)
	}
	return &d8xConfigMemReadWriter{cfg: initial}
}

type d8xConfigMemReadWriter struct {
	cfg *D8XConfig
}

func (m *d8xConfigMemReadWriter) Read() (*D8XConfig, error) {
	// Deep-copy so concurrent read/mutate/write callers don't observe each other.
	data, err := json.Marshal(m.cfg)
	if err != nil {
		return nil, err
	}
	out := NewD8XConfig()
	if err := json.Unmarshal(data, out); err != nil {
		return nil, err
	}
	if out.Services == nil {
		out.Services = make(map[D8XServiceName]D8XService)
	}
	if out.HttpRpcList == nil {
		out.HttpRpcList = make(map[string][]string)
	}
	if out.WsRpcList == nil {
		out.WsRpcList = make(map[string][]string)
	}
	return out, nil
}

func (m *d8xConfigMemReadWriter) Write(cfg *D8XConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		m.cfg = cfg
		return nil
	}
	clone := NewD8XConfig()
	if err := json.Unmarshal(data, clone); err != nil {
		m.cfg = cfg
		return nil
	}
	if clone.Services == nil {
		clone.Services = make(map[D8XServiceName]D8XService)
	}
	if clone.HttpRpcList == nil {
		clone.HttpRpcList = make(map[string][]string)
	}
	if clone.WsRpcList == nil {
		clone.WsRpcList = make(map[string][]string)
	}
	m.cfg = clone
	return nil
}

func (c *D8XConfig) SuggestSubdomain(svc D8XServiceName, chainName string, chainId uint) string {
	if c.SetupDomain == "" {
		return ""
	}

	if subdomain, ok := SuggestedSubdomains[svc]; ok {
		return fmt.Sprintf("%s-%s-%d.%s", subdomain, chainName, chainId, c.SetupDomain)
	}
	return ""
}
