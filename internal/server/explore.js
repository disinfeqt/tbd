"use strict";
const $ = (sel, root) => (root || document).querySelector(sel);
const fmt = (n) => Number(n).toLocaleString("en-US");
// The one definition of "narrow": the stylesheet's breakpoint, so the
// layout and the behaviour that depends on it can never disagree.
const NARROW = matchMedia("(max-width: 720px)");

function el(tag, cls, text) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text != null) n.textContent = text;
  return n;
}

const TYPE_LABELS = {
  photo: "photo",
  video: "video",
  animated_gif: "GIF",
};
const KIND_TABS = [
  ["all", "All"],
  ["photo", "Photos"],
  ["video", "Videos"],
  ["gif", "GIFs"],
  ["text", "Text"],
  ["missing", "Missing"],
];

/* ---- State ---- */
const state = {
  view: "grid", // grid | authors | media | timeline | settings | setup
  account: "", // which X account's archive is on screen
  q: "",
  type: "all",
  author: "",
  month: "",
  sort: "newest",
  page: 1,
  total: 0,
  counts: null,
  items: [],
};
let fetchController = null;
let statsCache = null;
let accountsCache = null;
let setupCache = null;

// Every archive query is scoped to one account; the activity log and the
// download strip stay app-wide and are left alone.
function withAccount(url) {
  if (!state.account) return url;
  return (
    url +
    (url.includes("?") ? "&" : "?") +
    "account=" +
    encodeURIComponent(state.account)
  );
}

async function loadAccounts(force) {
  if (accountsCache && !force) return accountsCache;
  const res = await fetch("/api/explore/accounts");
  if (!res.ok) throw new Error("accounts request failed");
  accountsCache = await res.json();
  return accountsCache;
}

async function loadSetup(force) {
  if (setupCache && !force) return setupCache;
  const res = await fetch("/api/explore/setup");
  if (!res.ok) throw new Error("setup request failed");
  setupCache = await res.json();
  return setupCache;
}

const accountName = (a) =>
  a.handle ? "@" + a.handle : "Account " + a.id.slice(0, 8);

function renderAccountSwitcher(data) {
  const items = (data && data.items) || [];
  // Always name the account on screen: the explorer shows exactly one
  // archive at a time, and that is invisible if nothing says which.
  $("#acctwrap").hidden = items.length === 0;
  $("#acct").disabled = items.length < 2;
  const sel = $("#acct");
  sel.textContent = "";
  for (const a of items) {
    const option = el("option", "", accountName(a));
    option.value = a.id;
    sel.append(option);
  }
  if (state.account) sel.value = state.account;
  fitAccountSelect();
}

// A native select is as wide as its widest option, so one long handle
// stretches the header for every account. Size it to the one on screen.
function fitAccountSelect() {
  const sel = $("#acct");
  const option = sel.options[sel.selectedIndex];
  if (!option) return;
  const probe = $("#acctprobe");
  probe.textContent = option.textContent;
  const width = probe.getBoundingClientRect().width;
  // 28px of horizontal padding, 2px of border, 2px of slack.
  if (width > 0) sel.style.width = Math.ceil(width) + 32 + "px";
}

function switchAccount(id) {
  if (!id || id === state.account) return;
  state.account = id;
  statsCache = null;
  authorsCache = null;
  state.counts = null; // tab counts (incl. Missing) belong to the old account
  clearFilters();
  loadStats().catch(() => {});
  setView("grid");
}

async function ensureStats() {
  if (statsCache) return statsCache;
  const res = await fetch(withAccount("/api/explore/stats"));
  if (!res.ok) throw new Error("stats request failed");
  statsCache = await res.json();
  return statsCache;
}

/* ---- Header: what this archive is, and one menu for everywhere else ---- */
const monthYear = (iso) =>
  new Date(iso).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
  });

// "Aug 2026 – Aug 2026" says nothing twice and wraps onto two lines.
// Collapse the range to whatever actually varies.
function timeSpan(firstISO, lastISO) {
  const first = new Date(firstISO);
  const last = new Date(lastISO);
  if (first.getFullYear() !== last.getFullYear())
    return first.getFullYear() + " – " + last.getFullYear();
  if (first.getMonth() === last.getMonth()) return monthYear(firstISO);
  const month = (d) =>
    d.toLocaleDateString(undefined, { month: "short" });
  return month(first) + " – " + month(last) + " " + last.getFullYear();
}

// Something the user has to act on: nothing is syncing, or the script
// that does the syncing is behind the one this app ships.
function setupAlert() {
  if (!setupCache) return "";
  // A phone is almost never the browser that syncs, so its own view of
  // the userscript says nothing about whether syncing works — only the
  // machine running the extension can answer that. Whether the archive
  // is empty is still worth saying anywhere.
  if (!NARROW.matches) {
    if (!setupCache.script.installed) return "Userscript not detected";
    if (setupCache.script.outdated) return "Userscript update available";
  }
  if (setupCache.bookmarks === 0) return "Finish setting up";
  return "";
}

let lastStats = null;

async function loadStats() {
  renderStatline(await ensureStats());
}

function renderStatline(stats) {
  lastStats = stats;

  // Only ever holds an alert: the bookmark total it used to carry is
  // already in the count bar under the filters, and in the menu.
  const line = $("#statline");
  line.textContent = "";

  const alert = setupAlert();
  if (alert) {
    const chip = el("button", "warn", alert);
    chip.addEventListener("click", () => setView("setup"));
    line.append(chip);
  }

  renderMenu(stats, alert);
}

function renderMenu(stats, alert) {
  const btn = $("#menubtn");
  btn.textContent = "Menu";
  if (alert) btn.prepend(el("span", "alert"));

  const pop = $("#menupop");
  pop.textContent = "";
  const item = (view, label, count, cls) => {
    const b = el("button", "menuitem" + (cls ? " " + cls : ""));
    b.append(el("span", "", label), el("span", "n", count || ""));
    if (state.view === view) b.setAttribute("aria-current", "true");
    b.addEventListener("click", () => {
      $("#menu").open = false;
      if (view === "grid") goHome();
      else setView(view);
    });
    pop.append(b);
    return b;
  };

  const downloaded = stats.totals.media_downloaded;
  const mediaTotal = stats.totals.media_total;
  item("grid", "Bookmarks", fmt(stats.totals.bookmarks));
  item(
    "media",
    "Media files",
    // The fraction only matters while downloads are catching up.
    downloaded < mediaTotal
      ? fmt(downloaded) + " / " + fmt(mediaTotal)
      : fmt(mediaTotal),
  );
  item("authors", "Authors", fmt(stats.totals.authors));
  if (stats.first_bookmark) {
    item(
      "timeline",
      "Timeline",
      timeSpan(stats.first_bookmark, stats.last_bookmark),
    );
  }
  item("activity", "Activity", "", "live");
  pop.append(el("div", "menusep"));
  item("settings", "Settings", "");
  item("setup", "Set up", alert ? "!" : "", alert ? "attention" : "");
}

// A menu that stays open after you click past it is its own kind of clutter.
document.addEventListener("click", (e) => {
  const menu = $("#menu");
  if (menu.open && !menu.contains(e.target)) menu.open = false;
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") $("#menu").open = false;
});

/* ---- Tabs with live counts ---- */
function renderTabs() {
  const tabs = $("#tabs");
  tabs.textContent = "";
  for (const [key, label] of KIND_TABS) {
    const b = el("button");
    b.setAttribute("aria-pressed", String(state.type === key));
    b.append(label);
    if (state.counts && state.counts[key] != null)
      b.append(el("span", "n", fmt(state.counts[key])));
    b.addEventListener("click", () => {
      if (state.type === key) return;
      state.type = key;
      resetAndLoad();
    });
    tabs.append(b);
  }
}

/* ---- Icons / avatar ---- */
const svgNS = "http://www.w3.org/2000/svg";
function icon(kind) {
  const s = document.createElementNS(svgNS, "svg");
  s.setAttribute("viewBox", "0 0 12 12");
  const p = document.createElementNS(svgNS, "path");
  if (kind === "play")
    p.setAttribute("d", "M2.5 1.5 L10.5 6 L2.5 10.5 Z");
  if (kind === "stack")
    p.setAttribute("d", "M1 4 h7 v7 h-7 Z M4 1 h7 v7 h-2 V4 H4 Z");
  s.append(p);
  return s;
}

