package main

import (
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

func downloadPhotos(tweet *twitterscraper.Tweet) error {
	var errors []error

	if len(tweet.Photos) > 0 {
		for _, t := range tweet.Photos {
			url := t.URL + "?name=orig"

			if err := downloadFile(url, tweet); err != nil {
				errors = append(errors, err)
			}
		}
	}

	if len(errors) > 0 {
		return eris.Errorf("failed to download %d photos: %v", len(errors), errors)
	}

	return nil
}

func downloadFile(urlStr string, tweet *twitterscraper.Tweet) error {
	// Get the file extension (keep as is)
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return eris.Wrap(err, "failed to parse URL")
	}

	fileExt := strings.ToLower(path.Ext(parsedURL.Path))

	// Format: twitter-@from_yukana-20250105-150007-1875920219195195742.jpg
	outputPath := path.Join(MEDIA_DIR,
		fmt.Sprintf("twitter-@%s-%s-%s%s",
			tweet.Username,
			tweet.TimeParsed.In(time.Local).Format("20060102-150405"),
			tweet.ID,
			fileExt,
		))

	// Create output directory if it doesn't exist
	if _, err := os.Stat(MEDIA_DIR); os.IsNotExist(err) {
		if err := os.MkdirAll(MEDIA_DIR, 0755); err != nil {
			return eris.Wrap(err, "failed to create output directory")
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
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return eris.Wrap(err, "failed to copy content to local file")
	}

	err = os.Chtimes(outputPath, time.Now(), tweet.TimeParsed)
	if err != nil {
		return eris.Wrap(err, "failed to set modified time")
	}

	fmt.Printf("Downloaded: %s to %s\n", urlStr, outputPath)
	return nil
}
