// Package logx provides colored terminal output.
package logx

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/rotisserie/eris"
)

// Entry is a recent log line kept for the dashboard activity view.
type Entry struct {
	ID    int64     `json:"id"`
	Time  time.Time `json:"time"`
	Level string    `json:"level"` // info, warn, error
	Msg   string    `json:"msg"`
}

const recentCap = 500

var (
	recentMu sync.Mutex
	recent   []Entry
	lastID   int64
)

func record(level, msg string) {
	recentMu.Lock()
	defer recentMu.Unlock()
	lastID++
	recent = append(recent, Entry{ID: lastID, Time: time.Now(), Level: level, Msg: msg})
	if len(recent) > recentCap {
		recent = recent[len(recent)-recentCap:]
	}
}

// Recent returns buffered entries with an ID greater than afterID, oldest
// first. Pass 0 for everything currently buffered.
func Recent(afterID int64) []Entry {
	recentMu.Lock()
	defer recentMu.Unlock()
	start := len(recent)
	for i, entry := range recent {
		if entry.ID > afterID {
			start = i
			break
		}
	}
	out := make([]Entry, len(recent)-start)
	copy(out, recent[start:])
	return out
}

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
	record("error", eris.ToString(err, false))
}

func Warn(message ...any) {
	printLine(os.Stdout, warningMark.Sprint("!"), fmt.Sprint(message...))
	record("warn", fmt.Sprint(message...))
}

func Warnf(format string, args ...any) {
	printLine(os.Stdout, warningMark.Sprint("!"), fmt.Sprintf(format, args...))
	record("warn", fmt.Sprintf(format, args...))
}

func Info(message ...any) {
	printLine(os.Stdout, infoMark.Sprint("•"), fmt.Sprint(message...))
	record("info", fmt.Sprint(message...))
}

func Infof(format string, args ...any) {
	printLine(os.Stdout, infoMark.Sprint("•"), fmt.Sprintf(format, args...))
	record("info", fmt.Sprintf(format, args...))
}

func Fatal(err error) {
	printLine(os.Stderr, errorMark.Sprint("✗"), eris.ToString(err, !isReleaseBuild))
	os.Exit(1)
}
