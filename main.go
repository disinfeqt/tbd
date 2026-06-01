package main

import (
	"flag"

	"github.com/rotisserie/eris"
)

func main() {
	importLegacy := flag.Bool("import-legacy", false, "Import legacy JSON files from tweets/ directory")
	repairMedia := flag.Bool("repair-media", false, "Repair video media URLs from stored raw tweet JSON and exit")
	exportHandles := flag.Bool("export-handles", false, "Export unique @ handles to JSON and exit")
	handlesOutput := flag.String("handles-output", "handles.json", "Output path for --export-handles")
	flag.Parse()

	// 1. Initialize Logger
	err := InitLogger("logs")
	if err != nil {
		panic(err)
	}
	defer CloseLogger()

	// 2. Load Settings
	if err := LoadConfig(configPath); err != nil {
		FatalError(eris.Wrap(err, "Failed to load config"))
	}

	// 3. Initialize Database
	err = InitDB("bookmarks.db")
	if err != nil {
		FatalError(eris.Wrap(err, "Failed to init DB"))
	}
	PrintInfo("Database initialized successfully")

	if *exportHandles {
		count, err := ExportUniqueHandles(*handlesOutput)
		if err != nil {
			FatalError(err)
		}
		PrintInfoF("Exported %d unique handles to %s", count, *handlesOutput)
		return
	}

	// Special Mode: Import Legacy Data
	if *importLegacy {
		if err := ImportLegacyData("tweets"); err != nil {
			FatalError(err)
		}
		return // Exit after import
	}

	repaired, err := RepairVideoMediaURLs()
	if err != nil {
		FatalError(eris.Wrap(err, "Failed to repair video media URLs"))
	}
	if repaired > 0 {
		PrintInfoF("Repaired %d video media URLs; worker will download them as MP4 files.", repaired)
	}
	if *repairMedia {
		return
	}

	// 4. Start Background Download Worker
	go StartDownloadWorker()

	// 5. Start HTTP Server
	if err := StartServer(":41008"); err != nil {
		FatalError(eris.Wrap(err, "Server failed"))
	}
}
