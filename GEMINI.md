# TBD (Twitter Bookmarks Depot) - Developer Guide

## 1. Architecture Overview

This project (TBD) uses a hybrid architecture to safely and reliably sync Twitter bookmarks.

- **Frontend (Userscript)**: A Tampermonkey script running on `https://x.com`. It acts as a **passive interceptor**.
- **Backend (Go)**: A local HTTP server (`:41008`) backed by SQLite. It handles data parsing, storage, and media downloading.

### Data Flow

1.  User scrolls the Twitter Bookmarks page.
2.  **Userscript** intercepts the native `XMLHttpRequest`.
3.  Userscript detects GraphQL responses containing "Bookmarks".
4.  Userscript forwards the **raw response text** (string) to `http://localhost:41008/api/sync-raw` using `GM_xmlhttpRequest` (to bypass CSP).
5.  **Go Server** parses the complex GraphQL JSON.
6.  **Go Server** saves metadata to SQLite (`bookmarks.db`) and checks for duplicates.
7.  **Background Worker** scans the DB for undownloaded media and downloads them to `media/`.

## 2. Key Technical Decisions & "Gotchas"

### Frontend (Userscript)

- **Why `GM_xmlhttpRequest`?** Twitter's Content Security Policy (CSP) blocks XHR/Fetch to `localhost`. `GM_xmlhttpRequest` operates in the extension context, bypassing this restriction.
- **Why Raw Text Forwarding?**
  - **Memory/Sandbox Limits**: Twitter's GraphQL responses can be huge. Parsing them in the browser or passing large objects across the Tampermonkey sandbox boundary often causes `RangeError: Invalid array length` or memory crashes.
  - **Stability**: Sending the raw string shifts the heavy lifting to Go, which handles large memory allocations much better.
- **Hooking Strategy**: We hook `XMLHttpRequest.prototype.send` and listen to the `load` event. This is more reliable than hooking `fetch` for capturing the complete response stream without stream locking issues.

### Backend (Go)

- **Deep Parsing (`sync.go`)**: Twitter's JSON structure is deeply nested and polymorphic.
  - User info path: `core.user_results.result.legacy` OR `core.user_results.result.core`.
  - The `result` field often contains a `__typename` wrapper.
  - **Strategy**: We use a comprehensive struct matching the observed `@d.json` schema.
- **Regex Fallback**: Parsing structure often fails due to API changes or suspended users. We use a Regex (`"screen_name"\s*:\s*"([^"]+)"`) as a last line of defense to extract the username. **This is critical for correct file naming.**
*   **RawJSON Preservation**: We store the complete, original JSON payload in the `raw_json` column of the `tweets` table. This serves as the **single source of truth** for future data recovery and re-parsing.
    *   **Heterogeneity Note**: After the legacy data import, `raw_json` may contain two distinct formats: the official Twitter GraphQL response (new data) and the `twitter-scraper` format (legacy data).
    *   **Parsing Policy**: Business logic (downloader, worker) must rely on **normalized fields** (e.g., `TweetModel.ID`, `MediaModel.URL`) rather than `RawJSON`. If re-parsing `RawJSON` is needed, use structural detection (e.g., checking for the presence of `"OrderedMedia"`) to determine the format.
- **Deduplication**: Sync stops (sets `duplicate_limit_reached`) if 5 consecutive existing tweets are encountered.

### File Naming Convention

Strictly adhere to the legacy format to avoid re-downloading existing libraries:
`twitter-@<ScreenName>-<YYYYMMDD>-<HHMMSS>-<TweetID>[-<Index>].<Ext>`

## 3. Configuration

- **Port**: `41008` (Hardcoded in `main.go` and Userscript).
- **Database**: `bookmarks.db` (SQLite).
- **Storage Migration**: Historically, bookmarks were saved as individual JSON files in `tweets/`. This feature has been **deprecated** in favor of full DB persistence (storing the raw payload in the `raw_json` column).
- **Directories**: `media/` (images/videos). The `tweets/` directory is no longer used for new bookmarks.

## 4. Development Workflow

### Task Management
*   **Conductor**: We use the Conductor extension for project planning and track management. 
    *   `conductor/tracks.md`: Index of active development tracks.
    *   `conductor/tracks/<track-id>/plan.md`: Detailed plan and status for a specific task.


### Rules

- **No Proactive Commits**: Do not commit code unless explicitly instructed.
- **Format**: Run `go fmt ./...` before every commit.
- **Commit Messages**: Simple, descriptive, non-conventional. Always include `Co-authored-by: Gemini <gemini@google.com>`.

### Testing Strategy

- **Unit Tests (`download_test.go`)**: Focus on pure logic (e.g., filename generation) using mocked data. Avoid external network dependencies.
- **Regression Tests**: For complex parsing scenarios (e.g., Mixed Media), use saved `RawJSON` snapshots from the database as test cases.

### Common Commands
*   **Run**: `go run .`
*   **Build**: `go build .`
*   **Import Legacy Data**: `./twitter-bookmarks-downloader --import-legacy` (scans `tweets/` directory)
*   **Test Userscript**: Update the version in `sync-bookmarks.user.js` and reinstall in Tampermonkey.

## 5. Troubleshooting

- **Missing ScreenName (files named `twitter-@-...`)**: Means struct parsing failed AND regex fallback failed. Check `logs` for warnings. The raw JSON structure likely changed.
- **CORS/Network Errors**: Ensure the Userscript version matches the Server port, and `GM_xmlhttpRequest` permission is granted in Tampermonkey.
