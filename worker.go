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

	"github.com/rotisserie/eris"
)

func StartDownloadWorker() {
	PrintInfo("Download worker started")

	// Initial report on startup (force display)
	ReportWorkerStatus(true)

	ticker := time.NewTicker(5 * time.Second)
	statusTicker := time.NewTicker(10 * time.Minute)

	for {
		select {
		case <-ticker.C:
			DownloadPendingMedia()
		case <-statusTicker.C:
			ReportWorkerStatus(false)
		}
	}
}

func ReportWorkerStatus(force bool) {
	var pending int64
	var failed int64
	if err := DB.Model(&MediaModel{}).Where("downloaded = ? AND failed = ?", false, false).Count(&pending).Error; err != nil {
		PrintError(eris.Wrap(err, "Failed to count pending media"))
	}
	if err := DB.Model(&MediaModel{}).Where("failed = ?", true).Count(&failed).Error; err != nil {
		PrintError(eris.Wrap(err, "Failed to count failed media"))
	}

	if force || pending > 0 || failed > 0 {
		PrintInfoF("[Worker Status] Pending: %d | Failed: %d", pending, failed)
		if failed > 0 {
			PrintWarningF("  %d media items failed permanently after multiple retries.", failed)
		}
	}
}

func DownloadPendingMedia() {
	var mediaList []MediaModel
	// Find up to 5 pending downloads
	err := DB.Where("downloaded = ? AND failed = ?", false, false).Limit(5).Find(&mediaList).Error
	if err != nil {
		PrintError(eris.Wrap(err, "Failed to query pending media"))
		return
	}

	if len(mediaList) == 0 {
		return
	}

	for _, media := range mediaList {
		err := processMediaDownload(&media)
		if err != nil {
			PrintError(eris.Wrapf(err, "Media ID: %s", media.ID))
			media.RetryCount++
			if media.RetryCount >= 3 {
				media.Failed = true
			}
		} else {
			media.Downloaded = true
		}
		if err := DB.Save(&media).Error; err != nil {
			PrintError(eris.Wrapf(err, "Failed to update media status for %s", media.ID))
		}
	}
}

func processMediaDownload(media *MediaModel) error {
	var tweet TweetModel
	if err := DB.First(&tweet, "id = ?", media.TweetID).Error; err != nil {
		return eris.Wrap(err, "Tweet not found for media")
	}

	parsedURL, err := url.Parse(media.URL)
	if err != nil {
		return err
	}

	// 规则 5: Adjust URL for high quality photos
	if media.Type == "photo" {
		params := parsedURL.Query()
		params.Set("name", "orig")
		parsedURL.RawQuery = params.Encode()
	}

	// Determine total media count for this tweet
	var mediaCount int64
	if err := DB.Model(&MediaModel{}).Where("tweet_id = ?", tweet.ID).Count(&mediaCount).Error; err != nil {
		return eris.Wrap(err, "Failed to count tweet media")
	}

	filename := buildFilename(&tweet, media.Index, int(mediaCount), parsedURL)
	outputPath := path.Join(config.MediaDir, filename)

	if err := downloadFile(parsedURL.String(), outputPath, tweet.CreatedAt); err != nil {
		return eris.Wrapf(err, "URL: %s (from Tweet: %s)", media.URL, tweet.PermanentURL)
	}

	return nil
}

// 规则 1, 2, 3, 4: 严格遵循原版文件名规则
func buildFilename(tweet *TweetModel, index int, total int, url *url.URL) string {
	fileExt := strings.ToLower(path.Ext(url.Path))
	filenameBase := fmt.Sprintf("twitter-@%s-%s-%s",
		tweet.ScreenName,
		tweet.CreatedAt.In(time.Local).Format("20060102-150405"),
		tweet.ID,
	)

	if total > 1 {
		return fmt.Sprintf("%s-%d%s", filenameBase, index, fileExt)
	}
	return fmt.Sprintf("%s%s", filenameBase, fileExt)
}

func downloadFile(urlStr string, outputPath string, modTime time.Time) error {
	if _, err := os.Stat(config.MediaDir); os.IsNotExist(err) {
		if err := os.MkdirAll(config.MediaDir, 0o755); err != nil {
			return eris.Wrap(err, "failed to create media directory")
		}
	}

	// 像原版一样校验文件大小
	if fileInfo, err := os.Stat(outputPath); err == nil {
		resp, err := http.Head(urlStr)
		if err != nil {
			PrintWarning("Failed to check remote file size, force downloading")
		} else {
			if resp.ContentLength > 0 {
				if fileInfo.Size() == resp.ContentLength {
					if err := resp.Body.Close(); err != nil {
						PrintWarningF("Failed to close HEAD response body: %v", err)
					}
					PrintInfoF("  Skipped: %s", outputPath)
					return nil
				}
			}
			if err := resp.Body.Close(); err != nil {
				PrintWarningF("Failed to close HEAD response body: %v", err)
			}
		}
	}

	resp, err := http.Get(urlStr)
	if err != nil {
		return eris.Wrap(err, "failed to download file from URL")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			PrintWarningF("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return eris.Errorf("failed to download file, status code: %d", resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return eris.Wrap(err, "failed to create local file")
	}

	written, copyErr := io.Copy(out, resp.Body)

	// Explicitly close file to ensure flush and release lock
	closeErr := out.Close()

	if copyErr != nil {
		return eris.Wrap(copyErr, "failed to copy content to local file")
	}
	if closeErr != nil {
		return eris.Wrap(closeErr, "failed to close local file")
	}

	// Verify download completeness
	if resp.ContentLength > 0 {
		if written != resp.ContentLength {
			return eris.Errorf("download incomplete, expected %d bytes, got %d bytes", resp.ContentLength, written)
		}
	} else {
		PrintWarning("Content-Length header not provided by the server.")
	}

	// Restore strict error check for Chtimes as per legacy code
	if err := os.Chtimes(outputPath, time.Now(), modTime); err != nil {
		return eris.Wrap(err, "failed to set modified time")
	}

	PrintInfoF("  Downloaded: %s", outputPath)
	return nil
}
