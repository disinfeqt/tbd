package main

import (
	"github.com/rotisserie/eris"
)

func main() {
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

	// 3. Start Background Download Worker
	go StartDownloadWorker()

	// 4. Start HTTP Server
	if err := StartServer(":41008"); err != nil {
		FatalError(eris.Wrap(err, "Server failed"))
	}
}
