package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetRemovesDatabaseFilesOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bookmarks.db")
	mediaFile := filepath.Join(dir, "media-keep.jpg")

	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm", mediaFile} {
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
	}

	require.NoError(t, Reset(dbPath))

	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		_, err := os.Stat(path)
		assert.True(t, os.IsNotExist(err), "expected %s to be deleted", path)
	}

	_, err := os.Stat(mediaFile)
	assert.NoError(t, err, "non-database files must never be touched")
}

func TestResetIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bookmarks.db")
	require.NoError(t, Reset(dbPath)) // nothing exists — still no error
}
