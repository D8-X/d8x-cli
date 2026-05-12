package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStageInfraRepoFile_NoGitHubToken_FallsBackToEmbedded(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	c := &Container{}
	localPath := filepath.Join(t.TempDir(), "rpc.main.json")

	err := c.stageInfraRepoFile(
		"trader-backend/rpc.main.json",
		"embedded/trader-backend/rpc.main.json",
		localPath,
	)
	require.NoError(t, err)

	data, err := os.ReadFile(localPath)
	require.NoError(t, err)
	assert.NotEmpty(t, data, "embedded fallback should produce non-empty content")
}

func TestStageInfraRepoFile_NoGitHubToken_NoSelectedEnv_StillFallsBack(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	c := &Container{SelectedEnv: ""}
	localPath := filepath.Join(t.TempDir(), "prometheus.yml")

	err := c.stageInfraRepoFile(
		"prometheus.yml",
		"embedded/prometheus.yml",
		localPath,
	)
	require.NoError(t, err)
	info, err := os.Stat(localPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestStageInfraRepoFile_NoGitHubToken_EmbeddedMissing_ReturnsError(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	c := &Container{}
	localPath := filepath.Join(t.TempDir(), "missing.json")

	err := c.stageInfraRepoFile(
		"nonexistent/path.json",
		"embedded/does-not-exist.json",
		localPath,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embedded")
}

func TestStageInfraRepoFile_TokenSetButNoSelectedEnv_SkipsGitHubAndFallsBack(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_dummy_should_not_be_used")

	c := &Container{SelectedEnv: ""}
	localPath := filepath.Join(t.TempDir(), "rpc.history.json")

	err := c.stageInfraRepoFile(
		"trader-backend/rpc.history.json",
		"embedded/trader-backend/rpc.history.json",
		localPath,
	)
	require.NoError(t, err)
	data, err := os.ReadFile(localPath)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestFetchSetupPlaybook_NoGitHubToken_FallsBackToEmbedded(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	c := &Container{SelectedEnv: "test-env"}
	path, err := c.fetchSetupPlaybook()
	require.NoError(t, err)
	defer os.Remove(path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Setup swarm cluster", "embedded playbook should be the swarm setup playbook")
}

func TestFetchSetupPlaybook_NoSelectedEnv_FallsBackToEmbedded(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_dummy")

	c := &Container{SelectedEnv: ""}
	path, err := c.fetchSetupPlaybook()
	require.NoError(t, err)
	defer os.Remove(path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

func TestStageInfraRepoFile_AllInfraRepoManagedFiles_FallbackWorks(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	c := &Container{}
	tmp := t.TempDir()

	for _, m := range infraRepoManagedFiles {
		t.Run(m.envRelPath, func(t *testing.T) {
			localPath := filepath.Join(tmp, strings.ReplaceAll(m.envRelPath, "/", "_"))
			err := c.stageInfraRepoFile(m.envRelPath, m.embeddedSrc, localPath)
			require.NoError(t, err)
			info, err := os.Stat(localPath)
			require.NoError(t, err)
			assert.Greater(t, info.Size(), int64(0), "embedded fallback should produce non-empty content for %s", m.envRelPath)
		})
	}
}