function hueFor(handle) {
  let h = 0;
  for (const c of handle.toLowerCase())
    h = (h * 31 + c.charCodeAt(0)) % 360;
  return h;
}
// First grapheme of the name — indexing with [0] splits emoji into a
// lone surrogate (�). Prefer the first letter/digit so "🔥 Jo" → "J";
// an all-emoji name shows the emoji itself.
function avatarChar(str) {
  const graphemes =
    typeof Intl !== "undefined" && Intl.Segmenter
      ? Array.from(new Intl.Segmenter().segment(str), (s) => s.segment)
      : Array.from(str);
  const letter = graphemes.find((g) => /\p{L}|\p{N}/u.test(g));
  return (letter || graphemes[0] || "?").toUpperCase();
}
function avatarEl(t) {
  const a = el("span", "avatar");
  const handle = t.screen_name || "?";
  a.style.background = `hsl(${hueFor(handle)} 45% 45%)`;
  a.textContent = avatarChar(t.name || handle);
  return a;
}

const fmtDate = (d) =>
  new Date(d).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
const shortDate = (d) =>
  new Date(d).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
  });

/* ---- Media helpers ---- */
// /media/<account>/<file>: each account keeps its own folder, and the
// same tweet saved by two accounts produces the same file name in both.
const mediaPath = (m) =>
  encodeURIComponent(m.account_id || state.account) +
  "/" +
  encodeURIComponent(m.file);
const mediaSrc = (m) => "/media/" + mediaPath(m);
// A frame pulled from the file by the local app. iOS draws nothing for a
// video it has not played, so this is the only preview a card can count
// on there; elsewhere it just saves fetching the video to show one.
const posterSrc = (m) => "/poster/" + mediaPath(m);
// iOS Safari draws nothing at all for a preload="metadata" video — no
// first frame, no poster, just an empty box — until the file plays, and
// on a touch screen the hover that would play it never comes. Asking for
// the frame a tenth of a second in gives it something to paint at rest.
const videoSrc = (m) => mediaSrc(m) + "#t=0.1";
const isMotion = (m) => m.type === "video" || m.type === "animated_gif";
function fmtDuration(ms) {
  const total = Math.round(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (n) => String(n).padStart(2, "0");
  return h ? h + ":" + pad(m) + ":" + pad(s) : m + ":" + pad(s);
}

function badgeEl(t) {
  const wrap = el("div", "badges");
  const kinds = new Set(t.media.map((m) => m.type));
  if (kinds.has("video")) {
    const b = el("span", "badge");
    b.append(icon("play"));
    const longest = Math.max(...t.media.map((m) => m.duration_ms || 0));
    if (longest > 0) b.append(" " + fmtDuration(longest));
    wrap.append(b);
  } else if (kinds.has("animated_gif")) {
    wrap.append(el("span", "badge", "GIF"));
  }
  if (t.media.length > 1) {
    const b = el("span", "badge right");
    b.append(icon("stack"), " " + t.media.length);
    wrap.append(b);
  }
  return wrap;
}

/* ---- Masonry columns ---- */
let cols = [];
let cardNodes = [];
function colCount() {
  // Phones get a fixed pair: fitting 240px columns would leave one
  // stack of oversized cards, and the same query drives the layout.
  if (NARROW.matches) return 2;
  const grid = $("#grid");
  const gap = parseFloat(getComputedStyle(grid).columnGap) || 14;
  return Math.max(
    1,
    Math.min(4, Math.floor((grid.clientWidth + gap) / (240 + gap))),
  );
}
function setupColumns() {
  const grid = $("#grid");
  grid.textContent = "";
  cols = [];
  for (let i = 0; i < colCount(); i++) {
    const c = el("div", "gcol");
    grid.append(c);
    cols.push(c);
  }
}
function placeCard(node) {
  let best = cols[0];
  for (const c of cols) if (c.offsetHeight < best.offsetHeight) best = c;
  best.append(node);
}
let resizeTimer = null;
addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    if (!cardNodes.length || cols.length === colCount()) return;
    setupColumns();
    cardNodes.forEach(placeCard);
  }, 150);
});

/* ---- Cards ---- */
function card(t) {
  const hasMedia = t.media && t.media.length > 0;
  const c = el("button", "mcard" + (hasMedia ? "" : " tcard"));
  // Look the index up at click time — deletions splice state.items.
  c.addEventListener("click", () => openLb(state.items.indexOf(t)));

  if (!hasMedia) {
    c.append(el("div", "quote", t.full_text || "(no text)"));
  } else {
    const lead = t.media.find((m) => m.file && !m.missing) || t.media[0];
    const thumb = el("div", "thumb");
    // Reserve the real aspect ratio up front so masonry placement and
    // infinite-scroll measurements are correct before the file loads.
    if (lead.width && lead.height)
      thumb.style.aspectRatio = lead.width + " / " + lead.height;
    if (lead.file && !lead.missing) {
      if (isMotion(lead)) {
        const v = document.createElement("video");
        v.src = videoSrc(lead);
        v.poster = posterSrc(lead);
        v.muted = true;
        v.loop = true;
        v.playsInline = true;
        // The poster carries the card at rest, so a phone never fetches
        // the video itself; a mouse still gets its hover preview, which
        // wants the metadata ready.
        v.preload = NARROW.matches ? "none" : "metadata";
        c.addEventListener("mouseenter", () => {
          v.play().catch(() => {});
        });
        c.addEventListener("mouseleave", () => {
          v.pause();
        });
        thumb.append(v);
      } else {
        const img = document.createElement("img");
        img.src = mediaSrc(lead);
        img.loading = "lazy";
        img.alt = "";
        thumb.append(img);
      }
    } else {
      thumb.classList.add("pending");
      thumb.append(
        (TYPE_LABELS[lead.type] || lead.type) +
          (lead.missing ? " · file missing" : " · not downloaded"),
      );
    }
    thumb.append(badgeEl(t));
    if (t.full_text) {
      const peek = el("div", "peek");
      peek.append(el("div", "ptext", t.full_text));
      thumb.append(peek);
    }
    c.append(thumb);
  }

  const meta = el("div", "meta");
  meta.append(avatarEl(t));
  meta.append(el("span", "handle", "@" + (t.screen_name || "unknown")));
  meta.append(el("span", "when", shortDate(t.created_at)));
  c.append(meta);

  return c;
}

/* ---- Fetch + render ---- */
function params() {
  const p = new URLSearchParams();
  if (state.q) p.set("q", state.q);
  if (state.author) p.set("author", state.author);
  if (state.month) p.set("month", state.month);
  if (state.type !== "all") p.set("type", state.type);
  if (state.sort !== "newest") p.set("sort", state.sort);
  if (state.account) p.set("account", state.account);
  p.set("page", String(state.page));
  return p;
}

async function fetchInto(url, emptyMessage) {
  const grid = $("#grid");
  if (fetchController) fetchController.abort();
  fetchController = new AbortController();
  try {
    const res = await fetch(url, { signal: fetchController.signal });
    if (!res.ok) throw new Error("request failed");
    return await res.json();
  } catch (err) {
    if (err.name === "AbortError") return null;
    grid.classList.remove("loading");
    grid.textContent = "";
    grid.append(el("div", "empty", emptyMessage));
    $("#more").hidden = true;
    return null;
  }
}

function appendCards(tweets) {
  tweets.forEach((t) => {
    const node = card(t);
    cardNodes.push(node);
    placeCard(node);
  });
}

function updateCount() {
  $("#count").textContent =
    fmt(state.total) + (state.total === 1 ? " bookmark" : " bookmarks");
}

async function loadPage(append) {
  if (state.type === "missing") return loadMissing();
  resetFixAll(true);
  const grid = $("#grid");
  if (!append) grid.classList.add("loading");

  const data = await fetchInto(
    "/api/explore/tweets?" + params(),
    "Could not load bookmarks — is the local app running?",
  );
  if (!data) return;

  state.total = data.total;
  const missingCount = state.counts ? state.counts.missing : null;
  state.counts = data.counts || null;
  if (state.counts && missingCount != null)
    state.counts.missing = missingCount;
  if (!append) {
    state.items = [];
    cardNodes = [];
    setupColumns();
  }
  state.items.push(...data.items);
  appendCards(data.items);

  if (!state.items.length) {
    grid.textContent = "";
    grid.append(el("div", "empty", "No bookmarks match."));
  }
  updateCount();
  $("#more").hidden = state.items.length >= data.total;
  renderTabs();
  updateChip();
  grid.classList.remove("loading");
  // A short result may leave the sentinel still in view — keep filling.
  maybeLoadMore();
}

