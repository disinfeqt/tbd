package main

import (
	"fmt"
	"os"
)

func Fatal(v ...any) {
	_, err := fmt.Fprintln(os.Stderr, v...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error writing to stderr:", err)
	}
	os.Exit(1)
}
