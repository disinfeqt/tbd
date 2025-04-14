package main

import (
	"net/url"
	"os"
	"path"
	"strings"
	"testing"

	twitterscraper "github.com/imperatrona/twitter-scraper"
	"github.com/stretchr/testify/assert"
)

var scraper *twitterscraper.Scraper

func TestMain(m *testing.M) {
	setupTestSuite()

	exitCode := m.Run()

	os.Exit(exitCode)
}

func setupTestSuite() {
	scraper = initScraper()
	config.MediaDir = "test_media"

	err := os.RemoveAll(config.MediaDir)
	if err != nil {
		FatalError(err)
	}
}

func TestFileExt(t *testing.T) {
	const urlStr = "https://pbs.twimg.com/media/GoaCyBaXAAAjPQD.JPG?name=orig"
	parsedURL, _ := url.Parse(urlStr)

	actual := strings.ToLower(path.Ext(parsedURL.Path))
	assert.Equal(t, ".jpg", actual)
}

func TestMultiplePhotos(t *testing.T) {
	const urlStr = "https://x.com/nekoplanetOuO/status/1895276249498689869"

	id := path.Base(urlStr)
	assert.Equal(t, "1895276249498689869", id)

	tweet, err := scraper.GetTweet(id)
	assert.Equal(t, nil, err)

	err = downloadPhotos(tweet, config)
	assert.Equal(t, nil, err)
	assert.FileExists(t, "test_media/twitter-@nekoplanetOuO-20250228-085405-1895276249498689869-0.jpg")
	assert.FileExists(t, "test_media/twitter-@nekoplanetOuO-20250228-085405-1895276249498689869-1.jpg")
	assert.FileExists(t, "test_media/twitter-@nekoplanetOuO-20250228-085405-1895276249498689869-2.jpg")
	assert.FileExists(t, "test_media/twitter-@nekoplanetOuO-20250228-085405-1895276249498689869-3.jpg")
}