async function loadMissing() {
  const grid = $("#grid");
  grid.classList.add("loading");
  $("#more").hidden = true;
  resetFixAll(true);

  const data = await fetchInto(
    withAccount("/api/explore/missing"),
    "Could not check for missing files — is the local app running?",
  );
  if (!data) return;

  state.total = data.total;
  if (!state.counts) state.counts = {};
  state.counts.missing = data.total;
  state.items = data.items;
  cardNodes = [];
  setupColumns();
  appendCards(data.items);

  if (!state.items.length) {
    grid.textContent = "";
    grid.append(
      el(
        "div",
        "empty",
        "No missing files — every downloaded media file is on disk.",
      ),
    );
  }
  updateCount();
  resetFixAll(!state.items.length);
  renderTabs();
  updateChip();
  grid.classList.remove("loading");
}

/* ---- Remove all missing bookmarks (two-click confirm) ---- */
let fixArmTimer = null;
function resetFixAll(hide) {
  const btn = $("#fixall");
  clearTimeout(fixArmTimer);
  fixArmTimer = null;
  btn.classList.remove("armed");
  btn.disabled = false;
  btn.textContent = "Remove all " + fmt(state.total);
  btn.hidden = hide;
}
$("#fixall").addEventListener("click", async () => {
  const btn = $("#fixall");
  if (!fixArmTimer) {
    btn.classList.add("armed");
    btn.textContent = "Click again to confirm";
    fixArmTimer = setTimeout(() => resetFixAll(false), 4000);
    return;
  }
  clearTimeout(fixArmTimer);
  fixArmTimer = null;
  btn.disabled = true;
  let res;
  try {
    res = await fetch(withAccount("/api/explore/missing/fix"), {
      method: "POST",
    });
  } catch {
    res = null;
  }
  if (!res || !res.ok) {
    alert("Failed to remove bookmarks — is the local app running?");
    resetFixAll(false);
    return;
  }
  if (state.counts) state.counts.missing = 0;
  resetAndLoad(); // re-fetches the Missing tab → empty state
});

/* ---- Infinite loading ---- */
let loadingMore = false;
let loadGen = 0; // bumped on every filter reset to cancel stale appends
function sentinelNear() {
  const s = $("#more");
  return !s.hidden && s.getBoundingClientRect().top < innerHeight + 600;
}
// The one way to pull the next page, whoever asks: the grid's sentinel,
// the feed running out of panes, or the panel's next arrow. Returns how
// many items arrived, so callers can stop when the archive runs out.
async function loadMoreItems() {
  if (loadingMore || $("#more").hidden) return 0;
  const gen = loadGen;
  loadingMore = true;
  state.page++;
  const before = state.items.length;
  await loadPage(true);
  loadingMore = false;
  // A reset mid-flight owns whatever it loaded; this call has nothing.
  return gen === loadGen ? state.items.length - before : 0;
}
async function maybeLoadMore() {
  if (!sentinelNear()) return;
  const gen = loadGen;
  // Keep filling until the sentinel leaves the viewport margin.
  if ((await loadMoreItems()) && gen === loadGen) maybeLoadMore();
}
new IntersectionObserver(
  (entries) => {
    if (entries.some((entry) => entry.isIntersecting)) maybeLoadMore();
  },
  { rootMargin: "600px 0px" },
).observe($("#more"));

// The pinned filter row steps aside on the way down and returns on the
// first scroll back up. The dead band keeps a wobbling finger — or iOS
// rubber-banding — from flickering it; near the top it always shows.
let lastScrollY = scrollY;
addEventListener(
  "scroll",
  () => {
    const y = scrollY;
    if (Math.abs(y - lastScrollY) < 6) return;
    document.body.classList.toggle("scrolldown", y > lastScrollY && y > 80);
    lastScrollY = y;
  },
  { passive: true },
);

// Mirror the active view and filters into the URL so views are
// shareable and survive a reload; defaults are omitted to keep it clean.
function syncURL() {
  const p = new URLSearchParams();
  if (state.view !== "grid") p.set("view", state.view);
  if (state.q) p.set("q", state.q);
  if (state.type !== "all") p.set("type", state.type);
  if (state.author) p.set("author", state.author);
  if (state.month) p.set("month", state.month);
  if (state.sort !== "newest") p.set("sort", state.sort);
  if (state.account) p.set("account", state.account);
  const qs = p.toString();
  history.replaceState(null, "", qs ? "?" + qs : location.pathname);
}

function resetAndLoad() {
  loadGen++;
  state.page = 1;
  syncURL();
  loadPage(false);
}

const monthName = (ym) => {
  const [y, mo] = ym.split("-").map(Number);
  return new Date(y, mo - 1, 1).toLocaleDateString(undefined, {
    month: "short",
    year: "numeric",
  });
};

function updateChip() {
  const chip = $("#chip");
  chip.hidden = !state.author;
  if (state.author) $("span", chip).textContent = "@" + state.author;
  const mchip = $("#mchip");
  mchip.hidden = !state.month;
  if (state.month) $("span", mchip).textContent = monthName(state.month);
}

/* ---- Subpages ---- */
function applyView() {
  const inGrid = state.view === "grid";
  for (const sel of [".bar", ".countbar", "#grid"])
    $(sel).hidden = !inGrid;
  if (!inGrid) {
    $("#more").hidden = true;
    $("#chip").hidden = true;
    $("#mchip").hidden = true;
    $("#fixall").hidden = true;
  }
  $("#subpage").hidden = inGrid;
}

function setView(view) {
  state.view = view;
  if (lastStats) renderMenu(lastStats, setupAlert());
  syncURL();
  applyView();
  if (view === "grid") resetAndLoad();
  else renderSubpage();
}

// Subpage counts are archive-wide, so navigating from a subpage row
// starts from a clean slate — lingering filters would contradict the
// count that was just clicked.
function clearFilters() {
  state.q = "";
  $("#q").value = "";
  state.type = "all";
  state.author = "";
  state.month = "";
}

// Logo: back to the unfiltered grid.
function goHome() {
  clearFilters();
  state.sort = "newest";
  $("#sort").value = "newest";
  setView("grid");
}

const VIEW_TITLES = {
  authors: "Authors",
  media: "Media",
  timeline: "Timeline",
  activity: "Activity",
  settings: "Settings",
  setup: "Set up",
};

async function renderSubpage() {
  const sp = $("#subpage");
  sp.textContent = "";
  const head = el("div", "sphead");
  const back = el("button", "spback", "←");
  back.addEventListener("click", () => setView("grid"));
  head.append(back, el("h2", "sptitle", VIEW_TITLES[state.view] || ""));
  sp.append(head);
  const body = el("div", "spbody");
  sp.append(body);
  body.append(el("div", "spnote", "Loading…"));
  try {
    if (state.view === "authors") await renderAuthors(body);
    else if (state.view === "media") await renderMedia(body);
    else if (state.view === "timeline") await renderTimeline(body);
    else if (state.view === "activity") await renderActivity(body);
    else if (state.view === "settings") await renderSettings(body);
    else if (state.view === "setup") await renderSetup(body);
  } catch {
    body.textContent = "";
    body.append(
      el("div", "spnote", "Could not load — is the local app running?"),
    );
  }
}

/* ---- Setup: what is still missing between here and a synced archive ---- */
const TAMPERMONKEY_URL =
  "https://chromewebstore.google.com/detail/tampermonkey/dhdgffkkebhmkfjojejmpbldmpobfkfo";

function setupStep(n, done, title, text, action) {
  const step = el("div", "step" + (done ? " done" : ""));
  step.append(el("span", "stepn", done ? "✓" : String(n)));
  const bodyCol = el("div");
  bodyCol.append(el("h3", "", title));
  if (text) bodyCol.append(el("p", "", text));
  if (action) bodyCol.append(action);
  step.append(bodyCol);
  return step;
}

function linkButton(label, href, quiet) {
  const a = el("a", "btn" + (quiet ? " quiet" : ""), label);
  a.href = href;
  a.target = "_blank";
  a.rel = "noreferrer";
  return a;
}

