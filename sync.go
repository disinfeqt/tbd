package main

import (
	"encoding/json"
	"regexp"
	"time"
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

const DUPLICATE_THRESHOLD = 5 // Stop syncing if we encounter this many existing tweets in a row

// screenNameRegex is our last line of defense to extract the username if JSON structural parsing fails.
var screenNameRegex = regexp.MustCompile(`"screen_name"\s*:\s*"([^"]+)"`)

// ProcessSyncRaw acts as the entry point. It unwraps the top-level GraphQL envelope
// to find the actual list of tweet entries.
func ProcessSyncRaw(fullJSON json.RawMessage) SyncResponse {
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
		PrintError(err)
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
		PrintWarningF("No tweets extracted. Found instruction types: %v", foundTypes)
	} else {
		PrintInfoF("Extracted %d tweets from raw GraphQL response", len(rawTweets))
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
			if wrap.Typename == "Tweet" {
				cleanTweets = append(cleanTweets, rt)
			} else if wrap.Tweet != nil {
				cleanTweets = append(cleanTweets, wrap.Tweet)
			}
		}
	}

	return processRawTweetResults(cleanTweets)
}

// processRawTweetResults iterates over individual tweet JSON objects, extracts metadata,
// checks for duplicates, and saves them to the database.
func processRawTweetResults(results []json.RawMessage) SyncResponse {
	savedCount := 0
	duplicateStreak := 0
	duplicateLimitReached := false

	for _, res := range results {
		// 1. Parse minimal fields required for indexing using a comprehensive struct matching common patterns.
		var tweet struct {
			Legacy struct {
				IDStr            string `json:"id_str"`
				FullText         string `json:"full_text"`
				CreatedAt        string `json:"created_at"`
				ExtendedEntities struct {
					Media []struct {
						IDStr         string `json:"id_str"`
						Type          string `json:"type"`
						MediaURLHttps string `json:"media_url_https"`
					} `json:"media"`
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

		// 2. Extract Screen Name with fallback logic.
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
				PrintWarningF("Recovered ScreenName via regex for tweet %s: %s", tweetID, screenName)
			} else {
				PrintWarningF("Failed to extract ScreenName for tweet %s", tweetID)
			}
		}

		// 3. Check for duplicates in the database.
		var exists int64
		DB.Model(&TweetModel{}).Where("id = ?", tweetID).Count(&exists)
		if exists > 0 {
			duplicateStreak++
			if duplicateStreak >= DUPLICATE_THRESHOLD {
				duplicateLimitReached = true
			}
			continue
		}
		duplicateStreak = 0

		// 4. Parse creation time.
		createdAt, _ := time.Parse(time.RubyDate, tweet.Legacy.CreatedAt)
		if createdAt.IsZero() {
			createdAt = time.Now()
		}

		// 5. Construct the TweetModel.
		// Note: We store the raw JSON payload to allow for future re-processing or data recovery.
		tm := &TweetModel{
			ID:           tweetID,
			FullText:     tweet.Legacy.FullText,
			Name:         name,
			ScreenName:   screenName,
			CreatedAt:    createdAt,
			PermanentURL: "https://x.com/" + screenName + "/status/" + tweetID,
			RawJSON:      string(res),
			SyncedAt:     time.Now(),
		}

		for _, m := range tweet.Legacy.ExtendedEntities.Media {
			tm.Media = append(tm.Media, MediaModel{
				ID:      m.IDStr,
				TweetID: tweetID,
				Type:    m.Type,
				URL:     m.MediaURLHttps,
			})
		}

		if err := DB.Create(tm).Error; err == nil {
			savedCount++
		}
	}

	PrintInfoF("Batch processing complete: %d new tweets saved. (Duplicate limit reached: %v)", savedCount, duplicateLimitReached)

	return SyncResponse{
		Success:               true,
		Message:               "Raw sync complete",
		DuplicateLimitReached: duplicateLimitReached,
		SavedCount:            savedCount,
	}
}