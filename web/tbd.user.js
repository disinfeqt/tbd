// ==UserScript==
// @name         TBD
// @namespace    https://github.com/disinfeqt/tbd
// @version      1.0
// @description  Intercept X's bookmark responses, sync them to the local TBD app, and auto-scroll for more.
// @author       disinfeqt
// @match        https://x.com/*
// @match        https://twitter.com/*
// @run-at       document-start
// @grant        unsafeWindow
// @grant        GM_xmlhttpRequest
// @connect      localhost
// ==/UserScript==

(function () {
  "use strict";
  // Reported to the local app so the explorer can tell you when the script it
  // ships is newer than the one you installed.
  const SCRIPT_VERSION =
    typeof GM_info !== "undefined" && GM_info.script
      ? GM_info.script.version
      : "1.0";
  const RAW_SYNC_URL = "http://localhost:41008/api/sync-raw";
  const SETTINGS_URL = "http://localhost:41008/api/settings";
  // Tampermonkey shows its install prompt for a .user.js URL, so the notice can
  // link straight at the copy the app ships.
  const INSTALL_URL = "http://localhost:41008/tbd.user.js";
  const PAGE_CHECK_INTERVAL_MS = 500;
  const SCROLL_INTERVAL_MS = 5000;
  const SCROLL_NUDGE_MS = 1500;
  // Stop once this many batches in a row bring nothing new.
  const AUTO_STOP_EMPTY_BATCHES = 3;
  // How long to wait for the page's own bookmark request before assuming this is
  // a cached timeline that will never fetch anything.
  const FIRST_BATCH_WAIT_MS = 5000;
  const AUTO_START_DELAY_MS = 300;
  const RESUME_KEY = "tbd_resume_sync";
  // scrollTo() is a no-op once we are pinned at the bottom: no scroll event fires,
  // so X's infinite loader can sit there waiting forever.
  const MAX_STALLED_SCROLLS = 4;
  const STALL_JIGGLE_PX = 1200;

  console.log(`[TBD v${SCRIPT_VERSION}] Overlay loaded.`);

  // Which X account is signed in decides which archive the bookmarks join. The
  // twid cookie carries the user id, which never changes; the handle is only a
  // label and is scraped from whatever the page happens to be showing.
  const Account = {
    id: "",
    handle: "",

    detect() {
      // Read the cookie every time: switching X accounts is exactly what this
      // is here for, and a cached id would file the new account's bookmarks
      // under the old one.
      let id = "";
      try {
        const cookie = document.cookie.match(/(?:^|;\s*)twid=([^;]+)/);
        if (cookie) {
          const value = decodeURIComponent(cookie[1]).replace(/^"|"$/g, "");
          const match = value.match(/u=?(\d+)/);
          if (match) id = match[1];
        }
      } catch (e) {}

      if (id && id !== this.id) {
        this.id = id;
        this.handle = ""; // the label belongs to whoever is signed in now
      }
      if (!this.handle) this.handle = this.detectHandle();
      return this;
    },

    detectHandle() {
      const selectors = [
        '[data-testid="SideNav_AccountSwitcher_Button"]',
        '[data-testid="AppTabBar_Profile_Link"]',
      ];
      for (const selector of selectors) {
        const node = document.querySelector(selector);
        if (!node) continue;
        const fromText = (node.innerText || "").match(/@([A-Za-z0-9_]{1,15})/);
        if (fromText) return fromText[1];
        const fromHref = (node.getAttribute("href") || "").match(
          /^\/([A-Za-z0-9_]{1,15})$/,
        );
        if (fromHref) return fromHref[1];
      }
      return "";
    },
  };

  // Every call to the local app identifies the script and the account it is
  // signed in as; without them the app cannot keep two accounts apart.
  function tbdHeaders(extra) {
    const account = Account.detect();
    const headers = Object.assign({ "X-TBD-Version": SCRIPT_VERSION }, extra);
    // The handle stands in as the id when the cookie is not readable. Sending
    // nothing would quietly pool every account into one archive, which is a far
    // worse outcome than an id that changes if the account is ever renamed.
    const id = account.id || (account.handle ? "@" + account.handle : "");
    if (id) headers["X-TBD-Account"] = id;
    if (account.handle) headers["X-TBD-Handle"] = account.handle;
    return headers;
  }

  const THEMES = {
    light: {
      bg: "#ffffff",
      text: "#0f1419",
      sub: "#536471",
      border: "#cfd9de",
      trackOff: "#cfd9de",
      shadow: "0 8px 24px rgba(15, 20, 25, 0.16)",
      tones: {
        neutral: "#536471",
        active: "#1d9bf0",
        success: "#008a00",
        warning: "#b45f00",
        danger: "#b00020",
      },
    },
    dark: {
      bg: "#16181c",
      text: "#e7e9ea",
      sub: "#71767b",
      border: "#2f3336",
      trackOff: "#3e4144",
      shadow: "0 8px 24px rgba(0, 0, 0, 0.45)",
      tones: {
        neutral: "#8b98a5",
        active: "#1d9bf0",
        success: "#00ba7c",
        warning: "#f7b955",
        danger: "#f66570",
      },
    },
  };

  const UI = {
    el: null,
    btn: null,
    titleEl: null,
    statusEl: null,
    statusDot: null,
    newCountEl: null,
    libraryCountEl: null,
    updateEl: null,
    latestVersion: "",
    rowsEl: null,
    rowLabels: [],
    videoSwitch: null,
    imageSwitch: null,
    theme: "light",
    statusText: "Ready",
    statusTone: "neutral",
    timeout: null,
    // Saved during this visit, against the whole library the local app holds.
    // libraryCount stays null until the app reports it, so we never show a
    // made-up 0.
    newCount: 0,
    libraryCount: null,
    settings: {
      media_dir: "media",
      download_videos: true,
      download_images: true,
    },

    createSwitchRow(labelText, title, onToggle) {
      const row = document.createElement("div");
      row.title = title;
      row.style.cssText = `
                display: flex;
                align-items: center;
                justify-content: space-between;
                gap: 8px;
                cursor: pointer;
            `;

      const label = document.createElement("span");
      label.innerText = labelText;
      label.style.cssText = "font-size: 13px; font-weight: 500;";

      const track = document.createElement("button");
      track.type = "button";
      track.setAttribute("role", "switch");
      track.style.cssText = `
                position: relative;
                width: 36px;
                height: 20px;
                border-radius: 10px;
                border: none;
                padding: 0;
                cursor: pointer;
                flex: none;
                transition: background 0.15s ease;
            `;

      const knob = document.createElement("span");
      knob.style.cssText = `
                position: absolute;
                top: 2px;
                left: 2px;
                width: 16px;
                height: 16px;
                border-radius: 50%;
                background: #ffffff;
                box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
                transition: transform 0.15s ease;
            `;
      track.appendChild(knob);

      row.appendChild(label);
      row.appendChild(track);
      row.onclick = onToggle;

      this.rowLabels.push(label);
      return { row, track, knob };
    },

    setSwitch(sw, on) {
      sw.track.setAttribute("aria-checked", on ? "true" : "false");
      sw.track.style.background = on ? "#1d9bf0" : THEMES[this.theme].trackOff;
      sw.knob.style.transform = on ? "translateX(16px)" : "translateX(0)";
    },

    init() {
      const theme = THEMES[this.theme];

      this.el = document.createElement("div");
      this.el.style.cssText = `
                position: fixed;
                bottom: 24px;
                left: 20px;
                width: 232px;
                background: ${theme.bg};
                color: ${theme.text};
                padding: 14px;
                border-radius: 14px;
                font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
                font-size: 13px;
                z-index: 999999;
                border: 1px solid ${theme.border};
                box-shadow: ${theme.shadow};
                display: none;
                flex-direction: column;
                gap: 12px;
                user-select: none;
                letter-spacing: 0;
            `;

      const header = document.createElement("div");
      header.style.cssText =
        "display: flex; align-items: center; justify-content: space-between; gap: 8px;";

      this.titleEl = document.createElement("div");
      this.titleEl.innerText = "TBD";
      this.titleEl.style.cssText =
        "font-size: 14px; font-weight: 800; letter-spacing: 0.2px; flex: 0 1 auto; min-width: 56px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;";

      const statusWrap = document.createElement("div");
      statusWrap.style.cssText =
        "display: flex; align-items: center; gap: 6px; flex: 0 1 auto; min-width: 0;";

      this.statusDot = document.createElement("span");
      this.statusDot.style.cssText =
        "width: 8px; height: 8px; border-radius: 50%; flex: none;";

      this.statusEl = document.createElement("div");
      this.statusEl.style.cssText =
        "font-size: 12px; font-weight: 600; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;";

      statusWrap.appendChild(this.statusDot);
      statusWrap.appendChild(this.statusEl);
      header.appendChild(this.titleEl);
      header.appendChild(statusWrap);

      // The headline numbers: what this visit actually did.
      const counts = document.createElement("div");
      counts.title =
        "New bookmarks saved during this visit, and how many the local app holds in total.";
      counts.style.cssText =
        "display: flex; align-items: baseline; flex-wrap: wrap; gap: 2px 6px; font-size: 13px; font-weight: 700;";

      this.newCountEl = document.createElement("span");
      const countsSep = document.createElement("span");
      countsSep.innerText = "·";
      countsSep.style.cssText = "opacity: 0.5; font-weight: 400;";
      this.libraryCountEl = document.createElement("span");
      this.libraryCountEl.style.cssText = "font-weight: 500;";

      counts.appendChild(this.newCountEl);
      counts.appendChild(countsSep);
      counts.appendChild(this.libraryCountEl);

      // The script cannot know it is stale on its own; the app tells it on every
      // page load, and this is the only place the user will see it.
      this.updateEl = document.createElement("a");
      this.updateEl.href = INSTALL_URL;
      this.updateEl.target = "_blank";
      this.updateEl.rel = "noreferrer";
      this.updateEl.style.cssText = `
                display: none;
                align-items: center;
                gap: 6px;
                font-size: 12px;
                font-weight: 600;
                text-decoration: none;
                border: 1px solid;
                border-radius: 8px;
                padding: 6px 9px;
            `;

      this.btn = document.createElement("button");
      this.btn.type = "button";
      this.btn.innerText = "Start sync";
      this.btn.title =
        "Syncing starts on its own. Press again after it stops to keep scanning older bookmarks.";
      this.btn.style.cssText = `
                display: flex;
                align-items: center;
                justify-content: center;
                text-align: center;
                height: 36px;
                border: none;
                border-radius: 10px;
                background: #1d9bf0;
                color: #ffffff;
                cursor: pointer;
                font: inherit;
                font-size: 14px;
                font-weight: 700;
                width: 100%;
                transition: filter 0.15s ease;
            `;
      this.btn.onmouseenter = () =>
        (this.btn.style.filter = "brightness(0.92)");
      this.btn.onmouseleave = () => (this.btn.style.filter = "");
      this.btn.onclick = () => Scroller.toggle();

      this.rowsEl = document.createElement("div");
      this.rowsEl.style.cssText = `
                display: flex;
                flex-direction: column;
                gap: 10px;
                border-top: 1px solid ${theme.border};
                padding-top: 12px;
            `;

      this.videoSwitch = this.createSwitchRow(
        "Download videos",
        "Save this as the download_videos setting. When off, video media stays queued but is not downloaded.",
        () => this.toggleVideos(),
      );
      this.imageSwitch = this.createSwitchRow(
        "Download images",
        "Save this as the download_images setting. When off, image media stays queued but is not downloaded.",
        () => this.toggleImages(),
      );

      this.rowsEl.appendChild(this.videoSwitch.row);
      this.rowsEl.appendChild(this.imageSwitch.row);

      this.el.appendChild(header);
      this.el.appendChild(this.updateEl);
      this.el.appendChild(counts);
      this.el.appendChild(this.btn);
      this.el.appendChild(this.rowsEl);
      this.updateSettingsButtons();
      this.renderCounts();
      this.renderUpdateNotice();
      this.renderStatus();
      this.loadSettings();

      // Check the page state on a slow interval instead of every animation frame,
      // and only touch the DOM when something actually changed.
      let visible = null;
      const tick = () => {
        if (!document.body) return;
        if (!this.el.parentElement) document.body.appendChild(this.el);

        const onBookmarksPage = this.onBookmarksPage();

        if (onBookmarksPage !== visible) {
          visible = onBookmarksPage;
          this.el.style.display = onBookmarksPage ? "flex" : "none";
          if (onBookmarksPage) Scroller.autoStart();
          else Scroller.armAutoStart();
        }
        if (!onBookmarksPage && Scroller.active) Scroller.stop();
        if (onBookmarksPage) this.applyTheme(this.detectTheme());
      };
      tick();
      setInterval(tick, PAGE_CHECK_INTERVAL_MS);
    },

    onBookmarksPage() {
      return (
        window.location.pathname.includes("/i/history") ||
        window.location.pathname.includes("/i/bookmarks")
      );
    },

    detectTheme() {
      try {
        const bg = getComputedStyle(document.body).backgroundColor;
        const parts = bg.match(/[\d.]+/g);
        if (
          parts &&
          parts.length >= 3 &&
          (parts.length < 4 || Number(parts[3]) > 0)
        ) {
          const luminance =
            0.299 * parts[0] + 0.587 * parts[1] + 0.114 * parts[2];
          return luminance < 128 ? "dark" : "light";
        }
      } catch (e) {}
      return window.matchMedia &&
        window.matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light";
    },

    applyTheme(name) {
      if (name === this.theme || !THEMES[name]) return;
      this.theme = name;
      const theme = THEMES[name];

      this.el.style.background = theme.bg;
      this.el.style.color = theme.text;
      this.el.style.boxShadow = theme.shadow;
      this.el.style.borderColor = Scroller.active ? "#1d9bf0" : theme.border;
      this.rowsEl.style.borderTopColor = theme.border;
      this.titleEl.style.color = theme.text;
      for (const label of this.rowLabels) label.style.color = theme.text;

      this.updateSettingsButtons();
      this.renderCounts();
      this.renderUpdateNotice();
      this.renderStatus();
    },

    // Any mismatch means reinstall, in either direction: a script newer than the
    // app is just as wrong as an old one.
    renderUpdateNotice() {
      if (!this.updateEl) return;
      const stale = this.latestVersion && this.latestVersion !== SCRIPT_VERSION;
      this.updateEl.style.display = stale ? "flex" : "none";
      if (!stale) return;

      const theme = THEMES[this.theme];
      this.updateEl.innerText = `Update this script to v${this.latestVersion}`;
      this.updateEl.title = `You are running v${SCRIPT_VERSION}. Click to reinstall from the local app.`;
      this.updateEl.style.color = theme.tones.warning;
      this.updateEl.style.borderColor = theme.tones.warning;
    },

    renderCounts() {
      const theme = THEMES[this.theme];
      this.newCountEl.innerText = `${this.newCount} new`;
      this.newCountEl.style.color =
        this.newCount > 0 ? theme.tones.success : theme.text;
      const library = this.libraryCount === null ? "—" : this.libraryCount;
      this.libraryCountEl.innerText = `${library} already saved`;
      this.libraryCountEl.style.color = theme.sub;
    },

    // newCount adds up over the visit; libraryCount is the local app's current
    // total, so it is replaced rather than accumulated.
    addCounts(newCount, libraryCount) {
      this.newCount += newCount;
      if (libraryCount !== null) this.libraryCount = libraryCount;
      this.renderCounts();
    },

    renderStatus() {
      const tones = THEMES[this.theme].tones;
      const color = tones[this.statusTone] || tones.neutral;
      this.statusEl.innerText = this.statusText;
      this.statusEl.style.color = color;
      this.statusDot.style.background = color;
    },

    updateStatus(text, tone = "neutral", autoReset = false) {
      this.statusText = text;
      this.statusTone = tone;
      this.renderStatus();

      if (this.timeout) clearTimeout(this.timeout);
      if (autoReset) {
        this.timeout = setTimeout(() => this.resetStatus(), 2200);
      }
    },

    resetStatus() {
      this.updateStatus(
        Scroller.active ? "Syncing" : "Ready",
        Scroller.active ? "active" : "neutral",
      );
    },

    setScrolling(isScrolling) {
      if (isScrolling) {
        this.btn.innerText = "Stop sync";
        this.btn.style.background = "#f4212e";
        this.el.style.borderColor = "#1d9bf0";
        this.resetStatus();
      } else {
        this.btn.innerText = "Start sync";
        this.btn.style.background = "#1d9bf0";
        this.el.style.borderColor = THEMES[this.theme].border;
        this.resetStatus();
      }
    },

    loadSettings() {
      GM_xmlhttpRequest({
        method: "GET",
        url: SETTINGS_URL,
        headers: tbdHeaders(),
        onload: (response) => {
          try {
            this.applySettings(JSON.parse(response.responseText));
          } catch (e) {
            this.updateStatus("Settings error", "danger", true);
          }
        },
        onerror: () => {
          this.updateStatus("Local app offline", "danger");
        },
      });
    },

    applySettings(settings) {
      this.settings = {
        media_dir: settings.media_dir || "media",
        download_videos: settings.download_videos !== false,
        download_images: settings.download_images !== false,
      };
      this.latestVersion = settings.latest_script_version || "";
      this.renderUpdateNotice();
      const handle = (settings.account && settings.account.handle) || "";
      this.titleEl.innerText = handle ? `@${handle}` : "TBD";
      this.titleEl.title = handle
        ? `Bookmarks and settings for @${handle}`
        : "TBD";
      this.updateSettingsButtons();
    },

    updateSettingsButtons() {
      if (!this.videoSwitch || !this.imageSwitch) return;
      this.setSwitch(this.videoSwitch, this.settings.download_videos);
      this.setSwitch(this.imageSwitch, this.settings.download_images);
    },

    saveSettings(next, successText) {
      GM_xmlhttpRequest({
        method: "POST",
        url: SETTINGS_URL,
        headers: tbdHeaders({ "Content-Type": "application/json" }),
        data: JSON.stringify(next),
        onload: (response) => {
          try {
            this.applySettings(JSON.parse(response.responseText));
            this.updateStatus(successText, "neutral", true);
          } catch (e) {
            this.updateStatus("Settings error", "danger", true);
          }
        },
        onerror: () => {
          this.updateStatus("Could not save", "danger", true);
        },
      });
    },

    toggleVideos() {
      const next = {
        media_dir: this.settings.media_dir,
        download_videos: !this.settings.download_videos,
        download_images: this.settings.download_images !== false,
      };

      this.saveSettings(
        next,
        next.download_videos ? "Video downloads on" : "Video downloads off",
      );
    },

    toggleImages() {
      const next = {
        media_dir: this.settings.media_dir,
        download_videos: this.settings.download_videos !== false,
        download_images: !this.settings.download_images,
      };

      this.saveSettings(
        next,
        next.download_images ? "Image downloads on" : "Image downloads off",
      );
    },
  };

  const Scroller = {
    active: false,
    timer: null,
    emptyBatches: 0,
    stalledScrolls: 0,
    autoStartDone: false,
    // When X last handed us a bookmark response; 0 means never on this page load.
    lastBatchAt: 0,

    toggle() {
      if (this.active) this.stop();
      else this.start();
    },

    // Opening the bookmarks page is the whole trigger — no button press needed.
    // New bookmarks only ever appear at the top of the timeline and scrolling
    // down never asks X for it again, so the one thing that matters here is that
    // the page actually fetched a timeline. A client-side navigation replays a
    // cached one and fetches nothing, which is why new bookmarks used to need a
    // manual reload.
    autoStart() {
      if (this.autoStartDone || this.active) return;
      this.autoStartDone = true;

      let mayReload = true;
      try {
        if (sessionStorage.getItem(RESUME_KEY) === "1") {
          sessionStorage.removeItem(RESUME_KEY);
          mayReload = false; // We already reloaded once for this run; never loop.
        }
      } catch (e) {}

      const startedAt = Date.now();
      const waitForFirstBatch = () => {
        if (this.active || !UI.onBookmarksPage()) return;
        if (this.lastBatchAt > 0) {
          this.start();
          return;
        }
        if (Date.now() - startedAt > FIRST_BATCH_WAIT_MS) {
          if (mayReload) this.reloadAndResume();
          else this.start();
          return;
        }
        setTimeout(waitForFirstBatch, AUTO_START_DELAY_MS);
      };

      UI.updateStatus("Loading bookmarks", "active");
      setTimeout(waitForFirstBatch, AUTO_START_DELAY_MS);
    },

    // Re-arm when the user leaves the page, so coming back syncs again.
    armAutoStart() {
      this.autoStartDone = false;
    },

    reloadAndResume() {
      UI.updateStatus("Refreshing page", "active");
      try {
        sessionStorage.setItem(RESUME_KEY, "1");
      } catch (e) {}
      unsafeWindow.location.reload();
    },

    start() {
      if (this.active) return;
      this.active = true;
      this.emptyBatches = 0;
      this.stalledScrolls = 0;
      UI.setScrolling(true);
      this.loop();
    },

    stop() {
      if (!this.active) return;
      this.active = false;
      this.emptyBatches = 0;
      this.stalledScrolls = 0;
      clearTimeout(this.timer);
      UI.setScrolling(false);
    },

    // Every batch X returns lands here. Saving something means there is more to
    // find; a few empty batches in a row means we are back in already-synced
    // territory and can stop.
    noteBatch(savedCount) {
      this.lastBatchAt = Date.now();
      this.stalledScrolls = 0;
      if (savedCount > 0) {
        this.emptyBatches = 0;
        return 0;
      }
      this.emptyBatches += 1;
      return this.emptyBatches;
    },

    // A batch just got processed, so the next content is likely already rendered:
    // scroll again soon instead of waiting out the full interval.
    nudge() {
      if (!this.active) return;
      clearTimeout(this.timer);
      this.timer = setTimeout(() => this.loop(), SCROLL_NUDGE_MS);
    },

    loop() {
      if (!this.active) return;

      const before = window.scrollY;
      window.scrollTo(0, document.body.scrollHeight);

      if (Math.abs(window.scrollY - before) < 2) {
        this.stalledScrolls += 1;
        if (this.stalledScrolls >= MAX_STALLED_SCROLLS) {
          this.stop();
          UI.updateStatus("Reached the end", "warning");
          return;
        }
        // Already pinned at the bottom: that scrollTo fired no scroll event at
        // all. Bounce up and back down so X's infinite loader gets re-armed.
        window.scrollTo(0, Math.max(0, before - STALL_JIGGLE_PX));
        setTimeout(() => {
          if (this.active) window.scrollTo(0, document.body.scrollHeight);
        }, 250);
      }

      this.timer = setTimeout(() => {
        this.loop();
      }, SCROLL_INTERVAL_MS);
    },
  };

  const isBookmarksRequest = (url) =>
    typeof url === "string" &&
    url.includes("Bookmarks") &&
    url.includes("graphql");

  // Hand one intercepted GraphQL response to the local app and report what it did.
  function syncBatch(responseData) {
    if (!responseData) return;
    GM_xmlhttpRequest({
      method: "POST",
      url: RAW_SYNC_URL,
      headers: tbdHeaders({ "Content-Type": "application/json" }),
      data: responseData,
      onload: function (response) {
        try {
          const res = JSON.parse(response.responseText);
          const savedCount = Number(res.saved_count) || 0;
          const libraryCount = Number.isFinite(res.library_count)
            ? res.library_count
            : null;

          UI.addCounts(savedCount, libraryCount);
          const emptyBatches = Scroller.noteBatch(savedCount);

          if (savedCount > 0) {
            UI.updateStatus(`Saved ${savedCount} new`, "success", true);
          } else if (!Scroller.active) {
            UI.updateStatus("Nothing new", "neutral", true);
          } else if (emptyBatches >= AUTO_STOP_EMPTY_BATCHES) {
            Scroller.stop();
            UI.updateStatus("All caught up", "success");
          } else {
            UI.updateStatus(
              `Checking older ${emptyBatches}/${AUTO_STOP_EMPTY_BATCHES}`,
              "neutral",
              true,
            );
          }

          // No-op if the scroller just stopped or was never running.
          Scroller.nudge();
        } catch (e) {
          UI.updateStatus("Server error", "danger", true);
        }
      },
      onerror: function () {
        UI.updateStatus("Local app offline", "danger");
      },
    });
  }

  const PageXHR = unsafeWindow.XMLHttpRequest;
  const originalOpen = PageXHR.prototype.open;
  const originalSend = PageXHR.prototype.send;

  PageXHR.prototype.open = function (method, url) {
    try {
      // url is sometimes a URL object rather than a string.
      this._tbd_url = url == null ? "" : String(url);
    } catch (e) {}
    return originalOpen.apply(this, arguments);
  };

  PageXHR.prototype.send = function () {
    const self = this;
    const onLoad = function () {
      self.removeEventListener("load", onLoad);
      try {
        if (!isBookmarksRequest(self._tbd_url)) return;
        syncBatch(readXHRBody(self));
      } catch (e) {
        console.warn("[TBD] XHR hook failed", e);
      }
    };
    self.addEventListener("load", onLoad);
    return originalSend.apply(this, arguments);
  };

  // responseText throws outright when the caller set a non-text responseType, so
  // never let that take the whole batch down with it.
  function readXHRBody(xhr) {
    try {
      return xhr.responseText;
    } catch (e) {}
    try {
      return typeof xhr.response === "string"
        ? xhr.response
        : JSON.stringify(xhr.response);
    } catch (e) {}
    return "";
  }

  // X does not always use XHR for GraphQL; mirror the hook on fetch so those
  // batches are not silently missed.
  const originalFetch = unsafeWindow.fetch;
  if (typeof originalFetch === "function") {
    unsafeWindow.fetch = function (input) {
      const promise = originalFetch.apply(unsafeWindow, arguments);
      let url = "";
      try {
        url =
          typeof input === "string"
            ? input
            : input && input.url
              ? input.url
              : String(input);
      } catch (e) {}
      if (!isBookmarksRequest(url)) return promise;

      return promise.then((response) => {
        // Clone before the page reads the body, or it gets consumed twice.
        try {
          response
            .clone()
            .text()
            .then(syncBatch)
            .catch(() => {});
        } catch (e) {
          console.warn("[TBD] fetch hook failed", e);
        }
        return response;
      });
    };
  }

  UI.init();
})();
