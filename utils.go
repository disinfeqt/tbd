package main

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/rotisserie/eris"
)

var (
	errorColor   = color.New(color.FgRed)
	warningColor = color.New(color.FgYellow)
	infoColor    = color.New(color.FgGreen)
	fatalColor   = color.New(color.FgRed, color.Bold)

	logFile *os.File
)

func InitLogger(logPath string) error {
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	logFile = file
	return nil
}

func CloseLogger() {
	if logFile != nil {
		err := logFile.Close()
		if err != nil {
			FatalError(err)
		}
	}
}

func logToFile(level string, message string) {
	if logFile != nil {
		_, err := fmt.Fprintln(logFile, level, message)
		if err != nil {
			FatalError(err)
		}
	}
}

func PrintError(err error) {
	errMsg := eris.ToString(err, !isReleaseBuild)
	fmt.Fprintln(os.Stderr, errorColor.Sprint("[ERROR]"), errMsg)
	logToFile("[ERROR]", errMsg)
}

func PrintWarning(message ...interface{}) {
	msg := fmt.Sprint(message...)
	fmt.Println(warningColor.Sprint("[WARN]"), msg)
	logToFile("[WARN]", msg)
}

func PrintInfo(message ...interface{}) {
	msg := fmt.Sprint(message...)
	fmt.Println(infoColor.Sprint("[INFO]"), msg)
	logToFile("[INFO]", msg)
}

func PrintInfoF(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Print(infoColor.Sprint("[INFO]"), msg)
	logToFile("[INFO]", msg)
}

func FatalError(err error) {
	errMsg := eris.ToString(err, !isReleaseBuild)
	fmt.Fprintln(os.Stderr, fatalColor.Sprint("[ERROR]"), errMsg)
	logToFile("[ERROR]", errMsg)
	os.Exit(1)
}

func CloseResource(closer io.Closer) {
	err := closer.Close()
	if err != nil {
		FatalError(err)
	}
}