async function renderSetup(body) {
  const data = await loadSetup(true);
  const script = data.script || {};
  // The header chip and menu badge derive from setupCache too — recompute
  // them from this fresh copy so "not detected" clears once it connects.
  if (lastStats) renderStatline(lastStats);
  body.textContent = "";

  const head = el("div", "panel");
  head.append(el("h3", "", "Getting bookmarks in"));
  const status = el("p");
  status.append(
    el(
      "span",
      "dot" +
        (script.installed ? (script.outdated ? " warn" : " ok") : ""),
    ),
  );
  if (!script.installed) {
    status.append(
      "The userscript has not called in since TBD started. Open x.com in a tab once it is installed.",
    );
  } else if (script.outdated) {
    status.append(
      "Userscript v" +
        script.version +
        " is installed, but v" +
        script.latest_version +
        " ships with this app. Reinstall to update.",
    );
  } else {
    status.append("Userscript v" + script.version + " is connected.");
  }
  head.append(status);
  body.append(head);

  const steps = el("div", "panel");
  steps.append(
    setupStep(
      1,
      script.installed,
      "Install Tampermonkey",
      "The browser extension that runs the userscript. Turn on Allow User Scripts in its settings, or nothing will run.",
      linkButton("Get Tampermonkey", TAMPERMONKEY_URL, true),
    ),
  );
  steps.append(
    setupStep(
      2,
      script.installed && !script.outdated,
      "Install the userscript",
      "Tampermonkey opens its install prompt for this link. Use the same link later to update.",
      linkButton(
        script.installed && !script.outdated
          ? "Reinstall the script"
          : "Install the script",
        data.install_url,
        script.installed && !script.outdated,
      ),
    ),
  );
  steps.append(
    setupStep(
      3,
      data.bookmarks > 0,
      "Open your X bookmarks",
      "Syncing starts on its own and the panel in the corner counts what it saves. Leave TBD running while it works.",
      linkButton(
        "Open x.com bookmarks",
        data.sync_url,
        data.bookmarks > 0,
      ),
    ),
  );
  body.append(steps);

  const accountsPanel = el("div", "panel");
  accountsPanel.append(el("h3", "", "Accounts"));
  const views = data.accounts || [];
  if (!views.length) {
    accountsPanel.append(
      el(
        "p",
        "",
        "Nothing synced yet. Whichever X account you are signed in as becomes the first one, and every account after that keeps its own bookmarks and its own media folder.",
      ),
    );
  } else {
    for (const account of views) {
      accountsPanel.append(
        el(
          "p",
          "",
          accountName(account) +
            " — " +
            fmt(account.bookmarks) +
            " bookmarks in " +
            (account.media_dir_resolved || account.media_dir || "media"),
        ),
      );
    }
    const go = el("button", "btn quiet", "Account settings");
    go.addEventListener("click", () => setView("settings"));
    accountsPanel.append(go);
  }
  body.append(accountsPanel);

  // The steps are completed elsewhere — in the extension, in an x.com tab —
  // so refresh while the page is open instead of asking for a reload.
  clearTimeout(setupTimer);
  setupTimer = setTimeout(() => {
    if (state.view === "setup") renderSubpage();
  }, 4000);
}

let setupTimer = null;

/* ---- Settings: one panel per account, plus the shared defaults ---- */
function settingsPanel(title, note, values, patchID) {
  const panel = el("div", "panel");
  panel.append(el("h3", "", title));
  if (note) panel.append(el("p", "", note));

  const saved = el("span", "saved");
  const save = async (patch) => {
    let res;
    try {
      res = await fetch("/api/explore/accounts", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(Object.assign({ id: patchID }, patch)),
      });
    } catch {
      res = null;
    }
    if (!res || !res.ok) {
      alert("Could not save — is the local app running?");
      return;
    }
    accountsCache = await res.json();
    statsCache = null;
    saved.textContent = "Saved";
    setTimeout(() => (saved.textContent = ""), 2000);
  };

  const field = el("div", "field");
  field.append(el("label", "", "Media folder"));
  const input = el("input");
  input.type = "text";
  // The stored value, not the resolved one: an account left empty follows
  // the global default, and saving the resolved path back would pin it.
  input.value = values.media_dir || "";
  input.placeholder = values.media_dir_resolved || "media";
  field.append(input);
  const saveBtn = el("button", "btn quiet", "Save");
  saveBtn.addEventListener("click", () =>
    save({ media_dir: input.value }),
  );
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") save({ media_dir: input.value });
  });
  field.append(saveBtn, saved);
  panel.append(field);

  const toggles = el("div", "toggles");
  const toggle = (label, key) => {
    const wrap = el("label");
    const box = el("input");
    box.type = "checkbox";
    box.checked = values[key] !== false;
    box.addEventListener("change", () => save({ [key]: box.checked }));
    wrap.append(box, label);
    return wrap;
  };
  toggles.append(
    toggle("Download videos", "download_videos"),
    toggle("Download images", "download_images"),
  );
  panel.append(toggles);
  return panel;
}

async function renderSettings(body) {
  const data = await loadAccounts(true);
  body.textContent = "";

  const items = data.items || [];
  for (const account of items) {
    const synced = account.last_sync_at
      ? "last synced " +
        new Date(account.last_sync_at).toLocaleDateString(undefined, {
          month: "short",
          day: "numeric",
          year: "numeric",
        })
      : "not synced yet";
    body.append(
      settingsPanel(
        accountName(account),
        fmt(account.bookmarks) + " bookmarks · " + synced,
        account,
        account.id,
      ),
    );
  }

  body.append(
    settingsPanel(
      "Defaults for new accounts",
      "Where a newly seen account starts. Existing accounts keep their own settings.",
      data.defaults || {},
      "",
    ),
  );

  const help = el("div", "panel");
  help.append(el("h3", "", "Userscript"));
  help.append(
    el(
      "p",
      "",
      "Install it, update it, or check whether it is talking to this app.",
    ),
  );
  const go = el("button", "btn quiet", "Open setup");
  go.addEventListener("click", () => setView("setup"));
  help.append(go);
  body.append(help);
}

function statRow(cls, cells, count, max, onClick) {
  const row = el("button", "srow " + cls);
  for (const cell of cells) row.append(cell);
  const track = el("span", "strack");
  const bar = el("span", "sbar");
  bar.style.width = (100 * count) / max + "%";
  track.append(bar);
  row.append(track, el("span", "scount", fmt(count)));
  row.addEventListener("click", onClick);
  return row;
}

let authorsCache = null;
async function renderAuthors(body) {
  if (!authorsCache) {
    const res = await fetch(withAccount("/api/explore/authors"));
    if (!res.ok) throw new Error("authors request failed");
    authorsCache = (await res.json()).items;
  }
  body.textContent = "";
  if (!authorsCache.length) {
    body.append(el("div", "spnote", "No authors yet."));
    return;
  }
  const max = Math.max(1, ...authorsCache.map((a) => a.count));
  for (const a of authorsCache) {
    const who = el("span", "swho");
    who.append(el("b", "", a.name || a.screen_name));
    who.append(el("span", "shandle", "@" + a.screen_name));
    body.append(
      statRow(
        "author",
        [avatarEl({ screen_name: a.screen_name, name: a.name }), who],
        a.count,
        max,
        () => {
          clearFilters();
          state.author = a.screen_name;
          setView("grid");
        },
      ),
    );
  }
}

async function renderMedia(body) {
  const stats = await ensureStats();
  body.textContent = "";
  const summary =
    fmt(stats.totals.media_downloaded) +
    " of " +
    fmt(stats.totals.media_total) +
    " files downloaded" +
    (stats.totals.media_failed > 0
      ? " · " + fmt(stats.totals.media_failed) + " failed"
      : "");
  body.append(el("p", "spsummary", summary));
  const LABELS = {
    photo: ["Photos", "photo"],
    video: ["Videos", "video"],
    animated_gif: ["GIFs", "gif"],
  };
  const types = (stats.media_types || []).filter((mt) => LABELS[mt.type]);
  if (!types.length) {
    body.append(el("div", "spnote", "No media yet."));
    return;
  }
  const max = Math.max(1, ...types.map((mt) => mt.count));
  for (const mt of types) {
    const [label, tab] = LABELS[mt.type];
    body.append(
      statRow(
        "plain",
        [el("span", "slabel", label)],
        mt.count,
        max,
        () => {
          clearFilters();
          state.type = tab;
          setView("grid");
        },
      ),
    );
  }
}

