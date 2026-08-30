# TBD

**English** | [中文](./README.zh-CN.md)

Sync your X/Twitter bookmarks to your own machine and download the media files in them.

> **Upgrading from an older version?**
>
> - Delete the old Tampermonkey script "Twitter Bookmarks Sync to Local". The script was renamed, so Tampermonkey keeps both and runs them side by side; the service ignores the old script's duplicate batches and warns in the log, but removing it is the clean fix.
> - If you use several X accounts in this browser, do the first sync after upgrading with the account that owns your existing bookmarks — the first account to sync claims them.

## How it works

TBD has two parts:

| Component                   | What it does                                                                                                        |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Local Go service (`./tbd`)  | Saves bookmarks to `bookmarks.db`, downloads media into each account's own folder, and serves the browse/stats/settings dashboard |
| Tampermonkey userscript     | Captures the bookmark data the browser receives on `x.com/i/history` and sends it to the local service              |

## Quick start

### 1. Build and start the local service

Install [Go](https://go.dev/dl/), then run in the project directory:

```bash
go build -o tbd ./cmd/tbd
./tbd
```

The service listens on `http://localhost:41008`. Keep this terminal window running while you use it.

Only this machine can reach it. To open the dashboard from a phone or another computer on the same network, start it with `./tbd --listen :41008` — the startup line then prints the LAN address to visit. TBD has no password, so anyone who can reach that address can read and delete the archive; use it only on a network you trust. The userscript still syncs from a browser on this machine.

### 2. Install the Tampermonkey script

Open <http://localhost:41008>. On a first run (before any bookmarks exist) it lands directly on the **Set up** guide — just follow its three steps:

1. Install the [Tampermonkey](https://chromewebstore.google.com/detail/tampermonkey/dhdgffkkebhmkfjojejmpbldmpobfkfo) browser extension
2. Enable **Allow User Scripts** in Tampermonkey's settings (required — nothing runs without it)
3. Click **Install the script** in the guide — it points at <http://localhost:41008/tbd.user.js>, served by the local app, and Tampermonkey opens its install prompt

The guide shows live whether the script has connected and whether it is up to date; when your installed copy is older than the one the app ships, it prompts you to reinstall (the same link updates it). You can also create the script manually by copying the contents of [web/tbd.user.js](./web/tbd.user.js).

### 3. Start syncing

With `./tbd` running, open:

```text
https://x.com/i/history
```

The TBD panel appears in the bottom-left corner and **syncing starts on its own** — no button press needed:

| Panel item                 | Meaning                                                                                                                     |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `N new · M already saved`  | N bookmarks saved during this visit, with M more already in the local archive                                                |
| `Start sync` / `Stop sync` | Stop any time. After a few batches with nothing new it stops on its own and shows `All caught up`; press again to keep scanning older bookmarks |
| `Download videos` switch   | Whether to download videos                                                                                                   |
| `Download images` switch   | Whether to download images                                                                                                   |

Sync results are saved to:

- Database: `bookmarks.db`
- Media files: that account's media folder (default `media/`)

## Multiple accounts

One TBD can archive several X accounts without them interfering:

- The script detects which X account is signed in (from the `twid` cookie) and tags every batch with it — no manual switching
- Each account keeps its own bookmark list, media folder, and download switches
- An account dropdown appears in the dashboard's top-left corner (once there are two or more accounts); switching it carries the stats, filters, authors, and missing-files views along
- When two accounts bookmark the same tweet, the tweet is stored once and its files are downloaded once, but both accounts see it; removing it from one account leaves the other untouched
- After upgrading to the multi-account version, your existing bookmarks are parked under a placeholder account and **claimed automatically by your real account on the next sync** — the media folder stays the same and no files are moved

## Browse dashboard

While the service runs, open <http://localhost:41008> to browse the whole archive:

- **Stats**: one line at the top shows total bookmarks, media download progress, author count, and the time span of your bookmarking; click through to the matching subpages (media types, top authors, monthly timeline, live activity log)
- **Filter & sort**: full-text/author search; type tabs — All / Photos (photo-only tweets, mixed photo+video excluded) / Videos / GIFs / Text / Missing files; sort by date added, tweet date, or video length
- **Grid**: masonry cards with hover text preview; video cards carry a duration badge
- **Lightbox**: multi-media tweets browse as a carousel (thumbnail strip + arrow keys); jump to the original tweet, reveal the file in Finder, or remove the bookmark (optionally deleting its downloaded files)
- **Download progress**: a live progress strip appears at the top of the page while media is downloading
- **Settings**: open `Settings` from the top to change each account's media folder and video/image switches, plus the "defaults for new accounts"; `Set up` is the userscript install and connection status page

## Command-line tools

Besides the default service mode, `./tbd` supports these one-shot commands:

| Command                     | What it does                                                                                                                                 |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `./tbd --export-handles`    | Export the unique author handles from saved bookmarks to `handles.json` (choose the output file with `--handles-output <path>`)               |
| `./tbd --repair-media`      | Repair video media URLs from the raw tweet JSON stored in the database                                                                        |
| `./tbd --fix-deleted-media` | Treat the `media/` folder as the source of truth and remove bookmark records whose media files were all deleted by hand                       |
| `./tbd --reset`             | After confirmation, delete `bookmarks.db` (including WAL files). Downloaded media in `media/` is **never touched**. Stop the running service first |

## Settings

Per-account settings live in the database; the dashboard's `Settings` page is the easiest place to change them. The two switches on the userscript panel also edit the current X account's settings.

`config.json` holds the **defaults for new accounts** (and the fallback while no account exists yet), generated on the first run:

```jsonc
{
  // Media folder: a relative path resolves against the directory ./tbd runs
  // in; any folder outside the project works too, including external drives,
  // e.g. "/Volumes/Archive/x-media";
  "media_dir": "media",
  "download_videos": true,
  "download_images": true,
}
```

## FAQ

**Installed the script but the dashboard says it hasn't connected?**

The script only contacts the local service when an x.com page loads, so open x.com once right after installing. The `Set up` page refreshes its status every few seconds and shows a green dot once connected.

**No panel in the bottom-left corner?**

Check in order: the Tampermonkey extension is enabled, the script is enabled, **Allow User Scripts** is on in Tampermonkey's settings, and the page is `https://x.com/i/history` (the old address `https://x.com/i/bookmarks` works too).

**A bookmark you just added isn't syncing?**

New bookmarks only appear at the top of the timeline, and X reuses a cached timeline on in-app navigation without making any request. When the script sees no bookmark request within a few seconds, it reloads the page once on its own before syncing — so normally no manual reload is needed.

**Downloads pending but no new files appearing yet?**

Large videos take time. The service logs download starts and progress in the terminal; if those logs are absent, restart `./tbd`.

**Updated the script but nothing changed?**

Reopen <http://localhost:41008/tbd.user.js> and let Tampermonkey reinstall over it (or re-copy the latest contents of [web/tbd.user.js](./web/tbd.user.js) and save). Also make sure the old script "Twitter Bookmarks Sync to Local" is deleted — it is a separate entry and runs alongside the new one.

## License

MIT. For personal archiving only — you are responsible for complying with X/Twitter's terms of service and content copyright.

## Thanks

TBD grew out of [0x1b2c/twitter-bookmarks-downloader](https://github.com/0x1b2c/twitter-bookmarks-downloader) — thanks to the original author for the foundation this project is built on.
