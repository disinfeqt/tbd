# Track Plan: Userscript UI Indicator

## Goal
Add a visual indicator (HUD or overlay) to the Twitter webpage via the Userscript. This indicator should show the current sync status (e.g., "Connecting...", "Saved: 5", "Limit Reached") so the user doesn't have to check the browser console.

## Context
Currently, the userscript only logs status to the console. This is inconvenient for regular usage. A small, non-intrusive UI element on the page would greatly improve the user experience.

## Status
- **Status**: ⚪️ Backlog (Planned)
- **Current Step**: Awaiting prioritization

## Tasks
- [ ] **Design**
	- [ ] Decide on position (e.g., bottom-left corner).
	- [ ] Decide on style (simple text, toast notification, or persistent status bar).
- [ ] **Implementation**
	- [ ] Inject HTML/CSS into the page.
	- [ ] Connect `GM_xmlhttpRequest` callbacks to the UI update logic.
	- [ ] Handle "Duplicate Limit Reached" state visually (e.g., turn red).