/* ---- Activity: live tail of the app's log ---- */
let logTimer = null;
let logLastId = 0;
function logRow(entry) {
  const row = el("div", "logrow " + entry.level);
  const marks = { info: "•", warn: "!", error: "✗" };
  row.append(
    el(
      "span",
      "ltime",
      new Date(entry.time).toLocaleTimeString(undefined, {
        hour12: false,
      }),
    ),
    el("span", "lmark", marks[entry.level] || "•"),
    el("span", "lmsg", entry.msg),
  );
  return row;
}
async function renderActivity(body) {
  body.classList.add("wide");
  body.textContent = "";
  const list = el("div", "loglist");
  body.append(list);
  logLastId = 0;
  clearInterval(logTimer);

  const poll = async () => {
    if (state.view !== "activity") {
      clearInterval(logTimer);
      logTimer = null;
      return;
    }
    let data;
    try {
      const res = await fetch("/api/explore/logs?after=" + logLastId);
      if (!res.ok) throw new Error();
      data = await res.json();
    } catch {
      return; // transient; try again next tick
    }
    if (!data.items.length) {
      if (!list.children.length)
        list.append(el("div", "spnote", "No activity yet."));
      return;
    }
    const note = list.querySelector(".spnote");
    if (note) note.remove();
    // Follow the tail only if the user is already at the bottom.
    const stick =
      list.scrollHeight - list.scrollTop - list.clientHeight < 40;
    for (const entry of data.items) list.append(logRow(entry));
    while (list.children.length > 500) list.firstChild.remove();
    logLastId = data.last_id;
    if (stick) list.scrollTop = list.scrollHeight;
  };
  await poll();
  list.scrollTop = list.scrollHeight;
  logTimer = setInterval(poll, 2000);
}

async function renderTimeline(body) {
  const stats = await ensureStats();
  body.textContent = "";
  const monthly = (stats.monthly || [])
    .filter((m) => m.count > 0) // gap months add noise, not information
    .reverse(); // newest first
  if (!monthly.length) {
    body.append(el("div", "spnote", "No bookmarks yet."));
    return;
  }
  const max = Math.max(1, ...monthly.map((m) => m.count));
  for (const m of monthly) {
    body.append(
      statRow(
        "plain",
        [el("span", "slabel", monthName(m.month))],
        m.count,
        max,
        () => {
          clearFilters();
          state.month = m.month;
          setView("grid");
        },
      ),
    );
  }
}

/* ---- Lightbox ---- */
let lbIndex = 0;
let lbMediaIndex = 0;
function openLb(i) {
  lbIndex = i;
  lbMediaIndex = 0;
  resetFeed();
  const asFeed = NARROW.matches;
  $("#lb").classList.toggle("feed", asFeed);
  $("#lb").classList.remove("chrome");
  $("#lb").classList.add("open");
  document.body.style.overflow = "hidden";
  if (asFeed) openFeed(i);
  else fillLb();
}
function closeLb() {
  $("#lb").classList.remove("open");
  document.body.style.overflow = "";
  $("#lbmedia").textContent = ""; // stop any playing video
  resetFeed();
}
// Crossing the breakpoint with the lightbox open rebuilds it in the
// other mode, on the bookmark that was showing.
NARROW.addEventListener("change", () => {
  // The userscript alert appears and disappears across the breakpoint.
  if (lastStats) renderStatline(lastStats);
  if ($("#lb").classList.contains("open")) openLb(lbIndex);
});

/* ---- Mobile feed ----
   One pane per bookmark, snapped to the viewport, so the browser owns
   the scrolling physics. Panes carry only media: the author, text and
   actions stay in the single .lb-side, overlaid and refilled as the
   showing pane changes. Only the three panes around the reader hold
   real media — the rest are empty boxes of the right height. */
let feedPanes = [];
let feedObserver = null;
let feedLive = false;
const filledPanes = new Set();

function resetFeed() {
  cancelTap();
  if (ctlVideo)
    for (const name of CTL_EVENTS)
      ctlVideo.removeEventListener(name, syncControls);
  ctlVideo = null;
  userPaused = false;
  $("#lb").classList.remove("hasvideo", "paused");
  if (feedObserver) feedObserver.disconnect();
  feedObserver = null;
  feedLive = false;
  feedPanes = [];
  filledPanes.clear();
  const box = $("#lbfeed");
  box.textContent = "";
  box.style.scrollSnapType = ""; // a close can race growFeed's restore
  box.hidden = true;
}

function openFeed(i) {
  const box = $("#lbfeed");
  box.hidden = false;
  feedObserver = new IntersectionObserver(onFeedPane, {
    root: box,
    threshold: 0.6,
  });
  appendPanes(state.items.length);
  fillLbSide();
  windowPanes(i);
  // Every pane is exactly one viewport tall, so the offset is the index.
  // Jump before observing, or the panes we pass on the way report in and
  // drag lbIndex back up the feed.
  box.scrollTop = i * box.clientHeight;
  feedLive = true;
  for (const pane of feedPanes) feedObserver.observe(pane);
  extendFeed();
}

function appendPanes(count) {
  const box = $("#lbfeed");
  for (let k = 0; k < count; k++) {
    const j = feedPanes.length;
    const pane = el("div", "lb-pane");
    pane.dataset.i = String(j);
    // Bound to the pane, not its contents, which are torn down and
    // rebuilt as the reader moves.
    pane.addEventListener("click", onPaneTap);
    feedPanes.push(pane);
    box.append(pane);
    if (feedLive && feedObserver) feedObserver.observe(pane);
  }
}

function onFeedPane(entries) {
  if (!feedLive) return;
  for (const e of entries) {
    if (!e.isIntersecting) continue;
    const i = Number(e.target.dataset.i);
    if (i === lbIndex) continue;
    lbIndex = i;
    lbMediaIndex = 0;
    fillLbSide();
    windowPanes(i);
    extendFeed();
  }
}

// Stay a few panes ahead of the reader: the feed should not end where
// the grid happened to stop loading.
async function extendFeed() {
  while (feedLive && lbIndex >= feedPanes.length - 3) {
    // The grid's own infinite scroll keeps paging behind the feed, so
    // items may already be ahead of the panes; fetch only once every
    // loaded item has one.
    if (feedPanes.length >= state.items.length) {
      const added = await loadMoreItems();
      if (!added || !feedLive) return;
    }
    growFeed();
  }
}

// Appending to a mandatory-snap scroller makes iOS Safari re-snap, and
// it re-snaps to the FIRST pane — throwing the reader back to the top
// of the feed just as they reach the end of what was loaded. Lift the
// snapping around the append and hold the offset until layout settles.
function growFeed() {
  const box = $("#lbfeed");
  const at = box.scrollTop;
  box.style.scrollSnapType = "none";
  appendPanes(state.items.length - feedPanes.length);
  box.scrollTop = at;
  requestAnimationFrame(() => {
    box.scrollTop = at;
    box.style.scrollSnapType = "";
  });
}

function windowPanes(i) {
  for (const j of [...filledPanes]) if (Math.abs(j - i) > 1) clearPane(j);
  for (const j of [i - 1, i, i + 1]) fillPane(j);
  pausePane(i - 1);
  pausePane(i + 1);
  playPane(i);
  bindControls();
  cancelTap();
}

function fillPane(j) {
  const pane = feedPanes[j];
  const t = state.items[j];
  if (!pane || !t || filledPanes.has(j)) return;
  filledPanes.add(j);

  const media = t.media || [];
  if (!media.length) {
    const quote = el("div", "lb-quote");
    quote.append(linkify(t.full_text || "(no text)"));
    const slide = el("div", "lb-slide");
    slide.append(quote);
    pane.append(slide);
    return;
  }

  const track = el("div", "lb-track");
  for (const item of media) {
    const slide = el("div", "lb-slide");
    slide.append(lbMediaEl(item, true));
    track.append(slide);
  }
  pane.append(track);
  if (media.length < 2) return;

  const dots = el("div", "lb-dots");
  media.forEach((_, k) => dots.append(el("span", k ? "" : "on")));
  pane.append(dots);
  track.addEventListener(
    "scroll",
    () => {
      const at = slideIndex(track);
      for (const [k, dot] of [...dots.children].entries())
        dot.classList.toggle("on", k === at);
      if (j !== lbIndex) return;
      lbMediaIndex = at;
      pausePane(j);
      playPane(j);
      bindControls();
    },
    { passive: true },
  );
}

function clearPane(j) {
  const pane = feedPanes[j];
  if (!pane) return;
  filledPanes.delete(j);
  pane.textContent = ""; // drops the <video>, stopping its download
}

const slideIndex = (track) =>
  Math.round(track.scrollLeft / (track.clientWidth || 1));

function currentSlide(j) {
  const pane = feedPanes[j];
  if (!pane) return null;
  const track = pane.querySelector(".lb-track");
  if (!track) return pane.querySelector(".lb-slide");
  return track.children[slideIndex(track)] || track.firstElementChild;
}

