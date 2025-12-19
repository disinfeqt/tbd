# Track Plan: Legacy Data Import

## Goal
Implement a reliable tool to scan existing `.json` bookmark files in the `tweets/` directory and import them into the SQLite database.

## Context
Before the server-based architecture, bookmarks were saved as individual JSON files. Since Twitter's web interface doesn't always show very old bookmarks, these local files are the only source for that data. Importing them into DB ensures unified management, deduplication, and media download tracking.

## Status
- **Status**: 🟢 Completed
- **Current Step**: Done

## Tasks
- [x] **Research & Design**
	- [x] Map legacy JSON fields to `TweetModel`.
	- [x] Determine how to handle media records if the files already exist in `media/`.
- [x] **Implementation**
	- [x] Create an importer module.
	- [x] Implement file scanning and parsing.
	- [x] Implement DB insertion with conflict resolution (skip existing).
- [x] **CLI Integration**
	- [x] Add `--import-legacy` flag to `main.go`.
- [x] **Verification**
	- [x] Run import on a sample of legacy files.
	- [x] Verify DB records and media download status.

## Notes
- Legacy JSONs usually use the `twitterscraper.Tweet` structure.
- We must populate `RawJSON` with the full content of the file to maintain consistency with new data.
