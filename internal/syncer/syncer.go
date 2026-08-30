// Package syncer turns raw GraphQL bookmark responses into database rows.
package syncer

import (
	"encoding/json"
	"net/url"
	"regexp"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"twitter-bookmarks-downloader/internal/download"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/store"
	"twitter-bookmarks-downloader/internal/twitter"
)

/*
Architecture and Logic Overview:

This file handles the synchronization of Twitter bookmarks from raw GraphQL responses
intercepted by the userscript.

Key Challenges with Twitter API:
1.  Deeply Nested Structure: The data we need (screen_name, full_text, media) is buried deep within
    `data.bookmark_timeline_v2.timeline.instructions...`.
2.  Polymorphic Results: The `result` field in `user_results` is not always a simple User object.
    It can be a wrapper containing `__typename`, or sometimes it's missing entire sections if the user is suspended.
3.  Redundant & Inconsistent Paths: Information like `screen_name` appears in multiple places (`legacy.screen_name`,
    `core.screen_name`), but not always consistently across different tweet types (original, retweet, quote).

Our Parsing Strategy (in processRawTweetResults):
1.  Comprehensive Struct Mapping: We define a single, large struct that maps to the most common
    successful response pattern observed (User object nested in `core.user_results.result`).
    We attempt to extract data from multiple known paths within this struct (e.g., trying both `Legacy` and `Core`
    sub-fields for screen_name).
2.  Regex Fallback (The Safety Net): If struct parsing fails (e.g., due to a new wrapper layer or
    unexpected nulls), we fall back to a regex search on the raw JSON string for `"screen_name":"..."`.
    This ensures that even if the structural shape changes slightly, we can still likely identify the user
    and save the tweet (since preserving the RawJSON allows for future re-parsing).
3.  Raw Preservation: We always save the original JSON bytes into the database (`RawJSON` column).
    This is crucial for data integrity and allows fixing parsing logic later without losing data.
*/

const DuplicateThreshold = 5 // Stop syncing if we encounter this many existing tweets in a row

type SyncResponse struct {
	Success               bool   `json:"success"`
	Message               string `json:"message"`
	DuplicateLimitReached bool   `json:"duplicate_limit_reached"`
	SavedCount            int    `json:"saved_count"`
	// LibraryCount is how many bookmarks the database holds in total once this
	// batch is in, so the userscript can show the real library size instead of a
	// per-session tally.
	LibraryCount int64 `json:"library_count"`
}

// screenNameRegex is our last line of defense to extract the username if JSON structural parsing fails.
var screenNameRegex = regexp.MustCompile(`"screen_name"\s*:\s*"([^"]+)"`)

// ProcessSyncRaw acts as the entry point. It unwraps the top-level GraphQL envelope
// to find the actual list of tweet entries.
func ProcessSyncRaw(fullJSON json.RawMessage, accountID string) SyncResponse {
	// 1. Unwrap the outer GraphQL envelope to access the timeline
	var resp struct {
		Data struct {
			BookmarkTimeline struct {
				Timeline struct {
					Instructions []struct {
						Type    string            `json:"type"`
						Entries []json.RawMessage `json:"entries"`
					} `json:"instructions"`
				} `json:"timeline"`
			} `json:"bookmark_timeline_v2"`
		} `json:"data"`
	}

	if err := json.Unmarshal(fullJSON, &resp); err != nil {
		logx.Error(err)
		return SyncResponse{Success: false, Message: "JSON unmarshal error"}
	}

	// 2. Iterate through instructions to find "TimelineAddEntries".
	// Twitter sometimes splits data across different instruction types, but "AddEntries" is the primary one for lists.
	var rawTweets []json.RawMessage
	var foundTypes []string

	for _, inst := range resp.Data.BookmarkTimeline.Timeline.Instructions {
		foundTypes = append(foundTypes, inst.Type)
		if inst.Type == "TimelineAddEntries" {
			for _, entryMsg := range inst.Entries {
				// Each entry might be a Tweet, a Promoted Tweet, or a Cursor.
				// We dig into content.itemContent.tweet_results.result to find the actual tweet data.
				var entry struct {
					Content struct {
						ItemContent struct {
							TweetResults struct {
								Result json.RawMessage `json:"result"`
							} `json:"tweet_results"`
						} `json:"itemContent"`
					} `json:"content"`
				}
				if err := json.Unmarshal(entryMsg, &entry); err == nil && entry.Content.ItemContent.TweetResults.Result != nil {
					rawTweets = append(rawTweets, entry.Content.ItemContent.TweetResults.Result)
				}
			}
		}
	}

	if len(rawTweets) == 0 {
		logx.Warnf("No tweets found in this batch (instruction types: %v)", foundTypes)
	} else {
		logx.Infof("Batch contains %d bookmarks, checking for new ones...", len(rawTweets))
	}

	// 3. Normalize the tweet objects.
	// Sometimes the result IS the tweet (typename="Tweet"), sometimes it WRAPS the tweet (typename="TweetWithVisibilityResults").
	var cleanTweets []json.RawMessage
	for _, rt := range rawTweets {
		var wrap struct {
			Typename string          `json:"__typename"`
			Tweet    json.RawMessage `json:"tweet"`
		}
		if err := json.Unmarshal(rt, &wrap); err == nil {
			if wrap.Tweet != nil {
				cleanTweets = append(cleanTweets, wrap.Tweet)
				continue
			}
		}
		cleanTweets = append(cleanTweets, rt)
	}

	return processRawTweetResults(cleanTweets, accountID)
}

