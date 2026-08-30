// Package tbd embeds the files that live outside internal/ so a built binary
// can serve them from any working directory.
package tbd

import (
	_ "embed"
	"regexp"
)

// Userscript is the Tampermonkey script the explorer offers for install.
//
//go:embed web/tbd.user.js
var Userscript string

var userscriptVersionRe = regexp.MustCompile(`(?m)^//\s*@version\s+(\S+)`)

// UserscriptVersion reads the version out of the embedded script, so the app
// and the script it ships can never disagree about which one is current.
func UserscriptVersion() string {
	match := userscriptVersionRe.FindStringSubmatch(Userscript)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
