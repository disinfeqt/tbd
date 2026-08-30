package accounts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"twitter-bookmarks-downloader/internal/store"
)

func freshDB(t *testing.T) {
	t.Helper()
	originalDB := store.DB
	t.Cleanup(func() { store.DB = originalDB })
	require.NoError(t, store.Init(":memory:"))
}

// An archive from before accounts existed is handed to the first account that
// actually syncs — not to whoever merely has x.com open.
func TestLegacyArchiveIsAdoptedOnFirstSync(t *testing.T) {
	freshDB(t)
	require.NoError(t, store.DB.Create(&store.TweetModel{ID: "t1", ScreenName: "alice"}).Error)
	require.NoError(t, Load())

	legacy, ok := Get(LegacyID)
	require.True(t, ok)
	assert.Empty(t, legacy.MediaDir, "the legacy archive follows the global media folder")

	// Signing in on x.com is not a claim on someone else's bookmarks.
	_, err := Ensure("111", "browsing")
	require.NoError(t, err)
	_, stillThere := Get(LegacyID)
	assert.True(t, stillThere, "a page visit must not adopt the archive")

	// Syncing is.
	_, err = Touch("222", "syncer")
	require.NoError(t, err)
	_, gone := Get(LegacyID)
	assert.False(t, gone, "the first sync takes the archive")

	var tweet store.TweetModel
	require.NoError(t, store.DB.First(&tweet, "id = ?", "t1").Error)
	assert.Equal(t, "222", tweet.AccountID)

	var membership store.AccountBookmarkModel
	require.NoError(t, store.DB.First(&membership, "tweet_id = ?", "t1").Error)
	assert.Equal(t, "222", membership.AccountID)

	// It keeps the folder those files are already in.
	adopter, ok := Get("222")
	require.True(t, ok)
	assert.Empty(t, adopter.MediaDir)
}

// Settings a user turned off have to stay off; a gorm default once quietly
// flipped them back on at insert time.
func TestNewAccountKeepsDisabledDownloads(t *testing.T) {
	freshDB(t)
	require.NoError(t, Load())

	_, err := Touch("333", "picky")
	require.NoError(t, err)

	no := false
	updated, err := Update("333", nil, &no, nil)
	require.NoError(t, err)
	assert.False(t, updated.DownloadVideos)
	assert.True(t, updated.DownloadImages)

	var stored store.AccountModel
	require.NoError(t, store.DB.First(&stored, "id = ?", "333").Error)
	assert.False(t, stored.DownloadVideos)
}
