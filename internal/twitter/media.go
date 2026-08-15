// Package twitter contains pure helpers for parsing Twitter/X media payloads.
package twitter

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"
)

type VideoVariant struct {
	Bitrate     int    `json:"bitrate"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}

type VideoInfo struct {
	Variants []VideoVariant `json:"variants"`
}

type MediaEntity struct {
	IDStr         string    `json:"id_str"`
	Type          string    `json:"type"`
	MediaURLHttps string    `json:"media_url_https"`
	VideoInfo     VideoInfo `json:"video_info"`
}

func DownloadURLForMedia(m MediaEntity) string {
	if url := BestMP4VariantURL(m.VideoInfo.Variants); url != "" {
		return url
	}

	return m.MediaURLHttps
}

func IsMP4MediaURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err == nil {
		return strings.EqualFold(path.Ext(parsedURL.Path), ".mp4")
	}

	return strings.Contains(strings.ToLower(rawURL), ".mp4")
}

func BestMP4VariantURL(variants []VideoVariant) string {
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

func MediaEntitiesFromRawTweet(rawJSON string) ([]MediaEntity, error) {
	var tweet struct {
		Legacy struct {
			Entities struct {
				Media []MediaEntity `json:"media"`
			} `json:"entities"`
			ExtendedEntities struct {
				Media []MediaEntity `json:"media"`
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