// Muting is a choice about the room, not the video, so it follows the
// reader from pane to pane. Only the muted state carries: unmuting must
// not wake the sound a GIF-style video was born without.
let feedMuted = false;
// Only a pause the reader asked for is allowed to stick; every other
// stop is something to recover from. Cleared whenever the pane or
// slide changes, so scrolling away and back autoplays again.
let userPaused = false;

function playPane(j) {
  const slide = currentSlide(j);
  const v = slide && slide.querySelector("video");
  if (!v) return;
  userPaused = false;
  if (feedMuted) v.muted = true;
  playFeedVideo(v, j);
}

// Keeping the pane playing is a fight on two fronts. A play() torn up
// by the feed's own churn rejects with an abort, and retrying once the
// dust settles is all it takes — muting there would silence a video
// nobody blocked. Only a refusal by autoplay policy gets the muted
// fallback, marked so the next real touch can put the sound back.
function playFeedVideo(v, j) {
  v.play().catch((err) => {
    if (!feedLive || j !== lbIndex || userPaused || !v.paused) return;
    const slide = currentSlide(j);
    if (!slide || slide.querySelector("video") !== v) return;
    if (err && err.name === "NotAllowedError") {
      if (!v.muted) {
        v.muted = true;
        v.dataset.mutedByPolicy = "1";
      }
      v.play().catch(() => {});
    } else {
      setTimeout(() => {
        if (feedLive && j === lbIndex && !userPaused && v.paused)
          playFeedVideo(v, j);
      }, 150);
    }
  });
}

function pausePane(j) {
  const pane = feedPanes[j];
  if (!pane) return;
  for (const v of pane.querySelectorAll("video")) v.pause();
}

/* ---- Gesture priming ----
   iOS lets a video element start with sound only from a user gesture,
   and the swipe that brings the next pane is over by the time the
   observer reports it — so the programmatic play would arrive muted or
   not at all. Every touch on the feed is a gesture, though: use each
   one to run a silent play/pause over the neighbouring videos, which
   unlocks their elements for the with-sound play the next swipe needs,
   and to repair the showing video if policy paused or muted it. */
function primePane(j) {
  const pane = feedPanes[j];
  if (!pane) return;
  for (const v of pane.querySelectorAll("video")) {
    if (v.dataset.primed) continue;
    v.dataset.primed = "1";
    const keepMuted = v.muted;
    v.muted = true;
    const p = v.play();
    // Pause straight away, synchronously: the play() call inside the
    // gesture is what unlocks the element, and nothing must reach the
    // screen — a prime that lingered used to double-start the very
    // pane a swipe was bringing in.
    v.pause();
    if (p) p.catch(() => {});
    v.muted = keepMuted;
  }
}

// When a touch revives a stalled video, the stamp lets the tap that
// rode in on the same touch know its work is already done.
let gestureResumeAt = 0;

function onFeedGesture() {
  if (!feedLive) return;
  const v = ctlVideo;
  if (v && !userPaused) {
    if (v.dataset.mutedByPolicy && !feedMuted) {
      delete v.dataset.mutedByPolicy;
      v.muted = false;
    }
    if (v.paused) {
      gestureResumeAt = performance.now();
      v.play().catch(() => {});
    }
  }
  primePane(lbIndex - 1);
  primePane(lbIndex + 1);
}
$("#lbfeed").addEventListener("touchend", onFeedGesture, {
  passive: true,
});
$("#lbfeed").addEventListener("pointerup", onFeedGesture);

// The tweet itself is hidden by default so nothing sits on the media;
// the state carries across swipes until it is tapped away again.
function toggleChrome() {
  $("#lb").classList.toggle("chrome");
  bindControls();
}

/* ---- Tap and double tap ----
   Over a video, one tap in the middle plays or pauses and one nearer an
   edge shows the tweet; two jump the video — so the first tap has to
   sit out the double-tap window before it acts. Over a photo there is
   no second meaning, and it acts at once. */
const DOUBLE_TAP_MS = 260;
const SEEK_STEP = 10;
let tapTimer = null;
let jumpTimer = null;

function cancelTap() {
  clearTimeout(tapTimer);
  tapTimer = null;
}

function onPaneTap(e) {
  if (!ctlVideo) return toggleChrome();
  if (tapTimer) {
    cancelTap();
    seekBy(e.clientX < innerWidth / 2 ? -SEEK_STEP : SEEK_STEP);
    return;
  }
  // The touch under this tap may already have revived a stalled video
  // (onFeedGesture runs on pointerup, before the click lands); pausing
  // it again here would take two taps to get it back.
  const revived = performance.now() - gestureResumeAt < 500;
  const centered =
    Math.abs(e.clientX - innerWidth / 2) < innerWidth / 4 &&
    Math.abs(e.clientY - innerHeight / 2) < innerHeight / 4;
  tapTimer = setTimeout(() => {
    tapTimer = null;
    if (!centered) toggleChrome();
    else if (!revived) togglePlay();
  }, DOUBLE_TAP_MS);
}

function togglePlay() {
  const v = ctlVideo;
  if (!v) return;
  userPaused = !v.paused;
  if (v.paused) v.play().catch(() => {});
  else v.pause();
}

function seekBy(step) {
  const v = ctlVideo;
  if (!v || !Number.isFinite(v.duration) || v.duration <= 0) return;
  v.currentTime = Math.min(
    v.duration,
    Math.max(0, v.currentTime + step),
  );
  const jump = $("#lbjump");
  jump.className = "lb-jump on " + (step < 0 ? "back" : "fwd");
  jump.textContent =
    (step < 0 ? "« " : "") + Math.abs(step) + "s" + (step < 0 ? "" : " »");
  // Held, then faded — and a second jump restarts the hold.
  clearTimeout(jumpTimer);
  jumpTimer = setTimeout(() => jump.classList.remove("on"), 400);
}

/* ---- Feed video controls ----
   Native controls draw at the foot of the video, where the overlay is,
   so the feed carries its own play/pause, elapsed time and seek rail.
   One set, re-pointed at whichever video is showing. */
let ctlVideo = null;
const CTL_EVENTS = [
  "timeupdate",
  "play",
  "pause",
  "loadedmetadata",
  "volumechange",
];

function bindControls() {
  const slide = currentSlide(lbIndex);
  const v = (slide && slide.querySelector("video")) || null;
  if (v !== ctlVideo) {
    if (ctlVideo)
      for (const name of CTL_EVENTS)
        ctlVideo.removeEventListener(name, syncControls);
    ctlVideo = v;
    if (v)
      for (const name of CTL_EVENTS)
        v.addEventListener(name, syncControls);
  }
  syncControls();
}

// The control row's rail and the hairline at the bottom edge show the
// same thing; only one of them is ever on screen.
function paintProgress(frac) {
  const pct = 100 * Math.min(1, Math.max(0, frac || 0)) + "%";
  $("#lbfill").style.width = pct;
  $("#lbbarfill").style.width = pct;
}

function syncControls() {
  const v = ctlVideo;
  $("#lb").classList.toggle("hasvideo", !!v);
  // The badge marks a chosen pause; a video still loading or blocked
  // by policy is being retried, and flashing ▶ over it just reads as
  // the feed stalling.
  $("#lb").classList.toggle("paused", !!v && v.paused && userPaused);
  if (!v || seeking) return;
  const dur = v.duration || 0;
  const at = v.currentTime || 0;
  paintProgress(dur ? at / dur : 0);
  $("#lbat").textContent = fmtDuration(at * 1000);
  $("#lbdur").textContent = fmtDuration(dur * 1000);
  $("#lbplay").textContent = v.paused ? "▶" : "❚❚";
  $("#lbplay").setAttribute("aria-label", v.paused ? "Play" : "Pause");
  $("#lbmute").classList.toggle("muted", v.muted);
  $("#lbmute").setAttribute("aria-label", v.muted ? "Unmute" : "Mute");
}

