// ==UserScript==
// @name         Twitter Bookmarks Sync to Local
// @namespace    http://tampermonkey.net/
// @version      0.3
// @description  Intercept XHR to sync bookmarks and provide Auto-Scroll feature.
// @author       Gemini
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

  console.log('[TBD v0.3] Terminal UI Edition loaded.')

  const UI = {
    el: null,
    btn: null,
    statusEl: null,
    forceBtn: null,
    imageBtn: null,
    timeout: null,
    isForce: false,
    settings: {
      media_dir: 'media',
      download_videos: true,
      download_images: true,
    },

    init() {
      this.el = document.createElement('div')
      this.el.style.cssText = `
                position: fixed;
                bottom: 80px;
                left: 20px;
                background: #000;
                color: #0f0;
                padding: 10px 14px;
                border-radius: 4px;
                font-family: "Consolas", "Monaco", "Courier New", monospace;
                font-size: 12px;
                z-index: 999999;
                border: 1px solid #333;
                box-shadow: 0 0 10px rgba(0, 255, 0, 0.1);
                display: flex;
                flex-direction: column;
                gap: 8px;
                min-width: 130px;
                user-select: none;
                letter-spacing: 0.5px;
            `

      const controls = document.createElement('div')
      controls.style.cssText =
        'display: flex; justify-content: space-between; align-items: center; gap: 12px;'

      this.btn = document.createElement('div')
      this.btn.innerText = '[RUN]'
      this.btn.style.cssText =
        'cursor: pointer; font-weight: bold; color: #0f0; text-shadow: 0 0 2px rgba(0,255,0,0.5);'
      this.btn.onclick = () => Scroller.toggle()

      this.forceBtn = document.createElement('div')
      this.forceBtn.innerText = '[FORCE:OFF]'
      this.forceBtn.style.cssText = 'cursor: pointer; color: #666; font-size: 10px;'
      this.forceBtn.title = 'Toggle Force Mode'
      this.forceBtn.onclick = () => this.toggleForce()

      controls.appendChild(this.btn)
      controls.appendChild(this.forceBtn)

      const settingsControls = document.createElement('div')
      settingsControls.style.cssText =
        'display: flex; justify-content: flex-start; align-items: center; gap: 12px;'

      this.imageBtn = document.createElement('div')
      this.imageBtn.innerText = '[IMG:ON]'
      this.imageBtn.style.cssText =
        'cursor: pointer; color: #ff9f00; font-size: 10px; text-shadow: 0 0 2px #ff9f00;'
      this.imageBtn.title = 'Toggle image downloads'
      this.imageBtn.onclick = () => this.toggleImages()
      settingsControls.appendChild(this.imageBtn)

      this.statusEl = document.createElement('div')
      this.statusEl.innerText = '> SYSTEM READY'
      this.statusEl.style.cssText =
        'color: #0f0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 150px; border-top: 1px dashed #333; padding-top: 6px; font-size: 11px;'

      this.el.appendChild(controls)
      this.el.appendChild(settingsControls)
      this.el.appendChild(this.statusEl)
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

    updateStatus(text, color = '#0f0') {
      this.statusEl.innerText = `> ${text}`
      this.statusEl.style.color = color

      if (color !== '#0f0' && color !== '#888') {
        if (this.timeout) clearTimeout(this.timeout)
        this.timeout = setTimeout(() => {
          if (Scroller.active) {
            const mode = this.isForce ? 'INFINITE' : 'SMART'
            this.statusEl.innerText = `> SCROLLING:${mode}`
            this.statusEl.style.color = '#0ff'
          } else {
            this.statusEl.innerText = '> SYSTEM READY'
            this.statusEl.style.color = '#0f0'
          }
        }, 2000)
      }
    },

    toggleForce() {
      this.isForce = !this.isForce
      this.forceBtn.innerText = this.isForce ? '[FORCE:ON]' : '[FORCE:OFF]'
      this.forceBtn.style.color = this.isForce ? '#ff9f00' : '#666'
      this.forceBtn.style.textShadow = this.isForce ? '0 0 2px #ff9f00' : 'none'

      if (Scroller.active) {
        this.statusEl.innerText = `> SCROLLING:${this.isForce ? 'INFINITE' : 'SMART'}`
      } else {
        this.updateStatus(
          this.isForce ? 'MODE:FORCE' : 'MODE:SMART',
          this.isForce ? '#ff9f00' : '#888'
        )
      }
    },

    setScrolling(isScrolling) {
      if (isScrolling) {
        this.btn.innerText = '[STOP]'
        this.btn.style.color = '#ff0033'
        this.btn.style.textShadow = '0 0 2px #ff0033'

        const mode = this.isForce ? 'INFINITE' : 'SMART'
        this.statusEl.innerText = `> SCROLLING:${mode}`
        this.statusEl.style.color = '#0ff' // Cyan for active state
        this.el.style.borderColor = '#0ff'
      } else {
        this.btn.innerText = '[RUN]'
        this.btn.style.color = '#0f0'
        this.btn.style.textShadow = '0 0 2px #0f0'

        this.statusEl.innerText = '> HALTED'
        this.statusEl.style.color = '#888'
        this.el.style.borderColor = '#333'
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
            this.updateStatus('SETTINGS ERR', '#ff0033')
          }
        },
        onerror: () => {
          this.updateStatus('CONN FAILED', '#ff0033')
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

    updateSettingsButtons() {
      if (!this.imageBtn) return

      const enabled = this.settings.download_images
      this.imageBtn.innerText = enabled ? '[IMG:ON]' : '[IMG:OFF]'
      this.imageBtn.style.color = enabled ? '#ff9f00' : '#666'
      this.imageBtn.style.textShadow = enabled ? '0 0 2px #ff9f00' : 'none'
    },

    toggleImages() {
      const next = {
        media_dir: this.settings.media_dir,
        download_videos: true,
        download_images: !this.settings.download_images,
      }

      GM_xmlhttpRequest({
        method: 'POST',
        url: SETTINGS_URL,
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify(next),
        onload: (response) => {
          try {
            this.applySettings(JSON.parse(response.responseText))
            this.updateStatus(this.settings.download_images ? 'IMG:ON' : 'IMG:OFF', '#888')
          } catch (e) {
            this.updateStatus('SETTINGS ERR', '#ff0033')
          }
        },
        onerror: () => {
          this.updateStatus('SAVE FAILED', '#ff0033')
        },
      })
    },
  }

  const Scroller = {
    active: false,
    timer: null,

    toggle() {
      if (this.active) this.stop()
      else this.start()
    },

    start() {
      if (this.active) return
      this.active = true
      UI.setScrolling(true)
      this.loop()
    },

    stop() {
      if (!this.active) return
      this.active = false
      clearTimeout(this.timer)
      UI.setScrolling(false)
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
      this._gemini_url = url
    } catch (e) {}
    return originalOpen.apply(this, arguments)
  }

  PageXHR.prototype.send = function () {
    const self = this
    const onLoad = function () {
      self.removeEventListener('load', onLoad)
      const url = self._gemini_url

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
                if (res.duplicate_limit_reached) {
                  if (!UI.isForceMode()) {
                    Scroller.stop()
                    UI.updateStatus('LIMIT REACHED', '#ff0033')
                  } else {
                    // Silent continuation in Force Mode
                    console.log(`[TBD] Limit hit (Force). Saved: ${res.saved_count}`)
                  }
                } else {
                  UI.updateStatus(`SAVED:${res.saved_count}`, '#0f0')
                }
              } catch (e) {
                UI.updateStatus('BACKEND ERR', '#ff0033')
              }
            },
            onerror: function (err) {
              UI.updateStatus('CONN FAILED', '#ff0033')
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
