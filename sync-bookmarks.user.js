// ==UserScript==
// @name         Twitter Bookmarks Sync to Local
// @namespace    http://tampermonkey.net/
// @version      0.7
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

  console.log('[TBD v0.7] Overlay loaded.')

  const UI = {
    el: null,
    btn: null,
    statusEl: null,
    forceBtn: null,
    videoBtn: null,
    imageBtn: null,
    timeout: null,
    isForce: false,
    settings: {
      media_dir: 'media',
      download_videos: true,
      download_images: true,
    },

    createButton(text) {
      const button = document.createElement('button')
      button.type = 'button'
      button.innerText = text
      button.style.cssText = `
                height: 34px;
                border: 1px solid #cfd9de;
                border-radius: 8px;
                background: #eff3f4;
                color: #0f1419;
                cursor: pointer;
                font: inherit;
                font-weight: 650;
                padding: 0 10px;
                white-space: nowrap;
                width: 100%;
            `
      return button
    },

    init() {
      this.el = document.createElement('div')
      this.el.style.cssText = `
                position: fixed;
                bottom: 24px;
                left: 20px;
                width: 220px;
                background: #ffffff;
                color: #0f1419;
                padding: 12px;
                border-radius: 8px;
                font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
                font-size: 13px;
                z-index: 999999;
                border: 1px solid #cfd9de;
                box-shadow: 0 8px 24px rgba(15, 20, 25, 0.16);
                display: flex;
                flex-direction: column;
                gap: 10px;
                user-select: none;
                letter-spacing: 0;
            `

      const header = document.createElement('div')
      header.style.cssText = 'display: flex; align-items: center; justify-content: space-between;'

      const title = document.createElement('div')
      title.innerText = 'TBD'
      title.style.cssText = 'font-size: 13px; font-weight: 750;'

      this.statusEl = document.createElement('div')
      this.statusEl.innerText = 'Ready'
      this.statusEl.style.cssText =
        'color: #536471; font-size: 12px; font-weight: 600; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: right;'

      header.appendChild(title)
      header.appendChild(this.statusEl)

      this.btn = this.createButton('Start sync')
      this.btn.style.width = '100%'
      this.btn.style.background = '#1d9bf0'
      this.btn.style.borderColor = '#1d9bf0'
      this.btn.style.color = '#ffffff'
      this.btn.onclick = () => Scroller.toggle()

      const settingsControls = document.createElement('div')
      settingsControls.style.cssText = 'display: flex; flex-direction: column; gap: 8px;'

      this.forceBtn = this.createButton('')
      this.forceBtn.title = 'Auto-stop stops syncing after several already-saved bookmarks in a row. Turn it off for a deeper re-scan.'
      this.forceBtn.onclick = () => this.toggleForce()

      this.videoBtn = this.createButton('')
      this.videoBtn.title = 'Save this as the download_videos setting. When off, video media stays queued but is not downloaded.'
      this.videoBtn.onclick = () => this.toggleVideos()

      this.imageBtn = this.createButton('')
      this.imageBtn.title = 'Save this as the download_images setting. When off, image media stays queued but is not downloaded.'
      this.imageBtn.onclick = () => this.toggleImages()

      settingsControls.appendChild(this.forceBtn)
      settingsControls.appendChild(this.videoBtn)
      settingsControls.appendChild(this.imageBtn)

      this.el.appendChild(header)
      this.el.appendChild(this.btn)
      this.el.appendChild(settingsControls)
      this.updateModeButton()
      this.updateSettingsButtons()
      this.loadSettings()

      const monitor = () => {
        if (!document.body) {
          requestAnimationFrame(monitor)
          return
        }
        if (!this.el.parentElement) document.body.appendChild(this.el)

        if (window.location.pathname.includes('/i/bookmarks')) {
          this.el.style.display = 'flex'
        } else {
          this.el.style.display = 'none'
          if (Scroller.active) Scroller.stop()
        }
        requestAnimationFrame(monitor)
      }
      monitor()
    },

    updateStatus(text, tone = 'neutral', autoReset = false) {
      const colors = {
        neutral: '#536471',
        active: '#1d9bf0',
        success: '#008a00',
        warning: '#b45f00',
        danger: '#b00020',
      }

      this.statusEl.innerText = text
      this.statusEl.style.color = colors[tone] || colors.neutral

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
        this.btn.style.borderColor = '#f4212e'
        this.el.style.borderColor = '#1d9bf0'
        this.resetStatus()
      } else {
        this.btn.innerText = 'Start sync'
        this.btn.style.background = '#1d9bf0'
        this.btn.style.borderColor = '#1d9bf0'
        this.el.style.borderColor = '#cfd9de'
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
      if (!this.forceBtn) return

      this.forceBtn.innerText = this.isForce ? 'Auto-stop off' : 'Auto-stop on'
      this.forceBtn.setAttribute('aria-pressed', this.isForce ? 'true' : 'false')
      this.forceBtn.style.background = this.isForce ? '#fff4e5' : '#eff3f4'
      this.forceBtn.style.borderColor = this.isForce ? '#f4a62a' : '#cfd9de'
      this.forceBtn.style.color = '#0f1419'
    },

    updateSettingsButtons() {
      if (!this.videoBtn || !this.imageBtn) return

      this.updateDownloadButton(this.videoBtn, 'Download videos', this.settings.download_videos)
      this.updateDownloadButton(this.imageBtn, 'Download images', this.settings.download_images)
    },

    updateDownloadButton(button, label, enabled) {
      button.innerText = `${label}: ${enabled ? 'on' : 'off'}`
      button.setAttribute('aria-pressed', enabled ? 'true' : 'false')
      button.style.background = enabled ? '#e6f4ea' : '#eff3f4'
      button.style.borderColor = enabled ? '#79c083' : '#cfd9de'
      button.style.color = '#0f1419'
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

    loop() {
      if (!this.active) return
      window.scrollTo(0, document.body.scrollHeight)
      this.timer = setTimeout(() => {
        this.loop()
      }, 5000)
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
