// Package download runs the background media download worker.
package download

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
	"sync"
	"time"

	"github.com/rotisserie/eris"
	"gorm.io/gorm"

	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/store"
	"twitter-bookmarks-downloader/internal/twitter"
)

// nudge lets the sync handler wake the worker as soon as new media arrives
// instead of waiting for the next poll tick.
var nudge = make(chan struct{}, 1)

func Nudge() {
	select {
	case nudge <- struct{}{}:
	default: // A wake-up is already queued.
	}
}

func StartWorker() {
	logx.Info("Download worker started")

	// Initial report on startup (force display)
	ReportStatus(true)

	ticker := time.NewTicker(5 * time.Second)
	statusTicker := time.NewTicker(10 * time.Minute)

	for {
		select {
		case <-ticker.C:
			ProcessQueue()
		case <-nudge:
			ProcessQueue()
		case <-statusTicker.C:
			ReportStatus(false)
		}
	}
}

func ReportStatus(force bool) {
	var pending int64
	var paused int64
	var failed int64
	if err := pendingQuery().Count(&pending).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count pending media"))
	}
	if err := store.DB.Model(&store.MediaModel{}).Where("downloaded = ? AND failed = ?", false, false).Count(&paused).Error; err == nil {
		paused -= pending
	} else {
		logx.Error(eris.Wrap(err, "Failed to count paused media"))
	}
	if err := store.DB.Model(&store.MediaModel{}).Where("failed = ?", true).Count(&failed).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count failed media"))
	}

	if force || pending > 0 || paused > 0 || failed > 0 {
		logx.Infof("Downloads — waiting: %d · paused by settings: %d · failed: %d", pending, paused, failed)
		if failed > 0 {
			logx.Warnf("%d media items gave up after multiple retries", failed)
		}
	}
}

func ShouldDownloadURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	current := config.Current()
	if twitter.IsMP4MediaURL(rawURL) {
		return current.DownloadVideos
	}

	return current.DownloadImages
}

func ShouldDownload(media store.MediaModel) bool {
	current := config.Current()

	switch media.Type {
	case "photo":
		return current.DownloadImages
	case "video", "animated_gif":
		return current.DownloadVideos
	default:
		return ShouldDownloadURL(media.URL)
	}
}

func pendingQuery() *gorm.DB {
	query := store.DB.Model(&store.MediaModel{}).Where("downloaded = ? AND failed = ?", false, false)
	current := config.Current()

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

// maxConcurrentDownloads keeps small images flowing while a large video downloads,
// without saturating the connection.
const maxConcurrentDownloads = 3

func ProcessQueue() {
	// Fetch the batch first so the idle path (no pending media) costs one query.
	var mediaList []store.MediaModel
	// Find up to 5 pending downloads
	if err := pendingQuery().Limit(5).Find(&mediaList).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to query pending media"))
		return
	}
	if len(mediaList) == 0 {
		return
	}

	var total int64
	var pending int64
	if err := store.DB.Model(&store.MediaModel{}).Count(&total).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count total media"))
		return
	}
	if err := pendingQuery().Count(&pending).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count pending media"))
		return
	}

	// One media-count query per tweet, shared by every item in the batch.
	mediaCounts := make(map[string]int64, len(mediaList))
	for _, media := range mediaList {
		if _, ok := mediaCounts[media.TweetID]; ok {
			continue
		}
		var count int64
		if err := store.DB.Model(&store.MediaModel{}).Where("tweet_id = ?", media.TweetID).Count(&count).Error; err != nil {
			logx.Error(eris.Wrap(err, "Failed to count tweet media"))
			continue
		}
		mediaCounts[media.TweetID] = count
	}

	completed := total - pending
	sem := make(chan struct{}, maxConcurrentDownloads)
	var wg sync.WaitGroup

	for i := range mediaList {
		media := &mediaList[i]
		progressLabel := fmt.Sprintf("%d/%d", completed+int64(i)+1, total)
		if !ShouldDownload(*media) {
			continue
		}
		mediaCount, ok := mediaCounts[media.TweetID]
		if !ok {
			continue // Count query failed above; the item stays pending for the next tick.
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(media *store.MediaModel, progressLabel string, mediaCount int) {
			defer wg.Done()
			defer func() { <-sem }()

			err := processMediaDownload(media, progressLabel, mediaCount)
			if err != nil {
				logx.Error(eris.Wrapf(err, "Media ID: %s", media.ID))
				media.RetryCount++
				if media.RetryCount >= 3 {
					media.Failed = true
				}
			} else {
				media.Downloaded = true
			}
			if err := store.DB.Save(media).Error; err != nil {
				logx.Error(eris.Wrapf(err, "Failed to update media status for %s", media.ID))
			}
		}(media, progressLabel, int(mediaCount))
	}
	wg.Wait()
}

func processMediaDownload(media *store.MediaModel, progressLabel string, mediaCount int) error {
	// Select only the columns BuildFilename and logging need; RawJSON holds the
	// full GraphQL payload and would dominate the read otherwise.
	var tweet store.TweetModel
	if err := store.DB.Select("id", "screen_name", "created_at", "permanent_url").
		First(&tweet, "id = ?", media.TweetID).Error; err != nil {
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

	filename := BuildFilename(&tweet, media.Index, mediaCount, parsedURL)
	outputPath := filepath.Join(config.Current().MediaDir, filename)

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
func BuildFilename(tweet *store.TweetModel, index int, total int, url *url.URL) string {
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
			logx.Infof("  [Download %s] Downloading: %s (%s/%s)", pw.label, pw.outputPath, formatByteSize(pw.written), formatByteSize(pw.total))
		} else {
			logx.Infof("  [Download %s] Downloading: %s (%s)", pw.label, pw.outputPath, formatByteSize(pw.written))
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
			logx.Warn("Failed to check remote file size, force downloading")
		} else {
			if resp.ContentLength > 0 {
				if fileInfo.Size() == resp.ContentLength {
					if err := resp.Body.Close(); err != nil {
						logx.Warnf("Failed to close HEAD response body: %v", err)
					}
					logx.Infof("  [Download %s] Skipped: %s", progressLabel, outputPath)
					return nil
				}
			}
			if err := resp.Body.Close(); err != nil {
				logx.Warnf("Failed to close HEAD response body: %v", err)
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
			logx.Warnf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return eris.Errorf("failed to download file, status code: %d", resp.StatusCode)
	}

	if resp.ContentLength > 0 {
		logx.Infof("  [Download %s] Starting: %s (%s)", progressLabel, outputPath, formatByteSize(resp.ContentLength))
	} else {
		logx.Infof("  [Download %s] Starting: %s (unknown size)", progressLabel, outputPath)
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
		logx.Warn("Content-Length header not provided by the server.")
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

	logx.Infof("  [Download %s] Downloaded: %s", progressLabel, outputPath)
	return nil
}
