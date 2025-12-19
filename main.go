package main

import (
	"flag"

	"github.com/rotisserie/eris"
)

func main() {
	importLegacy := flag.Bool("import-legacy", false, "Import legacy JSON files from tweets/ directory")
	flag.Parse()

	// 1. Initialize Logger
	err := InitLogger("logs")
	if err != nil {
		panic(err)
	}
	defer CloseLogger()

	// 2. Initialize Database
	err = InitDB("bookmarks.db")
	if err != nil {
		FatalError(eris.Wrap(err, "Failed to init DB"))
	}
	PrintInfo("Database initialized successfully")

	// Special Mode: Import Legacy Data
	if *importLegacy {
		if err := ImportLegacyData("tweets"); err != nil {
			FatalError(err)
		}
		return // Exit after import
	}

	// 3. Start Background Download Worker
	go StartDownloadWorker()

	// 4. Start HTTP Server
	if err := StartServer(":41008"); err != nil {
		FatalError(eris.Wrap(err, "Server failed"))
	}
}
