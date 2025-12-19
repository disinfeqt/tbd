// ==UserScript==
// @name         Twitter Bookmarks Sync to Local
// @namespace    http://tampermonkey.net/
// @version      0.1
// @description  Intercept XHR and use GM_xmlhttpRequest to bypass CSP.
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

  console.log(
    '%c[Sync v0.3] CSP Bypass Mode (GM_xmlhttpRequest).',
    'background: #1d9bf0; color: white; padding: 4px;'
  )

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
                const limitMsg = res.duplicate_limit_reached ? ' | 🛑 History limit reached!' : ''
                console.log(`%c[Sync] ✅ Saved ${res.saved_count} new tweets${limitMsg}`)
              } catch (e) {
                console.log('[Sync] ✅ Data forwarded to backend.')
              }
            },
            onerror: function (err) {
              console.error('[Sync] ❌ Backend connection failed:', err)
            },
          })
        }
      }
    }

    self.addEventListener('load', onLoad)
    return originalSend.apply(this, arguments)
  }
})()
