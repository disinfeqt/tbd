# Track Plan: Userscript UI Indicator

## Goal
Add a visual indicator (HUD) and an Auto-Scroll feature to the Twitter webpage via the Userscript. 
- **HUD**: Show sync status (Saved count, Limit reached).
- **Auto-Scroll**: A toggle button to automatically scroll down the page to fetch bookmarks history, stopping when the duplicate limit is reached.

## Context
Manual scrolling is tedious for large bookmark collections. Automation is required.

## Status
- **Status**: 🟢 Completed
- **Current Step**: Done

## Tasks
- [x] **Design**
	- [x] HUD Position: Fixed bottom-left (120px up).
	- [x] UI Elements: Status Text + Play/Stop Icon + Force Toggle.
- [x] **Implementation**
	- [x] Inject minimal CSS/HTML overlay.
	- [x] Implement `AutoScroll` class with Force Mode.
	- [x] Connect Backend Response to Logic.
- [x] **Verification**
	- [x] Test auto-scroll on Twitter's virtual list.
	- [x] Ensure it stops correctly on limit (Smart Mode) or continues (Force Mode).