// processRawTweetResults iterates over individual tweet JSON objects, extracts metadata,
// checks for duplicates, and saves them to the database.
func processRawTweetResults(results []json.RawMessage, accountID string) SyncResponse {
	// 1. Parse every tweet up front so duplicates can be checked in one query
	// and all inserts can share one transaction.
	type parsedTweet struct {
		raw        json.RawMessage
		id         string
		name       string
		screenName string
		fullText   string
		createdAt  string
		media      []twitter.MediaEntity
	}

	parsed := make([]parsedTweet, 0, len(results))
	ids := make([]string, 0, len(results))

	for _, res := range results {
		// Parse minimal fields required for indexing using a comprehensive struct matching common patterns.
		var tweet struct {
			Legacy struct {
				IDStr            string `json:"id_str"`
				FullText         string `json:"full_text"`
				CreatedAt        string `json:"created_at"`
				ExtendedEntities struct {
					Media []twitter.MediaEntity `json:"media"`
				} `json:"extended_entities"`
			} `json:"legacy"`
			Core struct {
				UserResults struct {
					Result struct {
						Core struct {
							Name       string `json:"name"`
							ScreenName string `json:"screen_name"`
						} `json:"core"`
						Legacy struct {
							Name       string `json:"name"`
							ScreenName string `json:"screen_name"`
						} `json:"legacy"`
					} `json:"result"`
				} `json:"user_results"`
			} `json:"core"`
		}

		if err := json.Unmarshal(res, &tweet); err != nil {
			continue
		}

		tweetID := tweet.Legacy.IDStr
		if tweetID == "" {
			continue
		}

		// Extract Screen Name with fallback logic.
		// Try legacy path first, then core path.
		screenName := tweet.Core.UserResults.Result.Legacy.ScreenName
		if screenName == "" {
			screenName = tweet.Core.UserResults.Result.Core.ScreenName
		}
		name := tweet.Core.UserResults.Result.Legacy.Name
		if name == "" {
			name = tweet.Core.UserResults.Result.Core.Name
		}

		// Regex Fallback: If struct parsing failed (likely due to unexpected JSON structure or nesting),
		// try to find the screen_name directly from the raw string. This is critical for generating correct filenames.
		if screenName == "" {
			matches := screenNameRegex.FindStringSubmatch(string(res))
			if len(matches) > 1 {
				screenName = matches[1]
				logx.Warnf("Recovered ScreenName via regex for tweet %s: %s", tweetID, screenName)
			} else {
				logx.Warnf("Failed to extract ScreenName for tweet %s", tweetID)
			}
		}

		parsed = append(parsed, parsedTweet{
			raw:        res,
			id:         tweetID,
			name:       name,
			screenName: screenName,
			fullText:   tweet.Legacy.FullText,
			createdAt:  tweet.Legacy.CreatedAt,
			media:      tweet.Legacy.ExtendedEntities.Media,
		})
		ids = append(ids, tweetID)
	}

	// 2. Two lookups, each a single IN query: which tweets are stored at all, and
	// which ones this account already has bookmarked. A tweet another account
	// archived is still new to this one — it just does not need a second copy of
	// the row or a second download.
	stored := make(map[string]struct{}, len(ids))
	mine := make(map[string]struct{}, len(ids))
	if len(ids) > 0 {
		var storedIDs []string
		if err := store.DB.Model(&store.TweetModel{}).Where("id IN ?", ids).Pluck("id", &storedIDs).Error; err != nil {
			logx.Error(err)
		}
		for _, id := range storedIDs {
			stored[id] = struct{}{}
		}

		var myIDs []string
		if err := store.DB.Model(&store.AccountBookmarkModel{}).
			Where("account_id = ? AND tweet_id IN ?", accountID, ids).
			Pluck("tweet_id", &myIDs).Error; err != nil {
			logx.Error(err)
		}
		for _, id := range myIDs {
			mine[id] = struct{}{}
		}
	}

	savedCount := 0
	duplicateStreak := 0
	duplicateLimitReached := false

	// Debug info for sparse saves
	type savedInfo struct {
		URL        string
		MediaFiles []string
	}
	var savedDebug []savedInfo

	// 3. Save the whole batch inside one transaction: SQLite commits (and fsyncs)
	// once per batch instead of once per tweet.
	txErr := store.DB.Transaction(func(tx *gorm.DB) error {
		for _, pt := range parsed {
			if _, isDuplicate := mine[pt.id]; isDuplicate {
				backfilled, err := createMissingMedia(tx, pt.id, pt.media)
				if err != nil {
					logx.Error(err)
				} else if backfilled > 0 {
					logx.Infof("Backfilled %d media records for existing tweet %s", backfilled, pt.id)
				}

				duplicateStreak++
				if duplicateStreak >= DuplicateThreshold {
					duplicateLimitReached = true
				}
				continue
			}
			duplicateStreak = 0

			if err := addBookmark(tx, accountID, pt.id); err != nil {
				logx.Warnf("Failed to bookmark tweet %s for account %s: %v", pt.id, accountID, err)
				continue
			}
			mine[pt.id] = struct{}{} // A repeated ID later in this batch is a duplicate.

			// Another account already archived the tweet, so the row and its files
			// stay put; this account simply gained a bookmark.
			if _, alreadyStored := stored[pt.id]; alreadyStored {
				savedCount++
				continue
			}

			// Parse creation time.
			createdAt, _ := time.Parse(time.RubyDate, pt.createdAt)
			if createdAt.IsZero() {
				createdAt = time.Now()
			}

			// Construct the TweetModel and MediaModels
			// Note: We store the raw JSON payload to allow for future re-processing or data recovery.
			tm := &store.TweetModel{
				ID:           pt.id,
				AccountID:    accountID,
				FullText:     pt.fullText,
				Name:         pt.name,
				ScreenName:   pt.screenName,
				CreatedAt:    createdAt,
				PermanentURL: "https://x.com/" + pt.screenName + "/status/" + pt.id,
				RawJSON:      string(pt.raw),
				SyncedAt:     time.Now(),
			}

			var mediaFilenames []string
			mediaCount := len(pt.media)
			tm.Media = mediaModelsForTweet(pt.id, pt.media)

			for _, media := range tm.Media {
				// Generate simulated filename for debug
				if download.ShouldDownload(media) {
					if parsedURL, err := url.Parse(media.URL); err == nil {
						mediaFilenames = append(mediaFilenames, download.BuildFilename(tm, media.Index, mediaCount, parsedURL))
					}
				}
			}

			if err := tx.Create(tm).Error; err != nil {
				// Swallowing this looks exactly like "the sync found nothing new",
				// and the tweet would quietly never come back on a later pass.
				logx.Warnf("Failed to save tweet %s: %v", pt.id, err)
				continue
			}

			savedCount++
			stored[pt.id] = struct{}{}
			savedDebug = append(savedDebug, savedInfo{
				URL:        tm.PermanentURL,
				MediaFiles: mediaFilenames,
			})
		}
		return nil
	})
	if txErr != nil {
		logx.Error(txErr)
		return SyncResponse{Success: false, Message: "Database error"}
	}

	if savedCount > 0 {
		download.Nudge()
	}

	var libraryCount int64
	if err := store.DB.Model(&store.AccountBookmarkModel{}).
		Where("account_id = ?", accountID).Count(&libraryCount).Error; err != nil {
		logx.Error(err)
	}

	logx.Infof("Batch done: %d new bookmarks saved", savedCount)

	// Debug: Identify why we are saving items after hitting duplicate limit
	if duplicateLimitReached && savedCount > 0 {
		for _, info := range savedDebug {
			logx.Warnf("  [Sparse Save] URL: %s", info.URL)
			for _, mf := range info.MediaFiles {
				logx.Warnf("                Media: %s", mf)
			}
		}
	}

	return SyncResponse{
		Success:               true,
		Message:               "Raw sync complete",
		DuplicateLimitReached: duplicateLimitReached,
		SavedCount:            savedCount,
		LibraryCount:          libraryCount,
	}
}

