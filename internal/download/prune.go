package download

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	"github.com/rotisserie/eris"
	"gorm.io/gorm"

	"twitter-bookmarks-downloader/internal/accounts"
	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/store"
)

// FindTweetsWithDeletedMedia returns bookmarks whose downloaded media files
// have all been removed from the media folder. The media folder is the source
// of truth: deleting a bookmark's files there means the bookmark itself should
// go. Tweets with no downloaded media (text-only, or downloads still pending)
// are never candidates, and a tweet is kept as long as at least one of its
// downloaded files is still present or cannot be verified.
func FindTweetsWithDeletedMedia() ([]store.TweetModel, error) {
	// Every account's folder has to be readable: an unmounted drive would
	// otherwise make that account's whole archive look deleted.
	for _, account := range accounts.All() {
		dir := accounts.MediaDir(account.ID)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return nil, eris.Errorf("media folder %q is not accessible — refusing to treat every bookmark as deleted", dir)
		}
	}
	if fallback := config.Current().MediaDir; len(accounts.All()) == 0 {
		if info, err := os.Stat(fallback); err != nil || !info.IsDir() {
			return nil, eris.Errorf("media folder %q is not accessible — refusing to treat every bookmark as deleted", fallback)
		}
	}

	var tweets []store.TweetModel
	if err := store.DB.
		Where("EXISTS (SELECT 1 FROM media WHERE media.tweet_id = tweets.id AND media.downloaded = ?)", true).
		Preload("Media").
		Find(&tweets).Error; err != nil {
		return nil, eris.Wrap(err, "failed to query tweets with downloaded media")
	}

	var deleted []store.TweetModel
	for i := range tweets {
		tweet := &tweets[i]
		mediaDir := accounts.MediaDir(tweet.AccountID)
		verified := false
		present := false
		for _, media := range tweet.Media {
			if !media.Downloaded {
				continue
			}
			parsedURL, err := url.Parse(media.URL)
			if err != nil {
				present = true // cannot compute the filename, so cannot prove it is gone
				continue
			}
			name := BuildFilename(tweet, media.Index, len(tweet.Media), parsedURL)
			if _, err := os.Stat(filepath.Join(mediaDir, name)); err == nil || !errors.Is(err, fs.ErrNotExist) {
				present = true
				continue
			}
			verified = true
		}
		if verified && !present {
			deleted = append(deleted, *tweet)
		}
	}
	return deleted, nil
}

// RemoveTweets deletes the given bookmarks and their media records.
func RemoveTweets(tweets []store.TweetModel) error {
	ids := make([]string, 0, len(tweets))
	for _, tweet := range tweets {
		ids = append(ids, tweet.ID)
	}
	return store.DB.Transaction(func(tx *gorm.DB) error {
		const chunkSize = 500 // stay under SQLite's bound-parameter limit
		for start := 0; start < len(ids); start += chunkSize {
			chunk := ids[start:min(start+chunkSize, len(ids))]
			if err := tx.Where("tweet_id IN ?", chunk).Delete(&store.MediaModel{}).Error; err != nil {
				return eris.Wrap(err, "failed to delete media records")
			}
			if err := tx.Where("tweet_id IN ?", chunk).Delete(&store.AccountBookmarkModel{}).Error; err != nil {
				return eris.Wrap(err, "failed to delete account bookmarks")
			}
			if err := tx.Where("id IN ?", chunk).Delete(&store.TweetModel{}).Error; err != nil {
				return eris.Wrap(err, "failed to delete tweets")
			}
		}
		return nil
	})
}
