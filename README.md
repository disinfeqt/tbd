# TBD: The Twitter Archival System

**English** | [中文文档](./README_zh.md)

A robust, self-hosted system to sync, archive, and explore your Twitter/X bookmarks locally.

Originally started as a **Twitter Bookmarks Downloader**, **TBD** has evolved into your personal **Twitter Backup Daemon**, acting as the ultimate **Twitter Bookmarks Depot** for your local media collection.

## Why TBD?

Most tools rely on expensive APIs or fragile scraping. **TBD** takes a different approach: **Passive Interception**.

It runs a local server and uses a browser userscript to intercept the _exact same data_ your browser receives from Twitter.

- **No API Keys Required**: If you can see it, TBD can save it.
- **True Sync**: It's not a one-off export; it's a persistent, deduplicated local library.
- **Media-First**: Focuses on preserving highest-quality images and videos before they are deleted or the user is suspended.
- **Privacy-Centric**: Your data stays on your machine in a local SQLite database.

## Features

- **🔄 Auto-Sync**: One-click auto-scroll to fetch your entire bookmark history.
- **📹 Media Daemon**: Background worker automatically downloads highest-quality images and videos with **smart skipping** of existing files.
- **🕰️ Timeline Fidelity**: Sets the file modification time to the **original tweet publication date**, keeping your local collection chronologically sorted.
- **🗄️ SQLite Database**: Deduplicates tweets and stores metadata efficiently.

* **🧠 Smart & Force Modes**: Choose between quick incremental syncs or deep historical recovery.
* **🔧 Resilient**: Multi-layer parsing (Struct + Regex Fallback) ensures it keeps working even when Twitter's API shifts.

## 🚀 Status & Roadmap

- [x] Sync bookmarks (Incremental & Force modes)
- [x] Auto-download bookmark media (Images & Videos)
- [ ] Local bookmark gallery/browser UI

## Architecture

1.  **Frontend (Userscript)**: Hooks into `XMLHttpRequest` on `x.com` to capture data silently.
2.  **Backend (Go)**: A lightweight daemon (`:41008`) that parses data, manages the SQLite database, and handles heavy-duty media downloads.

## Getting Started

### 1. Backend

Ensure you have [Go](https://go.dev/dl/) installed.

```bash
git clone https://github.com/rainux/twitter-bookmarks-downloader.git tbd
cd tbd
go build -o tbd .
./tbd
```

### 2. Frontend

1.  Install **Tampermonkey**.
2.  Create a new script using the content of `sync-bookmarks.user.js`.
3.  Open your **[Twitter Bookmarks](https://x.com/i/bookmarks)** and use the TBD control panel in the bottom-left corner.

---

## Disclaimer

This tool is for personal archiving only. Please respect content creators' copyrights and Twitter's TOS.
