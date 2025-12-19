package main

import (
	"runtime/debug"
	"strings"
)

type Config struct {
	MediaDir string
}

var isReleaseBuild bool

var config Config = Config{
	MediaDir: "media",
}

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
