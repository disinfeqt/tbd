package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	twitterscraper "github.com/imperatrona/twitter-scraper"
	"github.com/rotisserie/eris"
)

func saveTweet(tweet *twitterscraper.Tweet, cfg Config) error {
	filename := fmt.Sprintf("twitter-@%s-%s-%s.json",
		tweet.Username,
		tweet.TimeParsed.In(time.Local).Format("20060102-150405"),
		tweet.ID,
	)
	outputPath := path.Join(cfg.TweetsDir, filename)

	jsonData, err := json.MarshalIndent(tweet, "", "  ")
	if err != nil {
		return eris.Wrap(err, "failed to generate JSON")
	}

	if _, err := os.Stat(cfg.TweetsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.TweetsDir, 0755); err != nil {
			return eris.Wrap(err, "failed to create tweets directory")
		}
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return eris.Wrap(err, "failed to create tweet file")
	}

	_, err = f.Write(jsonData)
	if err != nil {
		return eris.Wrap(err, "failed to write tweet file")
	}

	err = os.Chtimes(outputPath, time.Now(), tweet.TimeParsed)
	if err != nil {
		return eris.Wrap(err, "failed to set modified time")
	}

	PrintInfoF("  Saved: %s", outputPath)
	return nil
}

func downloadMedia(tweet *twitterscraper.Tweet, cfg Config) error {
	var errors []error
	mediaCount := len(tweet.OrderedMedia)

	if mediaCount > 0 {
		for i, media := range tweet.OrderedMedia {
			parsedURL, err := url.Parse(media.URL)
			if err != nil {
				errors = append(errors, eris.Wrap(err, "failed to parse media URL"))
			}

			if media.Type == twitterscraper.MediaTypePhoto {
				params := parsedURL.Query()
				params.Set("name", "orig")
				parsedURL.RawQuery = params.Encode()
			}

			filename, err := buildTweetMediaFilename(parsedURL, i, mediaCount, tweet)
			if err != nil {
				errors = append(errors, err)
				continue
			}

			if err := downloadFile(parsedURL.String(), filename, tweet, cfg); err != nil {
				errors = append(errors, err)
			}
		}
	}

	if len(errors) > 0 {
		return eris.Errorf("failed to download %d photos: %v", len(errors), errors)
	}

	return nil
}

func buildTweetMediaFilename(url *url.URL, index int, total int, tweet *twitterscraper.Tweet) (string, error) {
	// Get the file extension (keep as is)
	fileExt := strings.ToLower(path.Ext(url.Path))

	var filename string

	// Format: twitter-@from_yukana-20250105-150007-1875920219195195742.jpg
	filenameBase := fmt.Sprintf("twitter-@%s-%s-%s",
		tweet.Username,
		tweet.TimeParsed.In(time.Local).Format("20060102-150405"),
		tweet.ID,
	)

	if total > 1 {
		filename = fmt.Sprintf("%s-%d%s", filenameBase, index, fileExt)
	} else {
		filename = fmt.Sprintf("%s%s", filenameBase, fileExt)
	}

	return filename, nil
}

func downloadFile(urlStr string, filename string, tweet *twitterscraper.Tweet, cfg Config) error {
	outputPath := path.Join(cfg.MediaDir, filename)

	// Create output directory if it doesn't exist
	if _, err := os.Stat(cfg.MediaDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.MediaDir, 0755); err != nil {
			return eris.Wrap(err, "failed to create media directory")
		}
	}

	// Skip if file already downloaded
	if fileInfo, err := os.Stat(outputPath); err == nil {
		resp, err := http.Head(urlStr)
		if err != nil {
			PrintWarning("Failed to check remote file size, force downloading")
		} else {
			if resp.ContentLength > 0 {
				if fileInfo.Size() == resp.ContentLength {
					PrintInfoF("  Skipped: %s", outputPath)
					return nil
				}
			}
		}
	}

	// Send HTTP GET request
	resp, err := http.Get(urlStr)
	if err != nil {
		return eris.Wrap(err, "failed to download file from URL")
	}
	defer CloseResource(resp.Body)

	// Check response status code
	if resp.StatusCode != http.StatusOK {
		return eris.Errorf("failed to download file, status code: %d", resp.StatusCode)
	}

	// Create local file
	out, err := os.Create(outputPath)
	if err != nil {
		return eris.Wrap(err, "failed to create local file")
	}
	defer CloseResource(out)

	// Copy content from response body to local file
	fileSize, err := io.Copy(out, resp.Body)
	if err != nil {
		return eris.Wrap(err, "failed to copy content to local file")
	}

	if resp.ContentLength > 0 {
		if fileSize != resp.ContentLength {
			return eris.Errorf("download incomplete, expected %d bytes, got %d bytes.", resp.ContentLength, fileSize)
		}
	} else {
		PrintWarning("Content-Length header not provided by the server.")
	}

	err = os.Chtimes(outputPath, time.Now(), tweet.TimeParsed)
	if err != nil {
		return eris.Wrap(err, "failed to set modified time")
	}

	PrintInfoF("  Downloaded: %s", outputPath)
	return nil
}
