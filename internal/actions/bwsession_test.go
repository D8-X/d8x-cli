package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBWSessionCache_RoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	assert.Equal(t, "", readCachedBWSession(), "no cache initially")

	require.NoError(t, writeCachedBWSession("session-token-xyz"))
	assert.Equal(t, "session-token-xyz", readCachedBWSession())

	clearCachedBWSession()
	assert.Equal(t, "", readCachedBWSession(), "cleared")
}

func TestBWSessionCache_FilePermissions0600(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, writeCachedBWSession("token"))

	p, err := bwSessionCachePath()
	require.NoError(t, err)
	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "session file must be 0600")
}

func TestBWSessionCachePath_UsesHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := bwSessionCachePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".d8x-cli", "session"), p)
}

func TestBWSessionCache_TrimsWhitespace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p, err := bwSessionCachePath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0700))
	require.NoError(t, os.WriteFile(p, []byte("  padded-token  \n"), 0600))
	assert.Equal(t, "padded-token", readCachedBWSession())
}
