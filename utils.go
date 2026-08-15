package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/rotisserie/eris"
)

var (
	errorMark   = color.New(color.FgRed, color.Bold)
	warningMark = color.New(color.FgYellow, color.Bold)
	infoMark    = color.New(color.FgGreen, color.Bold)
	timeStyle   = color.New(color.Faint)

	logFile   *os.File
	logFileMu sync.Mutex
)

// Rotate the log once it grows past 5 MB, keeping one previous generation.
const maxLogFileSize = 5 << 20

func InitLogger(logPath string) error {
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogFileSize {
		// Best effort — never block startup on rotation.
		_ = os.Rename(logPath, logPath+".old")
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	logFile = file
	return nil
}

func CloseLogger() {
	logFileMu.Lock()
	defer logFileMu.Unlock()

	if logFile == nil {
		return
	}
	if err := logFile.Close(); err != nil {
		fmt.Fprintln(os.Stderr, errorMark.Sprint("✗"), "Failed to close log file:", err)
	}
	logFile = nil
}

func logToFile(level string, message string) {
	logFileMu.Lock()
	defer logFileMu.Unlock()

	if logFile == nil {
		return
	}
	line := time.Now().Format("2006-01-02 15:04:05") + " " + level + " " + message
	if _, err := fmt.Fprintln(logFile, line); err != nil {
		// Disable file logging rather than crashing on a bad disk.
		logFile = nil
		fmt.Fprintln(os.Stderr, errorMark.Sprint("✗"), "Log file write failed, file logging disabled:", err)
	}
}

func printLine(w io.Writer, mark string, message string) {
	fmt.Fprintln(w, timeStyle.Sprint(time.Now().Format("15:04:05")), mark, message)
}

func PrintError(err error) {
	errMsg := eris.ToString(err, !isReleaseBuild)
	printLine(os.Stderr, errorMark.Sprint("✗"), errMsg)
	logToFile("[ERROR]", errMsg)
}

func PrintWarning(message ...any) {
	msg := fmt.Sprint(message...)
	printLine(os.Stdout, warningMark.Sprint("!"), msg)
	logToFile("[WARN]", msg)
}

func PrintWarningF(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	printLine(os.Stdout, warningMark.Sprint("!"), msg)
	logToFile("[WARN]", msg)
}

func PrintInfo(message ...any) {
	msg := fmt.Sprint(message...)
	printLine(os.Stdout, infoMark.Sprint("•"), msg)
	logToFile("[INFO]", msg)
}

func PrintInfoF(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	printLine(os.Stdout, infoMark.Sprint("•"), msg)
	logToFile("[INFO]", msg)
}

func FatalError(err error) {
	errMsg := eris.ToString(err, !isReleaseBuild)
	printLine(os.Stderr, errorMark.Sprint("✗"), errMsg)
	logToFile("[ERROR]", errMsg)
	os.Exit(1)
}

func CloseResource(closer io.Closer) {
	err := closer.Close()
	if err != nil {
		FatalError(err)
	}
}
