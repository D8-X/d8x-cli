package actions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testNewChainEnv = "test-new-chain"

func TestBuildScaffoldConfigJSON_Linode_TestNewChain(t *testing.T) {
	cfg := &configs.D8XConfig{
		ChainId:        42161,
		ServerProvider: configs.D8XServerProviderLinode,
		LinodeConfig: &configs.D8XLinodeConfig{
			LabelPrefix:        testNewChainEnv,
			Region:             "eu-central",
			NumWorker:          3,
			BrokerServerSize:   "g6-dedicated-2",
			SwarmNodeSize:      "g6-dedicated-2",
			CreateBrokerServer: true,
			DeploySwarm:        true,
			DbId:               "",
		},
	}
	out, err := buildScaffoldConfigJSON(cfg)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.EqualValues(t, 42161, parsed["chain_id"])
	assert.Equal(t, "linode", parsed["server_provider"])

	lc, ok := parsed["linode_config"].(map[string]any)
	require.True(t, ok, "linode_config must be present")
	assert.Equal(t, testNewChainEnv, lc["label_prefix"])
	assert.Equal(t, "eu-central", lc["region"])
	assert.EqualValues(t, 3, lc["num_worker"])
	assert.Equal(t, "g6-dedicated-2", lc["broker_server_size"])
	assert.Equal(t, "g6-dedicated-2", lc["swarm_node_size"])
	assert.Equal(t, true, lc["create_broker_server"])
	assert.Equal(t, true, lc["deploy_swarm"])
	assert.Equal(t, "", lc["db_id"])

	_, hasAws := parsed["aws_config"]
	assert.False(t, hasAws, "aws_config must not be emitted for a linode env")

	assert.True(t, strings.HasSuffix(out, "\n"), "must end with newline for clean diffs")
}

func TestBuildScaffoldConfigJSON_LinodeWithExternalDb_TestNewChain(t *testing.T) {
	cfg := &configs.D8XConfig{
		ChainId:        42161,
		ServerProvider: configs.D8XServerProviderLinode,
		LinodeConfig: &configs.D8XLinodeConfig{
			LabelPrefix:        testNewChainEnv,
			Region:             "eu-central",
			NumWorker:          3,
			BrokerServerSize:   "g6-dedicated-2",
			SwarmNodeSize:      "g6-dedicated-2",
			CreateBrokerServer: true,
			DeploySwarm:        true,
			DbId:               "12345678",
		},
	}
	out, err := buildScaffoldConfigJSON(cfg)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	lc := parsed["linode_config"].(map[string]any)
	assert.Equal(t, "12345678", lc["db_id"])
}

func TestBuildScaffoldConfigJSON_AWS_TestNewChain(t *testing.T) {
	cfg := &configs.D8XConfig{
		ChainId:        8453,
		ServerProvider: configs.D8XServerProviderAWS,
		AWSConfig: &configs.D8XAWSConfig{
			LabelPrefix:        testNewChainEnv,
			Region:             "us-east-1",
			NumWorker:          4,
			RDSInstanceClass:   "db.t4g.small",
			CreateBrokerServer: true,
			DeploySwarm:        true,
		},
	}
	out, err := buildScaffoldConfigJSON(cfg)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.EqualValues(t, 8453, parsed["chain_id"])
	assert.Equal(t, "aws", parsed["server_provider"])

	ac, ok := parsed["aws_config"].(map[string]any)
	require.True(t, ok, "aws_config must be present")
	assert.Equal(t, testNewChainEnv, ac["label_prefix"])
	assert.Equal(t, "us-east-1", ac["region"])
	assert.EqualValues(t, 4, ac["num_worker"])
	assert.Equal(t, "db.t4g.small", ac["rds_instance_class"])
	assert.Equal(t, true, ac["create_broker_server"])
	assert.Equal(t, true, ac["deploy_swarm"])

	_, hasLinode := parsed["linode_config"]
	assert.False(t, hasLinode, "linode_config must not be emitted for an aws env")
}