// Dragging drives the rail directly and writes currentTime as it goes:
// waiting for the video to report back leaves the handle lagging the
// finger, and on a long file it barely moves at all.
let seeking = false;
function seekTo(e) {
  const v = ctlVideo;
  if (!v || !Number.isFinite(v.duration) || v.duration <= 0) return;
  const rail = $("#lbseek").getBoundingClientRect();
  const at = Math.min(
    1,
    Math.max(0, (e.clientX - rail.left) / (rail.width || 1)),
  );
  paintProgress(at);
  $("#lbat").textContent = fmtDuration(at * v.duration * 1000);
  v.currentTime = at * v.duration;
}
$("#lbseek").addEventListener("pointerdown", (e) => {
  if (!ctlVideo) return;
  e.preventDefault();
  seeking = true;
  $("#lb").classList.add("seeking");
  // Capture so the drag keeps seeking after the finger leaves the rail.
  try {
    $("#lbseek").setPointerCapture(e.pointerId);
  } catch {}
  seekTo(e);
});
$("#lbseek").addEventListener("pointermove", (e) => {
  if (seeking) seekTo(e);
});
for (const name of ["pointerup", "pointercancel"]) {
  $("#lbseek").addEventListener(name, () => {
    seeking = false;
    $("#lb").classList.remove("seeking");
  });
}
$("#lbplay").addEventListener("click", togglePlay);
$("#lbmute").addEventListener("click", () => {
  if (!ctlVideo) return;
  feedMuted = ctlVideo.muted = !ctlVideo.muted;
});
/* ---- Linkify URLs, @mentions, and #hashtags in tweet text ---- */
const LINKIFY_RE =
  /(https?:\/\/[^\s]+)|((?<![\w@])@[A-Za-z0-9_]{1,15})|((?<![\w#])#[\p{L}\p{N}_]+)/gu;
function linkify(text) {
  const frag = document.createDocumentFragment();
  let last = 0;
  for (const m of text.matchAll(LINKIFY_RE)) {
    let s = m[0];
    if (m[1]) {
      // Trailing punctuation belongs to the sentence, not the URL.
      const trimmed = s.replace(/[.,;:!?)'"\]]+$/, "");
      s = trimmed || s;
    }
    if (m.index > last) frag.append(text.slice(last, m.index));
    const a = document.createElement("a");
    a.target = "_blank";
    a.rel = "noopener";
    a.textContent = s;
    if (m[1]) a.href = s;
    else if (m[2]) a.href = "https://x.com/" + s.slice(1);
    else
      a.href = "https://x.com/hashtag/" + encodeURIComponent(s.slice(1));
    frag.append(a);
    last = m.index + s.length;
  }
  if (last < text.length) frag.append(text.slice(last));
  return frag;
}

function lbMediaEl(item, feed) {
  if (!item.file || item.missing) {
    return el(
      "div",
      "pending",
      (TYPE_LABELS[item.type] || item.type) +
        (item.duration_ms ? " · " + fmtDuration(item.duration_ms) : "") +
        (item.missing ? " · file missing" : " · not downloaded"),
    );
  }
  if (isMotion(item)) {
    const v = document.createElement("video");
    v.src = videoSrc(item);
    v.poster = posterSrc(item);
    v.preload = "metadata";
    v.playsInline = true;
    const gif = item.type === "animated_gif";
    if (feed) {
      // The feed plays whichever pane is showing and a tap pauses it,
      // so native controls would only collide with the overlay.
      v.loop = true;
      v.muted = gif;
    } else if (gif) {
      v.muted = true;
      v.loop = true;
      v.autoplay = true;
    } else {
      v.controls = true;
    }
    return v;
  }
  const img = document.createElement("img");
  img.src = mediaSrc(item);
  img.alt = "";
  return img;
}
function playLbVideo(container) {
  // Opening/navigating is a user gesture, so play-with-sound is allowed;
  // if the browser still blocks it, retry muted.
  const v = container.querySelector("video[controls]");
  if (!v) return;
  v.play().catch(() => {
    v.muted = true;
    v.play().catch(() => {});
  });
}
function stepLbMedia(delta) {
  const t = state.items[lbIndex];
  const media = (t && t.media) || [];
  const next = lbMediaIndex + delta;
  if (media.length < 2 || next < 0 || next >= media.length) return false;
  lbMediaIndex = next;
  renderLbMedia(t);
  return true;
}
function renderLbMedia(t) {
  const m = $("#lbmedia");
  m.textContent = "";
  const media = t.media || [];
  m.classList.toggle("carousel", media.length > 1);
  if (!media.length) {
    const quote = el("div", "lb-quote");
    quote.append(linkify(t.full_text || "(no text)"));
    m.append(quote);
    return;
  }
  if (media.length === 1) {
    m.append(lbMediaEl(media[0]));
    playLbVideo(m);
    return;
  }
  lbMediaIndex = Math.min(Math.max(lbMediaIndex, 0), media.length - 1);
  const stage = el("div", "lb-stage");
  stage.append(lbMediaEl(media[lbMediaIndex]));
  stage.append(
    el("div", "lb-count", lbMediaIndex + 1 + " / " + media.length),
  );
  const nav = (cls, label, delta) => {
    const b = el("button", "lb-mnav " + cls, label);
    b.type = "button";
    b.setAttribute(
      "aria-label",
      delta < 0 ? "Previous media" : "Next media",
    );
    b.disabled =
      delta < 0 ? lbMediaIndex <= 0 : lbMediaIndex >= media.length - 1;
    b.addEventListener("click", () => stepLbMedia(delta));
    return b;
  };
  stage.append(nav("prev", "‹", -1), nav("next", "›", 1));
  m.append(stage);
  const strip = el("div", "lb-strip");
  media.forEach((item, i) => {
    const b = el(
      "button",
      "lb-thumb" + (i === lbMediaIndex ? " active" : ""),
    );
    b.type = "button";
    b.setAttribute("aria-label", "Media " + (i + 1));
    if (item.file && !item.missing) {
      if (isMotion(item)) {
        const v = document.createElement("video");
        v.src = videoSrc(item);
        v.muted = true;
        v.playsInline = true;
        v.preload = "metadata";
        b.append(v);
      } else {
        const img = document.createElement("img");
        img.src = mediaSrc(item);
        img.alt = "";
        b.append(img);
      }
    } else {
      b.classList.add("pending");
      b.append(TYPE_LABELS[item.type] || item.type);
    }
    b.addEventListener("click", () => {
      if (i !== lbMediaIndex) {
        lbMediaIndex = i;
        renderLbMedia(t);
      }
    });
    strip.append(b);
  });
  m.append(strip);
  playLbVideo(stage);
}

function fillLb() {
  const t = state.items[lbIndex];
  if (!t) return;
  renderLbMedia(t);
  fillLbSide();
}
// Everything about the tweet that is not its media. The feed shows the
// same element as an overlay, moving it to whichever pane is on screen.
function fillLbSide() {
  const t = state.items[lbIndex];
  if (!t) return;
  const av = avatarEl(t);
  av.id = "lbavatar";
  $("#lbavatar").replaceWith(av);
  $("#lbname").textContent = t.name || t.screen_name || "unknown";
  $("#lbhandle").textContent = "@" + (t.screen_name || "unknown");
  $("#lbfilter").hidden = !t.screen_name;
  if (t.screen_name)
    $("#lbfilter").textContent = "More from @" + t.screen_name;
  const lbtext = $("#lbtext");
  lbtext.textContent = "";
  lbtext.append(linkify(t.full_text || ""));
  $("#lbdate").textContent = fmtDate(t.created_at) + " ↗";
  $("#lbdate").href = t.permanent_url || "#";
  $("#lbreveal").hidden = !(t.media || []).some(
    (item) => item.file && !item.missing,
  );
  hideConfirm();
  $("#lbprev").disabled = lbIndex <= 0;
  // Not the end of the archive while there are pages left to fetch.
  $("#lbnext").disabled =
    lbIndex >= state.items.length - 1 && $("#more").hidden;
}
$("#lbclose").addEventListener("click", closeLb);
$("#lb").addEventListener("click", (e) => {
  if (e.target === e.currentTarget) closeLb();
});
$("#lbprev").addEventListener("click", () => {
  if (lbIndex > 0) {
    lbIndex--;
    lbMediaIndex = 0;
    fillLb();
  }
});
$("#lbnext").addEventListener("click", async () => {
  if (lbIndex >= state.items.length - 1) await loadMoreItems();
  if (lbIndex < state.items.length - 1) {
    lbIndex++;
    lbMediaIndex = 0;
    fillLb();
  }
});
addEventListener("keydown", (e) => {
  if (!$("#lb").classList.contains("open")) return;
  if (e.key === "Escape") return closeLb();
  // The feed scrolls itself; only the panel has arrows to drive.
  if (NARROW.matches) return;
  // Arrows step through a tweet's own media first, then move on to the
  // previous/next tweet.
  if (e.key === "ArrowLeft" && !stepLbMedia(-1)) $("#lbprev").click();
  if (e.key === "ArrowRight" && !stepLbMedia(1)) $("#lbnext").click();
});
function filterAuthor(handle) {
  state.author = handle;
  closeLb();
  resetAndLoad();
  scrollTo({ top: 0, behavior: "smooth" });
}
for (const sel of ["#lbname", "#lbhandle"]) {
  $(sel).addEventListener("click", () => {
    const t = state.items[lbIndex];
    if (t && t.screen_name) filterAuthor(t.screen_name);
  });
}
$("#lbfilter").addEventListener("click", () => {
  const t = state.items[lbIndex];
  if (t && t.screen_name) filterAuthor(t.screen_name);
});

/* ---- Remove bookmark (optionally with its files) ---- */
function decrementCounts(t) {
  const c = state.counts;
  if (!c) return;
  const dec = (key) => {
    if (c[key] != null && c[key] > 0) c[key]--;
  };
  dec("all");
  const kinds = new Set((t.media || []).map((m) => m.type));
  if (!kinds.size) dec("text");
  if (kinds.size === 1 && kinds.has("photo")) dec("photo");
  if (kinds.has("video")) dec("video");
  if (kinds.has("animated_gif")) dec("gif");
  if ((t.media || []).some((m) => m.missing)) dec("missing");
}
function showConfirm() {
  const t = state.items[lbIndex];
  if (!t) return;
  const hasFiles = (t.media || []).some(
    (item) => item.file && !item.missing,
  );
  $("#lbdelfiles").hidden = !hasFiles;
  $("#lbkeep").textContent = hasFiles ? "Just the bookmark" : "Remove";
  $("#lbactions").hidden = true;
  $("#lbconfirm").hidden = false;
}
function hideConfirm() {
  $("#lbconfirm").hidden = true;
  $("#lbactions").hidden = false;
}
async function removeTweet(deleteFiles) {
  const t = state.items[lbIndex];
  if (!t) return;
  let res;
  try {
    res = await fetch("/api/explore/tweets/delete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: t.id,
        account: state.account,
        delete_files: deleteFiles,
      }),
    });
  } catch {
    res = null;
  }
  if (!res || !res.ok) {
    alert("Failed to remove the bookmark — is the local app running?");
    return;
  }
  // Remove the card in place: no refetch, no masonry reflow.
  const node = cardNodes[lbIndex];
  if (node) node.remove();
  state.items.splice(lbIndex, 1);
  cardNodes.splice(lbIndex, 1);
  state.total = Math.max(0, state.total - 1);
  decrementCounts(t);
  updateCount();
  if (state.type === "missing") resetFixAll(!state.items.length);
  renderTabs();
  closeLb();
}
$("#lbreveal").addEventListener("click", async () => {
  const t = state.items[lbIndex];
  const item = ((t && t.media) || []).find((m) => m.file && !m.missing);
  if (!item) return;
  let res;
  try {
    res = await fetch("/api/explore/reveal", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        file: item.file,
        account: item.account_id || state.account,
      }),
    });
  } catch {
    res = null;
  }
  if (!res || !res.ok)
    alert("Could not reveal the file — is the local app running?");
});
$("#lbremove").addEventListener("click", showConfirm);
$("#lbcancel").addEventListener("click", hideConfirm);
$("#lbkeep").addEventListener("click", () => removeTweet(false));
$("#lbdelfiles").addEventListener("click", () => removeTweet(true));

