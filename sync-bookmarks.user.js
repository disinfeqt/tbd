// ==UserScript==
// @name         Twitter Bookmarks Sync to Local
// @namespace    http://tampermonkey.net/
// @version      0.2
// @description  Intercept XHR to sync bookmarks and provide Auto-Scroll feature.
// @author       Gemini
// @match        https://x.com/*
// @match        https://twitter.com/*
// @run-at       document-start
// @grant        unsafeWindow
// @grant        GM_xmlhttpRequest
// ==/UserScript==

;(function () {
  'use strict'
  const RAW_SYNC_URL = 'http://localhost:41008/api/sync-raw'

  console.log('[TBD v0.2] Minimal UI Edition loaded.')

  const UI = {
    el: null,
    btn: null,
    statusEl: null,
    forceBtn: null,
    timeout: null,
    isForce: false,

    init() {
      this.el = document.createElement('div')
      this.el.style.cssText = `
                position: fixed;
                bottom: 100px;
                left: 20px;
                background: rgba(0, 0, 0, 0.7);
                color: #eee;
                padding: 8px 12px;
                border-radius: 8px;
                font-family: monospace;
                font-size: 11px;
                z-index: 999999;
                backdrop-filter: blur(4px);
                border: 1px solid rgba(255,255,255,0.1);
                display: flex;
                flex-direction: column;
                gap: 6px;
                min-width: 200px;
                user-select: none;
            `

      const controls = document.createElement('div')
      controls.style.cssText =
        'display: flex; justify-content: space-between; align-items: center; gap: 10px;'

      this.btn = document.createElement('div')
      this.btn.innerText = '▶'
      this.btn.style.cssText =
        'cursor: pointer; font-size: 16px; color: #1d9bf0; transition: color 0.2s;'
      this.btn.onclick = () => Scroller.toggle()

      this.forceBtn = document.createElement('div')
      this.forceBtn.innerText = '∞'
      this.forceBtn.style.cssText =
        'cursor: pointer; font-size: 16px; color: #666; transition: color 0.2s;'
      this.forceBtn.title = 'Force Mode (Ignore Limit)'
      this.forceBtn.onclick = () => this.toggleForce()

      controls.appendChild(this.btn)
      controls.appendChild(this.forceBtn)

      this.statusEl = document.createElement('div')
      this.statusEl.innerText = 'TBD Ready'
      this.statusEl.style.cssText =
        'text-align: center; color: #888; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 180px;'

      this.el.appendChild(controls)
      this.el.appendChild(this.statusEl)

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

    updateStatus(text, color = '#888') {
      this.statusEl.innerText = text
      this.statusEl.style.color = color

      // Auto revert after success/error messages
      if (color !== '#888') {
        if (this.timeout) clearTimeout(this.timeout)
        this.timeout = setTimeout(() => {
          if (Scroller.active) {
            this.statusEl.innerText = this.isForce ? 'Scrolling (Infinite)' : 'Scrolling (Smart)'
            this.statusEl.style.color = '#eee'
          } else {
            this.statusEl.innerText = 'TBD Ready'
            this.statusEl.style.color = '#888'
          }
        }, 3000)
      }
    },

    toggleForce() {
      this.isForce = !this.isForce
      this.forceBtn.style.color = this.isForce ? '#f4900c' : '#666'
      if (Scroller.active) {
        this.statusEl.innerText = this.isForce ? 'Scrolling (Infinite)' : 'Scrolling (Smart)'
      } else {
        this.updateStatus(
          this.isForce ? 'Force Mode' : 'Smart Mode',
          this.isForce ? '#f4900c' : '#888'
        )
      }
    },

    setScrolling(isScrolling) {
      if (isScrolling) {
        this.btn.innerText = '◼'
        this.btn.style.color = '#f4212e'
        this.statusEl.innerText = this.isForce ? 'Scrolling (Infinite)' : 'Scrolling (Smart)'
        this.statusEl.style.color = '#eee'
      } else {
        this.btn.innerText = '▶'
        this.btn.style.color = '#1d9bf0'
      }
    },

    isForceMode() {
      return this.isForce
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
      UI.updateStatus('Stopped', '#888')
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
                    UI.updateStatus('Limit Reached', '#f4212e')
                  } else {
                    console.log(`[Sync] Limit hit (Force Mode). Saved: ${res.saved_count}`)
                  }
                } else {
                  UI.updateStatus(`Saved: ${res.saved_count}`, '#00ba7c')
                }
              } catch (e) {
                UI.updateStatus('Error', '#f4212e')
              }
            },
            onerror: function (err) {
              UI.updateStatus('Conn Fail', '#f4212e')
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