// addBookmark records that this account has the tweet bookmarked. Repeats are
// ignored, so re-scanning a page costs nothing.
func addBookmark(db *gorm.DB, accountID, tweetID string) error {
	membership := store.AccountBookmarkModel{
		AccountID: accountID,
		TweetID:   tweetID,
		SyncedAt:  time.Now(),
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&membership).Error
}

func mediaModelsForTweet(tweetID string, entities []twitter.MediaEntity) []store.MediaModel {
	mediaModels := make([]store.MediaModel, 0, len(entities))

	for i, m := range entities {
		downloadURL := twitter.DownloadURLForMedia(m)
		if downloadURL == "" {
			continue
		}

		mediaModels = append(mediaModels, store.MediaModel{
			ID:         m.IDStr,
			TweetID:    tweetID,
			Index:      i,
			Type:       m.Type,
			URL:        downloadURL,
			Width:      m.OriginalInfo.Width,
			Height:     m.OriginalInfo.Height,
			DurationMs: m.VideoInfo.DurationMillis,
		})
	}

	return mediaModels
}

func createMissingMedia(db *gorm.DB, tweetID string, entities []twitter.MediaEntity) (int64, error) {
	mediaModels := mediaModelsForTweet(tweetID, entities)
	if len(mediaModels) == 0 {
		return 0, nil
	}

	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&mediaModels)
	return result.RowsAffected, result.Error
}
