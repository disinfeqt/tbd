// Package accounts owns the X accounts TBD archives for: their media folders,
// their download settings, and which bookmarks belong to whom.
package accounts

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rotisserie/eris"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/store"
)

// LegacyID holds bookmarks archived before TBD knew about accounts. The first
// real account to sync adopts them, because a single-account archive can only
// ever have belonged to whoever is syncing it.
const LegacyID = "legacy"

// Media dirs and settings are read on every download and every media request,
// so the table is mirrored in memory and refreshed on write.
var (
	mu     sync.RWMutex
	cached map[string]store.AccountModel
)

// Load fills the in-memory mirror and moves any pre-accounts bookmarks under
// the legacy account. Safe to call again; it is a no-op once settled.
func Load() error {
	if err := adoptOrphanBookmarks(); err != nil {
		return err
	}
	return refresh()
}

func refresh() error {
	var rows []store.AccountModel
	if err := store.DB.Find(&rows).Error; err != nil {
		return eris.Wrap(err, "failed to load accounts")
	}

	next := make(map[string]store.AccountModel, len(rows))
	for _, row := range rows {
		next[row.ID] = row
	}

	mu.Lock()
	cached = next
	mu.Unlock()
	return nil
}

// adoptOrphanBookmarks gives every account-less tweet to the legacy account,
// keeping the media folder they were downloaded into.
func adoptOrphanBookmarks() error {
	var orphans int64
	if err := store.DB.Model(&store.TweetModel{}).Where("account_id = '' OR account_id IS NULL").Count(&orphans).Error; err != nil {
		return eris.Wrap(err, "failed to count account-less bookmarks")
	}
	if orphans == 0 {
		return backfillMemberships()
	}

	legacy := store.AccountModel{
		ID:     LegacyID,
		Handle: "",
		// An empty dir means "follow config.json", so the archive that existed
		// before accounts keeps using the media folder it always used, even if
		// that setting changes later.
		MediaDir:       "",
		DownloadVideos: config.Current().DownloadVideos,
		DownloadImages: config.Current().DownloadImages,
		CreatedAt:      time.Now(),
	}
	if err := store.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&legacy).Error; err != nil {
		return eris.Wrap(err, "failed to create the legacy account")
	}
	if err := store.DB.Model(&store.TweetModel{}).
		Where("account_id = '' OR account_id IS NULL").
		Update("account_id", LegacyID).Error; err != nil {
		return eris.Wrap(err, "failed to assign existing bookmarks to the legacy account")
	}
	logx.Infof("Assigned %d existing bookmarks to a first account — it takes on your X handle the next time you sync", orphans)

	return backfillMemberships()
}

// backfillMemberships makes sure every tweet is bookmarked by the account that
// owns it, so upgraded archives behave like freshly synced ones.
func backfillMemberships() error {
	err := store.DB.Exec(`INSERT OR IGNORE INTO account_bookmarks (account_id, tweet_id, synced_at)
		SELECT account_id, id, synced_at FROM tweets WHERE account_id <> ''`).Error
	return eris.Wrap(err, "failed to backfill account bookmarks")
}

// All returns every known account, most recently synced first.
func All() []store.AccountModel {
	mu.RLock()
	defer mu.RUnlock()

	out := make([]store.AccountModel, 0, len(cached))
	for _, row := range cached {
		out = append(out, row)
	}
	sortByRecency(out)
	return out
}

func sortByRecency(rows []store.AccountModel) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].LastSyncAt.After(rows[j-1].LastSyncAt); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func Get(id string) (store.AccountModel, bool) {
	mu.RLock()
	defer mu.RUnlock()

	row, ok := cached[id]
	return row, ok
}

// Default is the account the explorer opens on when it has no preference: the
// one that synced most recently.
func Default() string {
	rows := All()
	if len(rows) == 0 {
		return ""
	}
	return rows[0].ID
}

// MediaDir is where this account's files live, falling back to the global
// default for unknown accounts so nothing is ever written to an empty path.
func MediaDir(id string) string {
	if row, ok := Get(id); ok && row.MediaDir != "" {
		return row.MediaDir
	}
	return config.Current().MediaDir
}

// Settings reports what this account downloads, falling back to the global
// defaults for unknown accounts.
func Settings(id string) (videos bool, images bool) {
	if row, ok := Get(id); ok {
		return row.DownloadVideos, row.DownloadImages
	}
	current := config.Current()
	return current.DownloadVideos, current.DownloadImages
}

// DefaultMediaDirFor keeps a new account's downloads beside the existing ones
// without mixing them: media/, then media/<handle>.
func DefaultMediaDirFor(handle string) string {
	base := config.Current().MediaDir
	handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")
	if handle == "" {
		return base
	}
	return filepath.Join(base, handle)
}

