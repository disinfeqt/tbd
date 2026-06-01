package main

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"

	"github.com/rotisserie/eris"
)

type tweetVideoVariant struct {
	Bitrate     int    `json:"bitrate"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}

type tweetVideoInfo struct {
	Variants []tweetVideoVariant `json:"variants"`
}

type tweetMediaEntity struct {
	IDStr         string         `json:"id_str"`
	Type          string         `json:"type"`
	MediaURLHttps string         `json:"media_url_https"`
	VideoInfo     tweetVideoInfo `json:"video_info"`
}

func downloadURLForMedia(m tweetMediaEntity) string {
	if url := bestMP4VariantURL(m.VideoInfo.Variants); url != "" {
		return url
	}

	return m.MediaURLHttps
}

func isMP4MediaURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err == nil {
		return strings.EqualFold(path.Ext(parsedURL.Path), ".mp4")
	}

	return strings.Contains(strings.ToLower(rawURL), ".mp4")
}

func bestMP4VariantURL(variants []tweetVideoVariant) string {
	bestIndex := -1
	bestBitrate := -1

	for i, variant := range variants {
		if variant.URL == "" {
			continue
		}

		if !strings.EqualFold(variant.ContentType, "video/mp4") && !strings.Contains(variant.URL, ".mp4") {
			continue
		}

		if bestIndex == -1 || variant.Bitrate > bestBitrate {
			bestIndex = i
			bestBitrate = variant.Bitrate
		}
	}

	if bestIndex == -1 {
		return ""
	}

	return variants[bestIndex].URL
}

func mediaEntitiesFromRawTweet(rawJSON string) ([]tweetMediaEntity, error) {
	var tweet struct {
		Legacy struct {
			Entities struct {
				Media []tweetMediaEntity `json:"media"`
			} `json:"entities"`
			ExtendedEntities struct {
				Media []tweetMediaEntity `json:"media"`
			} `json:"extended_entities"`
		} `json:"legacy"`
	}

	if err := json.Unmarshal([]byte(rawJSON), &tweet); err != nil {
		return nil, err
	}

	if len(tweet.Legacy.ExtendedEntities.Media) > 0 {
		return tweet.Legacy.ExtendedEntities.Media, nil
	}

	return tweet.Legacy.Entities.Media, nil
}

func matchingMediaEntity(media MediaModel, entities []tweetMediaEntity) (tweetMediaEntity, bool) {
	if media.ID != "" {
		for _, entity := range entities {
			if entity.IDStr == media.ID {
				return entity, true
			}
		}
	}

	if media.Index >= 0 && media.Index < len(entities) {
		return entities[media.Index], true
	}

	return tweetMediaEntity{}, false
}

func RepairVideoMediaURLs() (int, error) {
	var mediaList []MediaModel
	if err := DB.
		Where("type IN ? AND url NOT LIKE ?", []string{"video", "animated_gif"}, "%.mp4%").
		Find(&mediaList).Error; err != nil {
		return 0, eris.Wrap(err, "failed to query video media for repair")
	}

	repaired := 0

	for _, media := range mediaList {
		var tweet TweetModel
		if err := DB.Select("id", "raw_json").First(&tweet, "id = ?", media.TweetID).Error; err != nil {
			PrintWarningF("Skipping video media repair for %s: raw tweet JSON not found", media.ID)
			continue
		}

		entities, err := mediaEntitiesFromRawTweet(tweet.RawJSON)
		if err != nil {
			PrintWarningF("Skipping video media repair for %s: raw tweet JSON could not be parsed", media.ID)
			continue
		}

		entity, ok := matchingMediaEntity(media, entities)
		if !ok {
			continue
		}

		nextURL := downloadURLForMedia(entity)
		if nextURL == "" || nextURL == media.URL {
			continue
		}

		if err := DB.Model(&MediaModel{}).
			Where("id = ?", media.ID).
			Updates(map[string]interface{}{
				"url":         nextURL,
				"downloaded":  false,
				"failed":      false,
				"retry_count": 0,
			}).Error; err != nil {
			return repaired, eris.Wrapf(err, "failed to update media %s", media.ID)
		}

		repaired++
	}

	return repaired, nil
}
