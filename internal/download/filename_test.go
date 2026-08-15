package download

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"twitter-bookmarks-downloader/internal/store"
)

func TestFileExt(t *testing.T) {
	const urlStr = "https://pbs.twimg.com/media/GoaCyBaXAAAjPQD.JPG?name=orig"
	parsedURL, _ := url.Parse(urlStr)

	actual := strings.ToLower(path.Ext(parsedURL.Path))
	assert.Equal(t, ".jpg", actual)
}

func TestBuildFilename(t *testing.T) {
	// Use a fixed UTC time for consistency
	createdAtUTC := time.Date(2025, 2, 28, 8, 54, 5, 0, time.UTC)

	tweet := &store.TweetModel{
		ID:         "1895276249498689869",
		ScreenName: "nekoplanetOuO",
		CreatedAt:  createdAtUTC,
	}

	// Calculate expected timestamp string based on local machine's timezone
	// because BuildFilename uses time.Local
	expectedTimeStr := createdAtUTC.In(time.Local).Format("20060102-150405")

	// Test Case 1: Single Image
	url1, _ := url.Parse("https://pbs.twimg.com/media/test.jpg")
	filename1 := BuildFilename(tweet, 0, 1, url1)
	expected1 := fmt.Sprintf("twitter-@nekoplanetOuO-%s-1895276249498689869.jpg", expectedTimeStr)
	assert.Equal(t, expected1, filename1)

	// Test Case 2: Multiple Images (Index 0)
	filename2 := BuildFilename(tweet, 0, 4, url1)
	expected2 := fmt.Sprintf("twitter-@nekoplanetOuO-%s-1895276249498689869-0.jpg", expectedTimeStr)
	assert.Equal(t, expected2, filename2)

	// Test Case 3: Multiple Images (Index 3)
	filename3 := BuildFilename(tweet, 3, 4, url1)
	expected3 := fmt.Sprintf("twitter-@nekoplanetOuO-%s-1895276249498689869-3.jpg", expectedTimeStr)
	assert.Equal(t, expected3, filename3)
}

func TestBuildFilenameVideo(t *testing.T) {
	createdAtUTC := time.Date(2025, 4, 14, 20, 29, 49, 0, time.UTC)
	tweet := &store.TweetModel{
		ID:         "1911758791701332134",
		ScreenName: "YamoakaMei",
		CreatedAt:  createdAtUTC,
	}

	expectedTimeStr := createdAtUTC.In(time.Local).Format("20060102-150405")

	// Test Case: Video
	urlVid, _ := url.Parse("https://video.twimg.com/ext_tw_video/123/pu/vid/720x1280/test.mp4")
	filename := BuildFilename(tweet, 0, 1, urlVid)
	expected := fmt.Sprintf("twitter-@YamoakaMei-%s-1911758791701332134.mp4", expectedTimeStr)
	assert.Equal(t, expected, filename)
}
