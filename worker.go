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
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ticker.C:
			DownloadPendingMedia()
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
			PrintError(eris.Wrapf(err, "Failed to download media %s", media.ID))
			media.RetryCount++
			if media.RetryCount >= 3 {
				media.Failed = true
			}
		} else {
			media.Downloaded = true
		}
		DB.Save(&media)
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
	DB.Model(&MediaModel{}).Where("tweet_id = ?", tweet.ID).Count(&mediaCount)

	filename := buildFilename(&tweet, media.Index, int(mediaCount), parsedURL)
	outputPath := path.Join(config.MediaDir, filename)

	if err := downloadFile(parsedURL.String(), outputPath, tweet.CreatedAt); err != nil {
		return err
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
		if err := os.MkdirAll(config.MediaDir, 0755); err != nil {
			return err
		}
	}

	// 增强：像原版一样校验文件大小
	if fileInfo, err := os.Stat(outputPath); err == nil {
		resp, err := http.Head(urlStr)
		if err == nil && resp.ContentLength > 0 {
			if fileInfo.Size() == resp.ContentLength {
				PrintInfoF("  Skipped: %s", outputPath)
				return nil
			}
		}
		// 如果大小不一致或 Head 失败，继续下载（覆盖）
	}

	resp, err := http.Get(urlStr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	os.Chtimes(outputPath, time.Now(), modTime)
	PrintInfoF("  Downloaded: %s", outputPath)
	return nil
}
