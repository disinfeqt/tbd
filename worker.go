package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/rotisserie/eris"
	"gorm.io/gorm"
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
	var paused int64
	var failed int64
	if err := pendingDownloadQuery().Count(&pending).Error; err != nil {
		PrintError(eris.Wrap(err, "Failed to count pending media"))
	}
	if err := DB.Model(&MediaModel{}).Where("downloaded = ? AND failed = ?", false, false).Count(&paused).Error; err == nil {
		paused -= pending
	} else {
		PrintError(eris.Wrap(err, "Failed to count paused media"))
	}
	if err := DB.Model(&MediaModel{}).Where("failed = ?", true).Count(&failed).Error; err != nil {
		PrintError(eris.Wrap(err, "Failed to count failed media"))
	}

	if force || pending > 0 || paused > 0 || failed > 0 {
		PrintInfoF("[Worker Status] Pending: %d | Paused by settings: %d | Failed: %d", pending, paused, failed)
		if failed > 0 {
			PrintWarningF("  %d media items failed permanently after multiple retries.", failed)
		}
	}
}

func pendingDownloadQuery() *gorm.DB {
	query := DB.Model(&MediaModel{}).Where("downloaded = ? AND failed = ?", false, false)
	current := CurrentConfig()

	switch {
	case current.DownloadImages && current.DownloadVideos:
		return query
	case current.DownloadImages:
		return query.Where("type = ?", "photo")
	case current.DownloadVideos:
		return query.Where("type IN ?", []string{"video", "animated_gif"})
	default:
		return query.Where("1 = 0")
	}
}

func DownloadPendingMedia() {
	var total int64
	var pending int64
	if err := DB.Model(&MediaModel{}).Count(&total).Error; err != nil {
		PrintError(eris.Wrap(err, "Failed to count total media"))
		return
	}
	if err := pendingDownloadQuery().Count(&pending).Error; err != nil {
		PrintError(eris.Wrap(err, "Failed to count pending media"))
		return
	}

	var mediaList []MediaModel
	// Find up to 5 pending downloads
	err := pendingDownloadQuery().Limit(5).Find(&mediaList).Error
	if err != nil {
		PrintError(eris.Wrap(err, "Failed to query pending media"))
		return
	}

	if len(mediaList) == 0 {
		return
	}

	completed := total - pending
	for i, media := range mediaList {
		progressLabel := fmt.Sprintf("%d/%d", completed+int64(i)+1, total)
		if !shouldDownloadMedia(media) {
			continue
		}

		err := processMediaDownload(&media, progressLabel)
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

func processMediaDownload(media *MediaModel, progressLabel string) error {
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
	outputPath := filepath.Join(CurrentConfig().MediaDir, filename)

	if err := downloadFile(parsedURL.String(), outputPath, tweet.CreatedAt, progressLabel); err != nil {
		return eris.Wrapf(err, "URL: %s (from Tweet: %s)", media.URL, tweet.PermanentURL)
	}

	return nil
}

const (
	headRequestTimeout       = 20 * time.Second
	downloadTimeout          = 30 * time.Minute
	downloadProgressInterval = 5 * time.Second
)

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

type progressWriter struct {
	label      string
	outputPath string
	total      int64
	written    int64
	lastLog    time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)

	now := time.Now()
	if now.Sub(pw.lastLog) >= downloadProgressInterval {
		pw.lastLog = now
		if pw.total > 0 {
			PrintInfoF("  [Download %s] Downloading: %s (%s/%s)", pw.label, pw.outputPath, formatByteSize(pw.written), formatByteSize(pw.total))
		} else {
			PrintInfoF("  [Download %s] Downloading: %s (%s)", pw.label, pw.outputPath, formatByteSize(pw.written))
		}
	}

	return n, nil
}

func formatByteSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func newTimedRequest(method string, urlStr string, timeout time.Duration) (*http.Request, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	req, err := http.NewRequestWithContext(ctx, method, urlStr, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")
	return req, cancel, nil
}

func downloadFile(urlStr string, outputPath string, modTime time.Time, progressLabel string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return eris.Wrap(err, "failed to create media directory")
	}

	// 像原版一样校验文件大小
	if fileInfo, err := os.Stat(outputPath); err == nil {
		req, cancel, err := newTimedRequest(http.MethodHead, urlStr, headRequestTimeout)
		if err != nil {
			return eris.Wrap(err, "failed to create HEAD request")
		}

		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err != nil {
			PrintWarning("Failed to check remote file size, force downloading")
		} else {
			if resp.ContentLength > 0 {
				if fileInfo.Size() == resp.ContentLength {
					if err := resp.Body.Close(); err != nil {
						PrintWarningF("Failed to close HEAD response body: %v", err)
					}
					PrintInfoF("  [Download %s] Skipped: %s", progressLabel, outputPath)
					return nil
				}
			}
			if err := resp.Body.Close(); err != nil {
				PrintWarningF("Failed to close HEAD response body: %v", err)
			}
		}
	}

	req, cancel, err := newTimedRequest(http.MethodGet, urlStr, downloadTimeout)
	if err != nil {
		return eris.Wrap(err, "failed to create download request")
	}
	defer cancel()

	resp, err := http.DefaultClient.Do(req)
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

	if resp.ContentLength > 0 {
		PrintInfoF("  [Download %s] Starting: %s (%s)", progressLabel, outputPath, formatByteSize(resp.ContentLength))
	} else {
		PrintInfoF("  [Download %s] Starting: %s (unknown size)", progressLabel, outputPath)
	}

	tmpPath := outputPath + ".part"
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		return eris.Wrap(err, "failed to remove stale partial file")
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return eris.Wrap(err, "failed to create local file")
	}

	progress := &progressWriter{
		label:      progressLabel,
		outputPath: outputPath,
		total:      resp.ContentLength,
		lastLog:    time.Now(),
	}
	written, copyErr := io.Copy(out, io.TeeReader(resp.Body, progress))

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

	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return eris.Wrap(err, "failed to replace existing local file")
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return eris.Wrap(err, "failed to move completed download into place")
	}

	// Restore strict error check for Chtimes as per legacy code
	if err := os.Chtimes(outputPath, time.Now(), modTime); err != nil {
		return eris.Wrap(err, "failed to set modified time")
	}

	PrintInfoF("  [Download %s] Downloaded: %s", progressLabel, outputPath)
	return nil
}
