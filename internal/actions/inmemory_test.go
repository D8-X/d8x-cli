package actions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadInfraRepoFile_NoGitHubToken_ReturnsEmbeddedBytes(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	c := &Container{}
	content, err := c.loadInfraRepoFile("prometheus.yml", "embedded/prometheus.yml")
	require.NoError(t, err)
	assert.NotEmpty(t, content)
	assert.True(t, strings.HasPrefix(string(content), "global:") || strings.Contains(string(content), "scrape_configs"),
		"prometheus.yml content should look like a Prometheus config, got: %q", truncate(string(content)))
}

func TestLoadInfraRepoFile_NoGitHubToken_AllManaged_ReturnsContent(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	c := &Container{}
	for _, m := range infraRepoManagedFiles {
		t.Run(m.envRelPath, func(t *testing.T) {
			content, err := c.loadInfraRepoFile(m.envRelPath, m.embeddedSrc)
			require.NoError(t, err)
			assert.NotEmpty(t, content, "embedded fallback for %s should be non-empty", m.envRelPath)
		})
	}
}

func TestLoadInfraRepoFile_NoEmbedded_ReturnsError(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	c := &Container{}
	_, err := c.loadInfraRepoFile("does-not-exist", "embedded/does-not-exist")
	require.Error(t, err)
}

func TestLoadOptionalInfraRepoFile_NoGitHubToken_ReturnsNil(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	c := &Container{SelectedEnv: "any"}
	content, err := c.loadOptionalInfraRepoFile("anything")
	require.NoError(t, err)
	assert.Nil(t, content)
}

func TestLoadOptionalInfraRepoFile_NoSelectedEnv_ReturnsNil(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_dummy")
	c := &Container{SelectedEnv: ""}
	content, err := c.loadOptionalInfraRepoFile("anything")
	require.NoError(t, err)
	assert.Nil(t, content)
}

func TestEditSwarmEnvBytes_SetsExpectedKeys(t *testing.T) {
	c := &Container{cachedChainJson: ChainJson{}}
	original := []byte("CHAIN_ID=old\nREDIS_PASSWORD=oldpw\nKEEP=me\n")
	cfg := &configs.D8XConfig{
		ChainId:                  8453,
		SwarmRedisPassword:       "newpw",
		SwarmRemoteBrokerHTTPUrl: "https://broker.example.com",
		DatabaseDSN:              "postgres://x:y@h/db",
	}

	out, err := c.EditSwarmEnvBytes(original, cfg)
	require.NoError(t, err)

	s := string(out)
	assert.Contains(t, s, "CHAIN_ID=8453")
	assert.Contains(t, s, "REDIS_PASSWORD=newpw")
	assert.Contains(t, s, "REMOTE_BROKER_HTTP=https://broker.example.com")
	assert.Contains(t, s, "DATABASE_DSN=postgres://x:y@h/db")
	assert.Contains(t, s, "KEEP=me", "untouched lines should survive")
	assert.NotContains(t, s, "CHAIN_ID=old")
	assert.NotContains(t, s, "REDIS_PASSWORD=oldpw")
}

func TestEditSwarmEnvBytes_EmptyStringValuesSkipped(t *testing.T) {
	c := &Container{cachedChainJson: ChainJson{}}
	original := []byte("REDIS_PASSWORD=existing-pw\nDATABASE_DSN=existing-dsn\n")
	cfg := &configs.D8XConfig{SwarmRedisPassword: "", DatabaseDSN: ""}

	out, err := c.EditSwarmEnvBytes(original, cfg)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "REDIS_PASSWORD=existing-pw", "empty cfg.SwarmRedisPassword must not overwrite")
	assert.Contains(t, s, "DATABASE_DSN=existing-dsn", "empty cfg.DatabaseDSN must not overwrite")
}

func TestEditRpcConfigUrlsBytes_AppendsForExistingChain(t *testing.T) {
	c := &Container{}
	original := []byte(`[{"chainId":8453,"HTTP":["https://old.rpc"],"WS":["wss://old.ws"]}]`)
	out, err := c.editRpcConfigUrlsBytes(original, 8453, []string{"wss://new.ws"}, []string{"https://new.rpc"})
	require.NoError(t, err)

	var entries []RPCConfigEntry
	require.NoError(t, json.Unmarshal(out, &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, uint(8453), entries[0].ChainId)
	assert.Contains(t, entries[0].HttpRpcs, "https://old.rpc")
	assert.Contains(t, entries[0].HttpRpcs, "https://new.rpc")
	require.NotNil(t, entries[0].WsRpcs)
	assert.Contains(t, *entries[0].WsRpcs, "wss://old.ws")
	assert.Contains(t, *entries[0].WsRpcs, "wss://new.ws")
}

func TestEditRpcConfigUrlsBytes_AddsEntryForNewChain(t *testing.T) {
	c := &Container{}
	original := []byte(`[{"chainId":1,"HTTP":["https://eth.rpc"]}]`)
	out, err := c.editRpcConfigUrlsBytes(original, 8453, nil, []string{"https://base.rpc"})
	require.NoError(t, err)

	var entries []RPCConfigEntry
	require.NoError(t, json.Unmarshal(out, &entries))
	require.Len(t, entries, 2)

	var base *RPCConfigEntry
	for i := range entries {
		if entries[i].ChainId == 8453 {
			base = &entries[i]
		}
	}
	require.NotNil(t, base)
	assert.Equal(t, []string{"https://base.rpc"}, base.HttpRpcs)
	assert.Nil(t, base.WsRpcs, "ws nil should stay nil")
}

func TestUpdateConfigBytes_TransformAppliedAndReturned(t *testing.T) {
	original := []byte(`{"a":[1,2,3],"b":"hello"}`)
	out, err := UpdateConfigBytes(original, func(m *map[string]any) error {
		(*m)["a"] = []int{4, 5}
		(*m)["c"] = "added"
		return nil
	})
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Equal(t, "hello", parsed["b"])
	assert.Equal(t, "added", parsed["c"])
	a, ok := parsed["a"].([]any)
	require.True(t, ok)
	assert.Len(t, a, 2)
}

func TestParseEnvBytes_HandlesQuotedAndComments(t *testing.T) {
	data := []byte("# comment\nFOO=bar\nQUOTED=\"hello world\"\nSPACES = value \n=novalue\nNO_EQ_SIGN\n")
	got := parseEnvBytes(data)
	assert.Equal(t, "bar", got["FOO"])
	assert.Equal(t, "hello world", got["QUOTED"])
	assert.Equal(t, "value", got["SPACES"])
	assert.NotContains(t, got, "")
	assert.NotContains(t, got, "NO_EQ_SIGN")
}

func truncate(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
