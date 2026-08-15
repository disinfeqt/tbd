// Package logx provides colored terminal output.
package logx

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/rotisserie/eris"
)

var (
	errorMark   = color.New(color.FgRed, color.Bold)
	warningMark = color.New(color.FgYellow, color.Bold)
	infoMark    = color.New(color.FgGreen, color.Bold)
	timeStyle   = color.New(color.Faint)

	isReleaseBuild bool
)

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

func printLine(w io.Writer, mark string, message string) {
	fmt.Fprintln(w, timeStyle.Sprint(time.Now().Format("15:04:05")), mark, message)
}

func Error(err error) {
	printLine(os.Stderr, errorMark.Sprint("✗"), eris.ToString(err, !isReleaseBuild))
}

func Warn(message ...any) {
	printLine(os.Stdout, warningMark.Sprint("!"), fmt.Sprint(message...))
}

func Warnf(format string, args ...any) {
	printLine(os.Stdout, warningMark.Sprint("!"), fmt.Sprintf(format, args...))
}

func Info(message ...any) {
	printLine(os.Stdout, infoMark.Sprint("•"), fmt.Sprint(message...))
}

func Infof(format string, args ...any) {
	printLine(os.Stdout, infoMark.Sprint("•"), fmt.Sprintf(format, args...))
}

func Fatal(err error) {
	printLine(os.Stderr, errorMark.Sprint("✗"), eris.ToString(err, !isReleaseBuild))
	os.Exit(1)
}
