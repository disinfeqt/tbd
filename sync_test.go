package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessSyncRawKeepsUnwrappedTweetResult(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() {
		DB = originalDB
	})

	require.NoError(t, InitDB(":memory:"))

	raw := json.RawMessage(`{
		"data": {
			"bookmark_timeline_v2": {
				"timeline": {
					"instructions": [{
						"type": "TimelineAddEntries",
						"entries": [{
							"content": {
								"itemContent": {
									"tweet_results": {
										"result": {
											"legacy": {
												"id_str": "tweet-1",
												"full_text": "hello",
												"created_at": "Mon Jan 02 15:04:05 +0000 2006"
											},
											"core": {
												"user_results": {
													"result": {
														"legacy": {
															"name": "Alice",
															"screen_name": "alice"
														}
													}
												}
											}
										}
									}
								}
							}
						}]
					}]
				}
			}
		}
	}`)

	response := ProcessSyncRaw(raw)

	require.True(t, response.Success)
	assert.Equal(t, 1, response.SavedCount)

	var tweet TweetModel
	require.NoError(t, DB.First(&tweet, "id = ?", "tweet-1").Error)
	assert.Equal(t, "alice", tweet.ScreenName)
}

func TestProcessSyncRawStoresImageMediaWhenImageDownloadsDisabled(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() {
		DB = originalDB
	})

	withTestConfig(t, Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: false,
	})
	require.NoError(t, InitDB(":memory:"))

	raw := json.RawMessage(`{
		"data": {
			"bookmark_timeline_v2": {
				"timeline": {
					"instructions": [{
						"type": "TimelineAddEntries",
						"entries": [{
							"content": {
								"itemContent": {
									"tweet_results": {
										"result": {
											"legacy": {
												"id_str": "tweet-with-image",
												"full_text": "hello",
												"created_at": "Mon Jan 02 15:04:05 +0000 2006",
												"extended_entities": {
													"media": [{
														"id_str": "media-1",
														"type": "photo",
														"media_url_https": "https://pbs.twimg.com/media/test.jpg"
													}]
												}
											},
											"core": {
												"user_results": {
													"result": {
														"legacy": {
															"name": "Alice",
															"screen_name": "alice"
														}
													}
												}
											}
										}
									}
								}
							}
						}]
					}]
				}
			}
		}
	}`)

	response := ProcessSyncRaw(raw)
	require.True(t, response.Success)
	assert.Equal(t, 1, response.SavedCount)

	var media MediaModel
	require.NoError(t, DB.First(&media, "id = ?", "media-1").Error)
	assert.Equal(t, "photo", media.Type)
	assert.False(t, media.Downloaded)
}

func TestProcessSyncRawBackfillsMediaForExistingTweet(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() {
		DB = originalDB
	})

	withTestConfig(t, Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: false,
	})
	require.NoError(t, InitDB(":memory:"))
	require.NoError(t, DB.Create(&TweetModel{
		ID:         "tweet-with-missing-media",
		ScreenName: "alice",
	}).Error)

	raw := json.RawMessage(`{
		"data": {
			"bookmark_timeline_v2": {
				"timeline": {
					"instructions": [{
						"type": "TimelineAddEntries",
						"entries": [{
							"content": {
								"itemContent": {
									"tweet_results": {
										"result": {
											"legacy": {
												"id_str": "tweet-with-missing-media",
												"full_text": "hello",
												"created_at": "Mon Jan 02 15:04:05 +0000 2006",
												"extended_entities": {
													"media": [{
														"id_str": "media-1",
														"type": "photo",
														"media_url_https": "https://pbs.twimg.com/media/test.jpg"
													}]
												}
											},
											"core": {
												"user_results": {
													"result": {
														"legacy": {
															"name": "Alice",
															"screen_name": "alice"
														}
													}
												}
											}
										}
									}
								}
							}
						}]
					}]
				}
			}
		}
	}`)

	response := ProcessSyncRaw(raw)
	require.True(t, response.Success)
	assert.Equal(t, 0, response.SavedCount)

	var media MediaModel
	require.NoError(t, DB.First(&media, "id = ?", "media-1").Error)
	assert.Equal(t, "tweet-with-missing-media", media.TweetID)
	assert.False(t, media.Downloaded)
}

func TestProcessSyncRawKeepsSavingAfterDuplicateLimitWithinBatch(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() {
		DB = originalDB
	})

	require.NoError(t, InitDB(":memory:"))
	for i := 0; i < DUPLICATE_THRESHOLD; i++ {
		require.NoError(t, DB.Create(&TweetModel{
			ID:         fmt.Sprintf("duplicate-%d", i),
			ScreenName: "alice",
		}).Error)
	}

	entries := make([]string, 0, DUPLICATE_THRESHOLD+1)
	for i := 0; i < DUPLICATE_THRESHOLD; i++ {
		entries = append(entries, bookmarkEntryJSON(fmt.Sprintf("duplicate-%d", i), "alice"))
	}
	entries = append(entries, bookmarkEntryJSON("new-after-duplicates", "bob"))

	response := ProcessSyncRaw(bookmarkTimelineJSON(entries...))

	require.True(t, response.Success)
	assert.True(t, response.DuplicateLimitReached)
	assert.Equal(t, 1, response.SavedCount)

	var tweet TweetModel
	require.NoError(t, DB.First(&tweet, "id = ?", "new-after-duplicates").Error)
	assert.Equal(t, "bob", tweet.ScreenName)
}

func bookmarkTimelineJSON(entries ...string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
		"data": {
			"bookmark_timeline_v2": {
				"timeline": {
					"instructions": [{
						"type": "TimelineAddEntries",
						"entries": [%s]
					}]
				}
			}
		}
	}`, strings.Join(entries, ",")))
}

func bookmarkEntryJSON(id string, screenName string) string {
	return fmt.Sprintf(`{
		"content": {
			"itemContent": {
				"tweet_results": {
					"result": {
						"legacy": {
							"id_str": %q,
							"full_text": "hello",
							"created_at": "Mon Jan 02 15:04:05 +0000 2006"
						},
						"core": {
							"user_results": {
								"result": {
									"legacy": {
										"name": "Test User",
										"screen_name": %q
									}
								}
							}
						}
					}
				}
			}
		}
	}`, id, screenName)
}