// Touch records a sync from an account, creating it the first time.
func Touch(id, handle string) (store.AccountModel, error) {
	return upsert(id, handle, true)
}

// Ensure records that an account is signed in on x.com without claiming it
// synced anything, so the explorer can show it before its first bookmark lands.
func Ensure(id, handle string) (store.AccountModel, error) {
	return upsert(id, handle, false)
}

// upsert keeps an account's handle current, creating it the first time. Only a
// real sync claims the legacy archive: merely having x.com open says nothing
// about whose bookmarks those are.
func upsert(id, handle string, synced bool) (store.AccountModel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return store.AccountModel{}, eris.New("empty account id")
	}
	handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")

	if row, ok := Get(id); ok {
		updates := map[string]any{}
		if synced {
			updates["last_sync_at"] = time.Now()
		}
		if handle != "" && handle != row.Handle {
			updates["handle"] = handle
		}
		if len(updates) == 0 {
			return row, nil
		}
		if err := store.DB.Model(&store.AccountModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return row, eris.Wrapf(err, "failed to update account %s", id)
		}
		if err := refresh(); err != nil {
			return row, err
		}
		if synced {
			if err := adoptLegacyInto(id, handle); err != nil {
				return row, err
			}
		}
		row, _ = Get(id)
		return row, nil
	}

	current := config.Current()
	row := store.AccountModel{
		ID:             id,
		Handle:         handle,
		MediaDir:       DefaultMediaDirFor(handle),
		DownloadVideos: current.DownloadVideos,
		DownloadImages: current.DownloadImages,
		CreatedAt:      time.Now(),
	}
	if synced {
		row.LastSyncAt = time.Now()
	}

	if err := store.DB.Create(&row).Error; err != nil {
		return store.AccountModel{}, eris.Wrapf(err, "failed to create account %s", id)
	}
	if handle != "" {
		logx.Infof("Syncing a new account: @%s", handle)
	}
	if err := refresh(); err != nil {
		return row, err
	}
	if synced {
		if err := adoptLegacyInto(id, handle); err != nil {
			return row, err
		}
	}
	row, _ = Get(id)
	return row, nil
}

// adoptLegacyInto hands the pre-accounts archive to the first account that
// actually syncs. It happens once: the legacy account is gone afterwards.
func adoptLegacyInto(id, handle string) error {
	if id == LegacyID {
		return nil
	}
	legacy, ok := Get(LegacyID)
	if !ok {
		return nil
	}

	err := store.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&store.TweetModel{}).Where("account_id = ?", LegacyID).
			Update("account_id", id).Error; err != nil {
			return eris.Wrap(err, "failed to move existing bookmarks to the new account")
		}
		if err := tx.Model(&store.AccountBookmarkModel{}).Where("account_id = ?", LegacyID).
			Update("account_id", id).Error; err != nil {
			return eris.Wrap(err, "failed to move existing bookmark memberships")
		}
		// Keep the folder those files are already in — moving thousands of them
		// is never worth the risk, and the account can be pointed elsewhere later.
		if err := tx.Model(&store.AccountModel{}).Where("id = ?", id).
			Update("media_dir", legacy.MediaDir).Error; err != nil {
			return eris.Wrap(err, "failed to keep the existing media folder")
		}
		if err := tx.Delete(&store.AccountModel{}, "id = ?", LegacyID).Error; err != nil {
			return eris.Wrap(err, "failed to remove the legacy account")
		}
		return nil
	})
	if err != nil {
		return err
	}

	if handle != "" {
		logx.Infof("Your existing bookmarks now belong to @%s", handle)
	}
	return refresh()
}

// Update changes an account's settings. Nil fields are left as they are.
func Update(id string, mediaDir *string, videos, images *bool) (store.AccountModel, error) {
	row, ok := Get(id)
	if !ok {
		return store.AccountModel{}, eris.Errorf("unknown account %q", id)
	}

	updates := map[string]any{}
	if mediaDir != nil {
		// Clearing the field is meaningful: it puts the account back on the
		// global default from config.json.
		updates["media_dir"] = strings.TrimSpace(*mediaDir)
	}
	if videos != nil {
		updates["download_videos"] = *videos
	}
	if images != nil {
		updates["download_images"] = *images
	}
	if len(updates) == 0 {
		return row, nil
	}

	if err := store.DB.Model(&store.AccountModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return row, eris.Wrapf(err, "failed to update account %s", id)
	}
	if err := refresh(); err != nil {
		return row, err
	}
	row, _ = Get(id)
	return row, nil
}

// Resolve turns a requested account id into one that exists, falling back to
// the most recently synced account so the explorer always has something to show.
func Resolve(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return Default()
	}
	if _, ok := Get(id); ok {
		return id
	}
	return Default()
}
