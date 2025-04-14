package main

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

	twitterscraper "github.com/imperatrona/twitter-scraper"
	"github.com/rotisserie/eris"
)

const MAX_BOOKMARKS_COUNT = 5

var isReleaseBuild bool

func init() {
	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range bi.Settings {
			if setting.Key == "-tags" && strings.Contains(setting.Value, "release") {
				isReleaseBuild = true
				break
			}
		}
	}
}

func main() {
	cookies, err := parseCookie()
	if err != nil {
		err = eris.Wrap(err, "Error parsing cookies")
		Fatal(eris.ToString(err, !isReleaseBuild))
		return
	}

	scraper := twitterscraper.New()
	scraper.SetCookies(cookies)

	// Call IsLoggedIn method to perform authentication
	if !scraper.IsLoggedIn() {
		Fatal("Invalid cookies")
	}

	for tweet := range scraper.GetBookmarks(context.Background(), MAX_BOOKMARKS_COUNT) {
		if tweet.Error != nil {
			err := eris.Wrap(tweet.Error, "Error getting bookmark")
			fmt.Println(eris.ToString(err, !isReleaseBuild))
			continue // Continue process next bookmark
		}
		fmt.Printf("%s: %s\n", tweet.Username, tweet.Text)
	}
}
