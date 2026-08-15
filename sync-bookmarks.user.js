// ==UserScript==
// @name         Twitter Bookmarks Sync to Local
// @namespace    http://tampermonkey.net/
// @version      0.9
// @description  Intercept XHR to sync bookmarks and provide Auto-Scroll feature.
// @author       TBD
// @match        https://x.com/*
// @match        https://twitter.com/*
// @run-at       document-start
// @grant        unsafeWindow
// @grant        GM_xmlhttpRequest
// @connect      localhost
// ==/UserScript==

;(function () {
  'use strict'
  const RAW_SYNC_URL = 'http://localhost:41008/api/sync-raw'
  const SETTINGS_URL = 'http://localhost:41008/api/settings'
  const AUTO_STOP_DUPLICATE_BATCHES = 3
  const PAGE_CHECK_INTERVAL_MS = 500
  const SCROLL_INTERVAL_MS = 5000
  const SCROLL_NUDGE_MS = 1500

  console.log('[TBD v0.9] Overlay loaded.')

  const THEMES = {
    light: {
      bg: '#ffffff',
      text: '#0f1419',
      sub: '#536471',
      border: '#cfd9de',
      trackOff: '#cfd9de',
      shadow: '0 8px 24px rgba(15, 20, 25, 0.16)',
      tones: { neutral: '#536471', active: '#1d9bf0', success: '#008a00', warning: '#b45f00', danger: '#b00020' },
    },
    dark: {
      bg: '#16181c',
      text: '#e7e9ea',
      sub: '#71767b',
      border: '#2f3336',
      trackOff: '#3e4144',
      shadow: '0 8px 24px rgba(0, 0, 0, 0.45)',
      tones: { neutral: '#8b98a5', active: '#1d9bf0', success: '#00ba7c', warning: '#f7b955', danger: '#f66570' },
    },
  }

  const UI = {
    el: null,
    btn: null,
    titleEl: null,
    statusEl: null,
    statusDot: null,
    rowsEl: null,
    rowLabels: [],
    forceSwitch: null,
    videoSwitch: null,
    imageSwitch: null,
    theme: 'light',
    statusText: 'Ready',
    statusTone: 'neutral',
    timeout: null,
    isForce: false,
    settings: {
      media_dir: 'media',
      download_videos: true,
      download_images: true,
    },

    createSwitchRow(labelText, title, onToggle) {
      const row = document.createElement('div')
      row.title = title
      row.style.cssText = `
                display: flex;
                align-items: center;
                justify-content: space-between;
                gap: 8px;
                cursor: pointer;
            `

      const label = document.createElement('span')
      label.innerText = labelText
      label.style.cssText = 'font-size: 13px; font-weight: 500;'

      const track = document.createElement('button')
      track.type = 'button'
      track.setAttribute('role', 'switch')
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
            `

      const knob = document.createElement('span')
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
            `
      track.appendChild(knob)

      row.appendChild(label)
      row.appendChild(track)
      row.onclick = onToggle

      this.rowLabels.push(label)
      return { row, track, knob }
    },

    setSwitch(sw, on) {
      sw.track.setAttribute('aria-checked', on ? 'true' : 'false')
      sw.track.style.background = on ? '#1d9bf0' : THEMES[this.theme].trackOff
      sw.knob.style.transform = on ? 'translateX(16px)' : 'translateX(0)'
    },

    init() {
      const theme = THEMES[this.theme]

      this.el = document.createElement('div')
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
            `

      const header = document.createElement('div')
      header.style.cssText = 'display: flex; align-items: center; justify-content: space-between; gap: 8px;'

      this.titleEl = document.createElement('div')
      this.titleEl.innerText = 'TBD'
      this.titleEl.style.cssText = 'font-size: 14px; font-weight: 800; letter-spacing: 0.2px;'

      const statusWrap = document.createElement('div')
      statusWrap.style.cssText = 'display: flex; align-items: center; gap: 6px; min-width: 0;'

      this.statusDot = document.createElement('span')
      this.statusDot.style.cssText = 'width: 8px; height: 8px; border-radius: 50%; flex: none;'

      this.statusEl = document.createElement('div')
      this.statusEl.style.cssText =
        'font-size: 12px; font-weight: 600; max-width: 130px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;'

      statusWrap.appendChild(this.statusDot)
      statusWrap.appendChild(this.statusEl)
      header.appendChild(this.titleEl)
      header.appendChild(statusWrap)

      this.btn = document.createElement('button')
      this.btn.type = 'button'
      this.btn.innerText = 'Start sync'
      this.btn.style.cssText = `
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
            `
      this.btn.onmouseenter = () => (this.btn.style.filter = 'brightness(0.92)')
      this.btn.onmouseleave = () => (this.btn.style.filter = '')
      this.btn.onclick = () => Scroller.toggle()

      this.rowsEl = document.createElement('div')
      this.rowsEl.style.cssText = `
                display: flex;
                flex-direction: column;
                gap: 10px;
                border-top: 1px solid ${theme.border};
                padding-top: 12px;
            `

      this.forceSwitch = this.createSwitchRow(
        'Auto-stop',
        'Stop syncing after several already-saved bookmarks in a row. Turn it off for a deeper re-scan.',
        () => this.toggleForce()
      )
      this.videoSwitch = this.createSwitchRow(
        'Download videos',
        'Save this as the download_videos setting. When off, video media stays queued but is not downloaded.',
        () => this.toggleVideos()
      )
      this.imageSwitch = this.createSwitchRow(
        'Download images',
        'Save this as the download_images setting. When off, image media stays queued but is not downloaded.',
        () => this.toggleImages()
      )

      this.rowsEl.appendChild(this.forceSwitch.row)
      this.rowsEl.appendChild(this.videoSwitch.row)
      this.rowsEl.appendChild(this.imageSwitch.row)

      this.el.appendChild(header)
      this.el.appendChild(this.btn)
      this.el.appendChild(this.rowsEl)
      this.updateModeButton()
      this.updateSettingsButtons()
      this.renderStatus()
      this.loadSettings()

      // Check the page state on a slow interval instead of every animation frame,
      // and only touch the DOM when something actually changed.
      let visible = null
      const tick = () => {
        if (!document.body) return
        if (!this.el.parentElement) document.body.appendChild(this.el)

        const onBookmarksPage =
          window.location.pathname.includes('/i/history') || window.location.pathname.includes('/i/bookmarks')

        if (onBookmarksPage !== visible) {
          visible = onBookmarksPage
          this.el.style.display = onBookmarksPage ? 'flex' : 'none'
        }
        if (!onBookmarksPage && Scroller.active) Scroller.stop()
        if (onBookmarksPage) this.applyTheme(this.detectTheme())
      }
      tick()
      setInterval(tick, PAGE_CHECK_INTERVAL_MS)
    },

    detectTheme() {
      try {
        const bg = getComputedStyle(document.body).backgroundColor
        const parts = bg.match(/[\d.]+/g)
        if (parts && parts.length >= 3 && (parts.length < 4 || Number(parts[3]) > 0)) {
          const luminance = 0.299 * parts[0] + 0.587 * parts[1] + 0.114 * parts[2]
          return luminance < 128 ? 'dark' : 'light'
        }
      } catch (e) {}
      return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
    },

    applyTheme(name) {
      if (name === this.theme || !THEMES[name]) return
      this.theme = name
      const theme = THEMES[name]

      this.el.style.background = theme.bg
      this.el.style.color = theme.text
      this.el.style.boxShadow = theme.shadow
      this.el.style.borderColor = Scroller.active ? '#1d9bf0' : theme.border
      this.rowsEl.style.borderTopColor = theme.border
      this.titleEl.style.color = theme.text
      for (const label of this.rowLabels) label.style.color = theme.text

      this.updateModeButton()
      this.updateSettingsButtons()
      this.renderStatus()
    },

    renderStatus() {
      const tones = THEMES[this.theme].tones
      const color = tones[this.statusTone] || tones.neutral
      this.statusEl.innerText = this.statusText
      this.statusEl.style.color = color
      this.statusDot.style.background = color
    },

    updateStatus(text, tone = 'neutral', autoReset = false) {
      this.statusText = text
      this.statusTone = tone
      this.renderStatus()

      if (this.timeout) clearTimeout(this.timeout)
      if (autoReset) {
        this.timeout = setTimeout(() => this.resetStatus(), 2200)
      }
    },

    resetStatus() {
      if (Scroller.active) {
        this.updateStatus(this.isForce ? 'Syncing - auto-stop off' : 'Syncing - auto-stop on', 'active')
      } else {
        this.updateStatus('Ready')
      }
    },

    toggleForce() {
      this.isForce = !this.isForce
      this.updateModeButton()
      if (Scroller.active) {
        this.resetStatus()
      } else {
        this.updateStatus(this.isForce ? 'Auto-stop off' : 'Auto-stop on', 'neutral', true)
      }
    },

    setScrolling(isScrolling) {
      if (isScrolling) {
        this.btn.innerText = 'Stop sync'
        this.btn.style.background = '#f4212e'
        this.el.style.borderColor = '#1d9bf0'
        this.resetStatus()
      } else {
        this.btn.innerText = 'Start sync'
        this.btn.style.background = '#1d9bf0'
        this.el.style.borderColor = THEMES[this.theme].border
        this.updateStatus('Stopped')
      }
    },

    isForceMode() {
      return this.isForce
    },

    loadSettings() {
      GM_xmlhttpRequest({
        method: 'GET',
        url: SETTINGS_URL,
        onload: (response) => {
          try {
            this.applySettings(JSON.parse(response.responseText))
          } catch (e) {
            this.updateStatus('Settings error', 'danger', true)
          }
        },
        onerror: () => {
          this.updateStatus('Local app offline', 'danger')
        },
      })
    },

    applySettings(settings) {
      this.settings = {
        media_dir: settings.media_dir || 'media',
        download_videos: settings.download_videos !== false,
        download_images: settings.download_images !== false,
      }
      this.updateSettingsButtons()
    },

    updateModeButton() {
      if (!this.forceSwitch) return
      // The switch shows whether auto-stop is enabled (force mode = auto-stop off).
      this.setSwitch(this.forceSwitch, !this.isForce)
    },

    updateSettingsButtons() {
      if (!this.videoSwitch || !this.imageSwitch) return
      this.setSwitch(this.videoSwitch, this.settings.download_videos)
      this.setSwitch(this.imageSwitch, this.settings.download_images)
    },

    saveSettings(next, successText) {
      GM_xmlhttpRequest({
        method: 'POST',
        url: SETTINGS_URL,
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify(next),
        onload: (response) => {
          try {
            this.applySettings(JSON.parse(response.responseText))
            this.updateStatus(successText, 'neutral', true)
          } catch (e) {
            this.updateStatus('Settings error', 'danger', true)
          }
        },
        onerror: () => {
          this.updateStatus('Could not save', 'danger', true)
        },
      })
    },

    toggleVideos() {
      const next = {
        media_dir: this.settings.media_dir,
        download_videos: !this.settings.download_videos,
        download_images: this.settings.download_images !== false,
      }

      this.saveSettings(next, next.download_videos ? 'Video downloads on' : 'Video downloads off')
    },

    toggleImages() {
      const next = {
        media_dir: this.settings.media_dir,
        download_videos: this.settings.download_videos !== false,
        download_images: !this.settings.download_images,
      }

      this.saveSettings(next, next.download_images ? 'Image downloads on' : 'Image downloads off')
    },
  }

  const Scroller = {
    active: false,
    timer: null,
    duplicateOnlyBatches: 0,

    toggle() {
      if (this.active) this.stop()
      else this.start()
    },

    start() {
      if (this.active) return
      this.active = true
      this.duplicateOnlyBatches = 0
      UI.setScrolling(true)
      this.loop()
    },

    stop() {
      if (!this.active) return
      this.active = false
      this.duplicateOnlyBatches = 0
      clearTimeout(this.timer)
      UI.setScrolling(false)
    },

    resetDuplicateOnlyBatches() {
      this.duplicateOnlyBatches = 0
    },

    noteDuplicateOnlyBatch() {
      this.duplicateOnlyBatches += 1
      return this.duplicateOnlyBatches
    },

    // A batch just got processed, so the next content is likely already rendered:
    // scroll again soon instead of waiting out the full interval.
    nudge() {
      if (!this.active) return
      clearTimeout(this.timer)
      this.timer = setTimeout(() => this.loop(), SCROLL_NUDGE_MS)
    },

    loop() {
      if (!this.active) return
      window.scrollTo(0, document.body.scrollHeight)
      this.timer = setTimeout(() => {
        this.loop()
      }, SCROLL_INTERVAL_MS)
    },
  }

  const PageXHR = unsafeWindow.XMLHttpRequest
  const originalOpen = PageXHR.prototype.open
  const originalSend = PageXHR.prototype.send

  PageXHR.prototype.open = function (method, url) {
    try {
      this._tbd_url = url
    } catch (e) {}
    return originalOpen.apply(this, arguments)
  }

  PageXHR.prototype.send = function () {
    const self = this
    const onLoad = function () {
      self.removeEventListener('load', onLoad)
      const url = self._tbd_url

      if (typeof url === 'string' && url.includes('Bookmarks') && url.includes('graphql')) {
        const responseData = self.responseText
        if (responseData) {
          GM_xmlhttpRequest({
            method: 'POST',
            url: RAW_SYNC_URL,
            headers: { 'Content-Type': 'application/json' },
            data: responseData,
            onload: function (response) {
              try {
                const res = JSON.parse(response.responseText)
                const savedCount = Number(res.saved_count) || 0

                if (savedCount > 0) {
                  Scroller.resetDuplicateOnlyBatches()
                }

                if (res.duplicate_limit_reached) {
                  if (!UI.isForceMode()) {
                    if (savedCount > 0) {
                      UI.updateStatus(`Saved ${savedCount} new`, 'success', true)
                    } else {
                      const duplicateBatches = Scroller.noteDuplicateOnlyBatch()
                      if (duplicateBatches >= AUTO_STOP_DUPLICATE_BATCHES) {
                        Scroller.stop()
                        UI.updateStatus('Stopped at repeats', 'warning')
                      } else {
                        UI.updateStatus(`Checking deeper ${duplicateBatches}/${AUTO_STOP_DUPLICATE_BATCHES}`, 'warning', true)
                      }
                    }
                  } else {
                    UI.updateStatus(`Saved ${savedCount} new`, 'success', true)
                    console.log(`[TBD] Limit hit (Force). Saved: ${savedCount}`)
                  }
                } else {
                  UI.updateStatus(
                    `Saved ${savedCount} new`,
                    savedCount > 0 ? 'success' : 'neutral',
                    true
                  )
                }

                // No-op if the scroller just stopped or was never running.
                Scroller.nudge()
              } catch (e) {
                UI.updateStatus('Server error', 'danger', true)
              }
            },
            onerror: function (err) {
              UI.updateStatus('Local app offline', 'danger')
            },
          })
        }
      }
    }
    self.addEventListener('load', onLoad)
    return originalSend.apply(this, arguments)
  }

  UI.init()
})()
