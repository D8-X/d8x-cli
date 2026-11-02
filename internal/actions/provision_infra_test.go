package actions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectProvisioningCredentialsOnly_NilLinodeConfig(t *testing.T) {
	var cfg configs.D8XConfig
	require.NoError(t, json.Unmarshal([]byte(`{"server_provider":"linode"}`), &cfg))
	require.Nil(t, cfg.LinodeConfig)

	ic := &InputCollector{
		ConfigRWriter: configs.NewInMemoryD8XConfigRW(&cfg),
		SelectedEnv:   "prod",
		SSHKeyPath:    writeTestKey(t),
	}
	t.Setenv("LINODE_TOKEN_PROD", "tok")
	t.Setenv("BW_SESSION", "")

	err := ic.CollectProvisioningCredentialsOnly(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no linode_config block")
}

func TestCollectProvisioningCredentialsOnly_NilAWSConfig(t *testing.T) {
	var cfg configs.D8XConfig
	require.NoError(t, json.Unmarshal([]byte(`{"server_provider":"aws"}`), &cfg))
	require.Nil(t, cfg.AWSConfig)

	ic := &InputCollector{
		ConfigRWriter: configs.NewInMemoryD8XConfigRW(&cfg),
		SelectedEnv:   "prod",
		SSHKeyPath:    writeTestKey(t),
	}
	t.Setenv("AWS_ACCESS_KEY_PROD", "ak")
	t.Setenv("AWS_SECRET_KEY_PROD", "sk")
	t.Setenv("BW_SESSION", "")

	err := ic.CollectProvisioningCredentialsOnly(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no aws_config block")
}

func writeTestKey(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nx\n"), 0600))
	require.NoError(t, os.WriteFile(path+".pub", []byte("ssh-ed25519 AAAA test\n"), 0644))
	return path
}

func TestBuildTfvars_Linode_CarriesNodeSizeAndDbId(t *testing.T) {
	cfg := &configs.D8XConfig{
		ServerProvider: configs.D8XServerProviderLinode,
		LinodeConfig: &configs.D8XLinodeConfig{
			Region:           "eu-central",
			LabelPrefix:      "prod",
			NumWorker:        4,
			BrokerServerSize: "g6-dedicated-4",
			SwarmNodeSize:    "g6-dedicated-8",
			DbId:             "12345678",
		},
	}
	out := buildTfvars(cfg)
	assert.Contains(t, out, `worker_size          = "g6-dedicated-8"`)
	assert.Contains(t, out, `broker_size          = "g6-dedicated-4"`)
	assert.Contains(t, out, "linode_db_cluster_id = 12345678")
}

func TestBuildTfvars_Linode_OmitsUnsetOrInvalidDbId(t *testing.T) {
	for _, id := range []string{"", "0", "-1", "not-a-number"} {
		cfg := &configs.D8XConfig{
			ServerProvider: configs.D8XServerProviderLinode,
			LinodeConfig:   &configs.D8XLinodeConfig{Region: "eu-central", DbId: id},
		}
		assert.NotContains(t, buildTfvars(cfg), "linode_db_cluster_id", "db id %q", id)
	}
}

func TestRecoverProviderFromTfvars_RoundTripsLinode(t *testing.T) {
	in := &configs.D8XLinodeConfig{
		Region:             "eu-central",
		LabelPrefix:        "prod",
		NumWorker:          4,
		BrokerServerSize:   "g6-dedicated-4",
		SwarmNodeSize:      "g6-dedicated-8",
		DbId:               "12345678",
		CreateBrokerServer: true,
		DeploySwarm:        true,
	}
	vars := parseTfvars(buildTfvars(&configs.D8XConfig{
		ServerProvider: configs.D8XServerProviderLinode,
		LinodeConfig:   in,
	}))

	got := &configs.D8XLinodeConfig{
		Region:             vars["region"],
		LabelPrefix:        vars["server_label_prefix"],
		NumWorker:          4,
		BrokerServerSize:   vars["broker_size"],
		SwarmNodeSize:      vars["worker_size"],
		DbId:               positiveIntOrEmpty(vars["linode_db_cluster_id"]),
		CreateBrokerServer: vars["create_broker_server"] == "true",
		DeploySwarm:        vars["create_swarm"] == "true",
	}
	assert.Equal(t, in, got)
}

func TestIndexOf_ReturnsMinusOneWhenAbsent(t *testing.T) {
	assert.Equal(t, 1, indexOf([]string{"a", "b"}, "b"))
	assert.Equal(t, -1, indexOf([]string{"a", "b"}, "c"))
}

func TestClearFetchedTerraformFiles_KeepsStateDropsStale(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"main.tf", "vars.tf", "sg.tf", "routes.tf", "env.auto.tfvars", "terraform.tfstate", "terraform.tfstate.backup", ".terraform.lock.hcl"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "swarm"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".terraform", "providers"), 0755))

	require.NoError(t, clearFetchedTerraformFiles(dir))

	for _, kept := range []string{"terraform.tfstate", "terraform.tfstate.backup", ".terraform.lock.hcl", ".terraform"} {
		_, err := os.Stat(filepath.Join(dir, kept))
		assert.NoError(t, err, "%s must survive", kept)
	}
	for _, gone := range []string{"main.tf", "vars.tf", "sg.tf", "routes.tf", "env.auto.tfvars", "swarm"} {
		_, err := os.Stat(filepath.Join(dir, gone))
		assert.True(t, os.IsNotExist(err), "%s must be removed", gone)
	}

	assert.NoError(t, clearFetchedTerraformFiles(filepath.Join(dir, "does-not-exist")))
}
