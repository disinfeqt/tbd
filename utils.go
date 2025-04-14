package main

import (
	"fmt"
	"io"
	"os"

	"github.com/rotisserie/eris"
)

func PrintError(err error) {
	fmt.Println(eris.ToString(err, !isReleaseBuild))
}

func FatalError(err error) {
	fmt.Fprintln(os.Stderr, eris.ToString(err, !isReleaseBuild))
	os.Exit(1)
}

func CloseResource(closer io.Closer) {
	err := closer.Close()
	if err != nil {
		FatalError(err)
	}
}