func TestBuildTfvars_Linode_TestNewChain(t *testing.T) {
	cfg := &configs.D8XConfig{
		ServerProvider: configs.D8XServerProviderLinode,
		LinodeConfig: &configs.D8XLinodeConfig{
			LabelPrefix:        testNewChainEnv,
			Region:             "eu-central",
			NumWorker:          3,
			BrokerServerSize:   "g6-dedicated-2",
			CreateBrokerServer: true,
			DeploySwarm:        true,
		},
	}
	out := buildTfvars(cfg)
	assert.Contains(t, out, `region               = "eu-central"`)
	assert.Contains(t, out, "num_workers          = 3")
	assert.Contains(t, out, `broker_size          = "g6-dedicated-2"`)
	assert.Contains(t, out, `server_label_prefix  = "`+testNewChainEnv+`"`)
	assert.Contains(t, out, "create_broker_server = true")
	assert.Contains(t, out, "create_swarm         = true")
}

func TestBuildTfvars_AWS_TestNewChain(t *testing.T) {
	cfg := &configs.D8XConfig{
		ServerProvider: configs.D8XServerProviderAWS,
		AWSConfig: &configs.D8XAWSConfig{
			LabelPrefix:        testNewChainEnv,
			Region:             "us-east-1",
			NumWorker:          4,
			RDSInstanceClass:   "db.t4g.small",
			CreateBrokerServer: true,
			DeploySwarm:        true,
		},
	}
	out := buildTfvars(cfg)
	assert.Contains(t, out, `region               = "us-east-1"`)
	assert.Contains(t, out, `server_label_prefix  = "`+testNewChainEnv+`"`)
	assert.Contains(t, out, "num_workers          = 4")
	assert.Contains(t, out, "create_broker_server = true")
	assert.Contains(t, out, "create_swarm         = true")
	assert.Contains(t, out, `rds_instance_class   = "db.t4g.small"`)
}

func TestBuildTfvars_AWS_DefaultsRdsInstanceClass_WhenEmpty(t *testing.T) {
	cfg := &configs.D8XConfig{
		ServerProvider: configs.D8XServerProviderAWS,
		AWSConfig: &configs.D8XAWSConfig{
			LabelPrefix: testNewChainEnv,
			Region:      "us-east-1",
		},
	}
	out := buildTfvars(cfg)
	assert.Contains(t, out, `rds_instance_class   = "db.t4g.small"`)
}

func TestBuildTfvars_NilConfigs_ReturnsEmpty(t *testing.T) {
	cfg := &configs.D8XConfig{ServerProvider: configs.D8XServerProviderLinode}
	assert.Equal(t, "", buildTfvars(cfg))

	cfg = &configs.D8XConfig{ServerProvider: configs.D8XServerProviderAWS}
	assert.Equal(t, "", buildTfvars(cfg))

	cfg = &configs.D8XConfig{}
	assert.Equal(t, "", buildTfvars(cfg))
}

func TestDefaultStr(t *testing.T) {
	assert.Equal(t, "fallback", defaultStr("", "fallback"))
	assert.Equal(t, "value", defaultStr("value", "fallback"))
}

func TestEnvFilesPaths_TestNewChain_ExpectedShape(t *testing.T) {
	cfg := &configs.D8XConfig{
		ChainId:        42161,
		ServerProvider: configs.D8XServerProviderLinode,
		LinodeConfig: &configs.D8XLinodeConfig{
			LabelPrefix:        testNewChainEnv,
			Region:             "eu-central",
			NumWorker:          3,
			BrokerServerSize:   "g6-dedicated-2",
			SwarmNodeSize:      "g6-dedicated-2",
			CreateBrokerServer: true,
			DeploySwarm:        true,
		},
	}
	cfgJSON, err := buildScaffoldConfigJSON(cfg)
	require.NoError(t, err)
	tfvars := buildTfvars(cfg)

	type expectFile struct {
		path     string
		contains []string
	}
	pathPrefix := testNewChainEnv + "/"
	expects := []expectFile{
		{pathPrefix + "config.json", []string{`"chain_id"`, `"server_provider"`, `"linode_config"`}},
		{pathPrefix + "terraform.tfvars", []string{`region`, `num_workers`, `broker_size`}},
	}
	for _, e := range expects {
		var body string
		switch {
		case strings.HasSuffix(e.path, "config.json"):
			body = cfgJSON
		case strings.HasSuffix(e.path, "terraform.tfvars"):
			body = tfvars
		}
		for _, sub := range e.contains {
			assert.Contains(t, body, sub, "%s missing expected substring %q", e.path, sub)
		}
	}
}
