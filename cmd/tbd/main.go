package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rotisserie/eris"

	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/download"
	"twitter-bookmarks-downloader/internal/export"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/server"
	"twitter-bookmarks-downloader/internal/store"
)

func main() {
	repairMedia := flag.Bool("repair-media", false, "Repair video media URLs from stored raw tweet JSON and exit")
	exportHandles := flag.Bool("export-handles", false, "Export unique @ handles to JSON and exit")
	handlesOutput := flag.String("handles-output", "handles.json", "Output path for --export-handles")
	reset := flag.Bool("reset", false, "Delete the bookmarks database and logs after confirmation (media files are never touched) and exit")
	flag.Parse()

	// Reset runs before the database is opened, so nothing holds the files
	// it is about to delete.
	if *reset {
		runReset()
		return
	}

	// 1. Load Settings
	if err := config.Load(config.DefaultPath); err != nil {
		logx.Fatal(eris.Wrap(err, "Failed to load config"))
	}

	// 2. Initialize Database
	if err := store.Init("bookmarks.db"); err != nil {
		logx.Fatal(eris.Wrap(err, "Failed to init DB"))
	}
	logx.Info("Database ready")

	if *exportHandles {
		count, err := export.UniqueHandles(*handlesOutput)
		if err != nil {
			logx.Fatal(err)
		}
		logx.Infof("Exported %d unique handles to %s", count, *handlesOutput)
		return
	}

	repaired, err := download.RepairVideoMediaURLs()
	if err != nil {
		logx.Fatal(eris.Wrap(err, "Failed to repair video media URLs"))
	}
	if repaired > 0 {
		logx.Infof("Repaired %d video media URLs; worker will download them as MP4 files.", repaired)
	}
	if *repairMedia {
		return
	}

	// 4. Start Background Download Worker
	go download.StartWorker()

	// 5. Start HTTP Server (localhost only — the dashboard exposes the whole archive)
	if err := server.Start("127.0.0.1:41008"); err != nil {
		logx.Fatal(eris.Wrap(err, "Server failed"))
	}
}

func runReset() {
	fmt.Println("This deletes bookmarks.db (your saved bookmark data).")
	fmt.Println("Downloaded media files in media/ are never touched.")
	fmt.Println("Make sure ./tbd is not running elsewhere before continuing.")
	fmt.Print(`Type "yes" to continue: `)

	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(line) != "yes" {
		fmt.Println("Reset cancelled — nothing was deleted.")
		return
	}

	if err := store.Reset("bookmarks.db"); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to delete database:", err)
		os.Exit(1)
	}
	// Sweep up log files left behind by older versions that wrote them.
	for _, path := range []string{"logs", "logs.old"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "Failed to delete", path+":", err)
			os.Exit(1)
		}
	}

	fmt.Println("Done — database deleted, media files kept. Run ./tbd to start fresh.")
}
