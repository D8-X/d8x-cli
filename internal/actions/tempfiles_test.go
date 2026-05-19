package actions

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkPath_IsUnderOSTempDir(t *testing.T) {
	p := workPath("foo.txt")
	assert.True(t, filepath.IsAbs(p), "workPath must return an absolute path, got %q", p)
	assert.True(t, strings.HasPrefix(p, os.TempDir()), "workPath %q must be under os.TempDir() %q", p, os.TempDir())
	assert.Contains(t, p, "d8x-cli", "workPath %q must contain the d8x-cli namespace", p)
	assert.True(t, strings.HasSuffix(p, "foo.txt"), "workPath %q must end with the given rel %q", p, "foo.txt")
}

func TestWorkPath_HandlesNestedRel(t *testing.T) {
	p := workPath("trader-backend/rpc.main.json")
	expectedSuffix := filepath.Join("trader-backend", "rpc.main.json")
	assert.True(t, strings.HasSuffix(p, expectedSuffix), "workPath %q must end with %q (platform separator)", p, expectedSuffix)
}

func TestEnsureWorkDir_CreatesNestedParentDirs(t *testing.T) {
	rel := "test-" + t.Name() + "/sub/leaf.txt"
	path, err := ensureWorkDir(rel)
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "d8x-cli", "test-"+t.Name()))
	})

	dirInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err, "parent dir for %s should be created", path)
	assert.True(t, dirInfo.IsDir())
}

func TestStageInfraRepoFile_WritesUnderTempDir_AndNotCwd(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	cwdBefore := snapshotCwd(t)

	c := &Container{}
	localPath := filepath.Join(t.TempDir(), "prometheus.yml")
	err := c.stageInfraRepoFile("prometheus.yml", "embedded/prometheus.yml", localPath)
	require.NoError(t, err)

	info, err := os.Stat(localPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))

	cwdAfter := snapshotCwd(t)
	assert.ElementsMatch(t, cwdBefore, cwdAfter, "stageInfraRepoFile must not write to cwd")
}

func TestFetchSetupPlaybook_WritesUnderTempDir(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	c := &Container{}
	path, err := c.fetchSetupPlaybook()
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(path) })

	assert.True(t, strings.HasPrefix(path, os.TempDir()), "playbook %q must be under os.TempDir() %q", path, os.TempDir())
	assert.Contains(t, path, "d8x-cli")
}

func TestWriteHostsToTempFile_WritesUnderTempDir(t *testing.T) {
	h := &fakeHostsInteractor{lines: []string{"manager 1.2.3.4", "broker 5.6.7.8"}}
	path, err := writeHostsToTempFile(h)
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(path) })

	assert.True(t, strings.HasPrefix(path, os.TempDir()), "hosts file %q must be under os.TempDir() %q", path, os.TempDir())
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "manager 1.2.3.4")
	assert.Contains(t, string(content), "broker 5.6.7.8")
}

func TestTempDir_CleanupRemovesContents(t *testing.T) {
	rel := "cleanup-probe-" + t.Name() + ".txt"
	p, err := ensureWorkDir(rel)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, []byte("x"), 0644))

	root := filepath.Join(os.TempDir(), "d8x-cli")
	require.NoError(t, os.RemoveAll(root))

	_, err = os.Stat(p)
	assert.True(t, os.IsNotExist(err), "file %s should be removed by os.RemoveAll of %s", p, root)
}

func TestTempPath_CrossPlatformSeparators(t *testing.T) {
	p := workPath("a/b/c.json")
	switch runtime.GOOS {
	case "windows":
		assert.Contains(t, p, "\\a\\b\\c.json")
	default:
		assert.Contains(t, p, "/a/b/c.json")
	}
}

type fakeHostsInteractor struct {
	lines []string
}

func (f *fakeHostsInteractor) GetLines() ([]string, error)            { return f.lines, nil }
func (f *fakeHostsInteractor) WriteLines([]string) error              { return nil }
func (f *fakeHostsInteractor) GetMangerPublicIp() (string, error)     { return "", nil }
func (f *fakeHostsInteractor) GetMangerPrivateIp() (string, error)    { return "", nil }
func (f *fakeHostsInteractor) GetBrokerPublicIp() (string, error)     { return "", nil }
func (f *fakeHostsInteractor) GetBrokerPrivateIp() (string, error)    { return "", nil }
func (f *fakeHostsInteractor) GetWorkerIps() ([]string, error)        { return nil, nil }
func (f *fakeHostsInteractor) GetWorkerPrivateIps() ([]string, error) { return nil, nil }
func (f *fakeHostsInteractor) GetAllPublicIps() []string              { return nil }
func (f *fakeHostsInteractor) GetPath() string                        { return "" }

func snapshotCwd(t *testing.T) []string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	entries, err := os.ReadDir(cwd)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