/* ---- Download progress strip ---- */
const fmtBytes = (n) => {
  if (n < 1024) return n + " B";
  const units = ["KB", "MB", "GB"];
  let u = -1;
  do {
    n /= 1024;
    u++;
  } while (n >= 1024 && u < units.length - 1);
  return n.toFixed(1) + " " + units[u];
};
let dlWasBusy = false;
let dlTimer = null;
// Parse time counts as checked: boot fetches setup itself.
let setupCheckedAt = Date.now();
async function pollDownloads() {
  let data = null;
  try {
    const res = await fetch("/api/explore/downloads");
    if (res.ok) data = await res.json();
  } catch {
    data = null;
  }
  const busy =
    !!data && (data.pending > 0 || (data.active || []).length > 0);
  $("#dlstrip").hidden = !busy;
  if (busy) {
    const target = data.downloaded + data.pending;
    const pct = target ? Math.floor((100 * data.downloaded) / target) : 0;
    $("#dltext").textContent =
      "Downloading media — " +
      fmt(data.downloaded) +
      " of " +
      fmt(target) +
      (data.failed > 0 ? " · " + fmt(data.failed) + " failed" : "");
    $("#dlpct").textContent = pct + "%";
    $("#dlbar").style.width = pct + "%";
    const files = $("#dlfiles");
    files.textContent = "";
    for (const f of data.active || []) {
      const row = el("div", "dlfile");
      row.append(el("span", "fname", f.file));
      const track = el("span", "ftrack");
      const bar = el("span", "fbar");
      const fp =
        f.total > 0 ? Math.floor((100 * f.written) / f.total) : 0;
      bar.style.width = (f.total > 0 ? fp : 100) + "%";
      track.append(bar);
      row.append(track);
      row.append(
        el("span", "fpct", f.total > 0 ? fp + "%" : fmtBytes(f.written)),
      );
      files.append(row);
    }
  } else if (dlWasBusy) {
    // Queue just drained — refresh the header stats.
    statsCache = null;
    loadStats().catch(() => {});
  }
  // Setup status changes outside this tab — the script gets installed or
  // updated on x.com, or the app restarts with a newer bundled script.
  // Re-check on a slow cadence either way: a chip must appear the moment
  // the script falls behind, not only clear once it catches up.
  if (Date.now() - setupCheckedAt > 15000) {
    setupCheckedAt = Date.now();
    const before = setupAlert();
    try {
      await loadSetup(true);
      if (setupAlert() !== before && lastStats)
        renderStatline(lastStats);
    } catch {}
  }
  dlWasBusy = busy;
  clearTimeout(dlTimer);
  dlTimer = setTimeout(pollDownloads, busy ? 2000 : 15000);
}

/* ---- Controls ---- */
$("#home").addEventListener("click", goHome);
let debounceTimer = null;
$("#q").addEventListener("input", (e) => {
  clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    state.q = e.target.value.trim();
    resetAndLoad();
  }, 300);
});
$("#sort").addEventListener("change", (e) => {
  state.sort = e.target.value;
  resetAndLoad();
});
$("#chip button").addEventListener("click", () => {
  state.author = "";
  resetAndLoad();
});
$("#mchip button").addEventListener("click", () => {
  state.month = "";
  resetAndLoad();
});

/* ---- Boot ---- */
let viewFromURL = false;
(function initFromURL() {
  const p = new URLSearchParams(location.search);
  const view = p.get("view");
  if (
    [
      "authors",
      "media",
      "timeline",
      "activity",
      "settings",
      "setup",
    ].includes(view)
  ) {
    state.view = view;
    viewFromURL = true;
  }
  state.account = (p.get("account") || "").trim();
  state.q = (p.get("q") || "").trim();
  const type = p.get("type") || "all";
  if (KIND_TABS.some(([key]) => key === type)) state.type = type;
  state.author = (p.get("author") || "").trim();
  if (/^\d{4}-\d{2}$/.test(p.get("month") || ""))
    state.month = p.get("month");
  const sort = p.get("sort");
  if (
    [
      "oldest",
      "tweet-newest",
      "tweet-oldest",
      "longest",
      "shortest",
    ].includes(sort)
  )
    state.sort = sort;
  $("#q").value = state.q;
  $("#sort").value = state.sort;
})();
$("#acct").addEventListener("change", (e) => {
  fitAccountSelect();
  switchAccount(e.target.value);
});

(async function boot() {
  try {
    const [data, setup] = await Promise.all([
      loadAccounts(),
      loadSetup(),
    ]);
    const items = data.items || [];
    if (!items.some((a) => a.id === state.account))
      state.account = data.default || "";
    renderAccountSwitcher(data);
    // An empty archive means nothing is syncing yet, and an empty grid
    // explains none of it — open the tutorial instead.
    if (!viewFromURL && setup.bookmarks === 0) state.view = "setup";
  } catch {
    // The dashboard still works against a single account without this.
  }

  renderTabs();
  loadStats().catch(() => {
    $("#statline").textContent =
      "Could not load stats — is the local app running?";
  });
  applyView();
  if (state.view === "grid") resetAndLoad();
  else renderSubpage();
  pollDownloads();
})();
