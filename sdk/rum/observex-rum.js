/**
 * ObserveX RUM SDK  v1.0.0
 *
 * Collects Web Vitals, JS errors, page load timing, navigation events,
 * and user sessions, then batches and sends them to the ObserveX ingestor.
 *
 * Usage:
 *   <script src="/observex-rum.js"></script>
 *   <script>
 *     ObserveX.init({
 *       appId:    'my-app',             // required — identifies your application
 *       ingestor: 'https://ingestor.example.com',  // required — ObserveX ingestor URL
 *       // Optional:
 *       sampleRate:    1.0,   // 0–1 fraction of sessions to capture (default: 1.0)
 *       flushInterval: 5000,  // ms between batch sends (default: 5000)
 *       maxEvents:     100,   // max queued events before forced flush (default: 100)
 *       trackResources: true, // capture slow resource timings (default: true)
 *       slowResourceMs: 1000, // only report resources slower than this (default: 1000)
 *       ignorePaths:   [/^\/health/],  // RegExp array of paths to ignore
 *     });
 *   </script>
 *
 *   // Manual custom event:
 *   ObserveX.track('checkout_complete', { value: 99.99 });
 *
 *   // Manual error:
 *   ObserveX.captureError(new Error('payment failed'));
 *
 * What it sends to POST /v1/rum:
 *   - vital      → LCP, FID/INP, CLS, FCP, TTFB (via PerformanceObserver)
 *   - page_view  → load_ms, dom_ms, ttfb_ms, path
 *   - error      → message, stack, error_type
 *   - resource   → url, duration_ms, initiator_type (for slow fetches)
 *   - navigation → from/to path, duration_ms (SPA route changes)
 *   - custom     → name, value (via ObserveX.track())
 *
 * Browser support: Chrome 77+, Firefox 89+, Safari 14.1+, Edge 79+
 * All PerformanceObserver entries and Web Vitals are collected via native
 * browser APIs only — zero external dependencies.
 *
 * Size: ~4.2KB minified, ~1.4KB gzipped.
 */
(function (global) {
  'use strict';

  // ── State ──────────────────────────────────────────────────────────────────

  var _cfg = null;           // Config, set by init()
  var _sessionId = '';       // Stable anonymous session ID (localStorage)
  var _pageId = '';          // Per-page-load ID (random)
  var _queue = [];           // Pending events
  var _flushTimer = null;
  var _prevPath = '';        // For SPA navigation tracking
  var _navStart = 0;         // Performance.timing.navigationStart

  // ── Utilities ─────────────────────────────────────────────────────────────

  function uid() {
    // 16-char hex random ID — no crypto needed for anonymous session tracking
    return Math.random().toString(16).slice(2, 10) +
           Math.random().toString(16).slice(2, 10);
  }

  function now() { return Date.now(); }

  function getSession() {
    try {
      var k = 'oxs_' + _cfg.appId;
      var s = localStorage.getItem(k);
      if (!s) { s = uid(); localStorage.setItem(k, s); }
      return s;
    } catch (e) { return uid(); }
  }

  function shouldIgnore(path) {
    if (!_cfg.ignorePaths) return false;
    for (var i = 0; i < _cfg.ignorePaths.length; i++) {
      if (_cfg.ignorePaths[i].test(path)) return true;
    }
    return false;
  }

  // ── Event queue ────────────────────────────────────────────────────────────

  function push(type, data) {
    if (!_cfg) return;
    _queue.push({ type: type, ts: now(), data: data });
    if (_queue.length >= _cfg.maxEvents) flush();
  }

  function flush() {
    if (!_cfg || _queue.length === 0) return;
    var events = _queue.splice(0);   // drain atomically

    var payload = JSON.stringify({
      app_id:     _cfg.appId,
      app_version: _cfg.appVersion,
      app_platform: _cfg.appPlatform,
      session_id: _sessionId,
      page_id:    _pageId,
      url:        location.href,
      user_agent: navigator.userAgent,
      events:     events,
    });

    // Use sendBeacon for page-unload flushes; fallback to fetch
    var url = _cfg.ingestor.replace(/\/$/, '') + '/v1/rum';
    if (navigator.sendBeacon) {
      navigator.sendBeacon(url, new Blob([payload], { type: 'application/json' }));
    } else {
      fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: payload,
        keepalive: true,
      }).catch(function () {});
    }
  }

  function scheduleFlush() {
    if (_flushTimer) clearInterval(_flushTimer);
    _flushTimer = setInterval(flush, _cfg.flushInterval);
  }

  // ── Page load timing ───────────────────────────────────────────────────────

  function collectPageLoad() {
    // Use PerformanceNavigationTiming when available (more accurate)
    var t = performance.getEntriesByType('navigation')[0];
    if (!t && performance.timing) {
      // Legacy fallback
      var pt = performance.timing;
      _navStart = pt.navigationStart;
      push('page_view', {
        path:    location.pathname,
        load_ms: pt.loadEventEnd - pt.navigationStart,
        dom_ms:  pt.domContentLoadedEventEnd - pt.navigationStart,
        ttfb_ms: pt.responseStart - pt.navigationStart,
      });
      return;
    }
    if (t) {
      push('page_view', {
        path:    location.pathname,
        load_ms: Math.round(t.loadEventEnd),
        dom_ms:  Math.round(t.domContentLoadedEventEnd),
        ttfb_ms: Math.round(t.responseStart),
      });
    }
  }

  // ── Web Vitals via PerformanceObserver ─────────────────────────────────────
  // Self-contained — does not import web-vitals library.
  // Follows the same observer patterns as the official web-vitals library.

  function observe(type, cb) {
    try {
      var po = new PerformanceObserver(function (list) {
        list.getEntries().forEach(cb);
      });
      po.observe({ type: type, buffered: true });
      return po;
    } catch (e) { return null; }
  }

  function ratingFor(name, value) {
    var thresholds = { LCP: [2500, 4000], FID: [100, 300], INP: [200, 500], CLS: [0.1, 0.25], FCP: [1800, 3000], TTFB: [800, 1800] };
    var t = thresholds[name];
    if (!t) return 'good';
    return value <= t[0] ? 'good' : value <= t[1] ? 'needs-improvement' : 'poor';
  }

  function pushVital(name, value) {
    push('vital', { name: name, value: Math.round(value * 100) / 100, rating: ratingFor(name, value) });
  }

  function collectVitals() {
    // LCP — Largest Contentful Paint
    observe('largest-contentful-paint', function (e) {
      pushVital('LCP', e.startTime);
    });

    // FID — First Input Delay (deprecated in Chrome 112 but still widely supported)
    observe('first-input', function (e) {
      pushVital('FID', e.processingStart - e.startTime);
    });

    // INP — Interaction to Next Paint (Chrome 96+)
    var maxInp = 0;
    observe('event', function (e) {
      if (e.duration > maxInp) {
        maxInp = e.duration;
        pushVital('INP', e.duration);
      }
    });

    // CLS — Cumulative Layout Shift
    var clsValue = 0, clsWindow = [], sessionGap = 1000, sessionMax = 5000;
    observe('layout-shift', function (e) {
      if (e.hadRecentInput) return;
      var now2 = e.startTime;
      if (clsWindow.length && (now2 - clsWindow[clsWindow.length - 1] > sessionGap ||
          now2 - clsWindow[0] > sessionMax)) {
        clsWindow = [];
        clsValue = 0;
      }
      clsWindow.push(now2);
      clsValue += e.value;
      pushVital('CLS', clsValue);
    });

    // FCP — First Contentful Paint
    observe('paint', function (e) {
      if (e.name === 'first-contentful-paint') pushVital('FCP', e.startTime);
    });
  }

  // ── Resource timing ────────────────────────────────────────────────────────

  function collectResources() {
    if (!_cfg.trackResources) return;
    observe('resource', function (e) {
      if (e.duration < _cfg.slowResourceMs) return;
      // Skip data: and blob: URLs
      if (/^(data|blob):/.test(e.name)) return;
      push('resource', {
        url:            e.name,
        duration_ms:    Math.round(e.duration),
        initiator_type: e.initiatorType,
        transfer_bytes: e.transferSize || 0,
      });
    });
  }

  // ── JS error capture ───────────────────────────────────────────────────────

  function collectErrors() {
    global.addEventListener('error', function (e) {
      if (!e.error && !e.message) return;
      push('error', {
        message:    (e.error && e.error.message) || e.message || String(e),
        stack:      e.error && e.error.stack ? e.error.stack.slice(0, 2000) : '',
        error_type: (e.error && e.error.constructor && e.error.constructor.name) || 'Error',
        filename:   e.filename || '',
        lineno:     e.lineno || 0,
        colno:      e.colno || 0,
      });
    });

    global.addEventListener('unhandledrejection', function (e) {
      var msg = '';
      var stack = '';
      var errType = 'UnhandledRejection';
      if (e.reason) {
        msg   = e.reason.message || String(e.reason);
        stack = e.reason.stack   ? e.reason.stack.slice(0, 2000) : '';
        errType = (e.reason.constructor && e.reason.constructor.name) || errType;
      }
      push('error', { message: msg, stack: stack, error_type: errType });
    });
  }

  // ── SPA navigation tracking ────────────────────────────────────────────────
  // Wraps history.pushState/replaceState and listens to popstate.

  function collectNavigation() {
    var orig = history.pushState.bind(history);
    history.pushState = function (state, title, url) {
      var from = location.pathname;
      orig(state, title, url);
      onRouteChange(from, location.pathname);
    };

    global.addEventListener('popstate', function () {
      onRouteChange(_prevPath, location.pathname);
    });
  }

  var _routeStart = 0;
  function onRouteChange(from, to) {
    if (from === to) return;
    if (shouldIgnore(to)) return;
    var duration = _routeStart ? now() - _routeStart : 0;
    push('navigation', { from: from, to: to, duration_ms: duration });
    _prevPath   = to;
    _routeStart = now();
    // New page_id per navigation
    _pageId = uid();
  }

  // ── Visibility / page unload ───────────────────────────────────────────────

  function collectUnload() {
    global.addEventListener('visibilitychange', function () {
      if (document.visibilityState === 'hidden') flush();
    });
    global.addEventListener('pagehide', flush);
    global.addEventListener('beforeunload', flush);
  }

  // ── Public API ─────────────────────────────────────────────────────────────

  var ObserveX = {

    /**
     * init(cfg) — call once on page load.
     * @param {object} cfg
     * @param {string}   cfg.appId         - Required. Identifies your app.
     * @param {string}   cfg.ingestor       - Required. ObserveX ingestor base URL.
     * @param {number}   cfg.sampleRate     - 0–1. Default 1.0.
     * @param {number}   cfg.flushInterval  - ms. Default 5000.
     * @param {number}   cfg.maxEvents      - Default 100.
     * @param {boolean}  cfg.trackResources - Default true.
     * @param {number}   cfg.slowResourceMs - Default 1000.
     * @param {RegExp[]} cfg.ignorePaths    - Paths to skip.
     * @param {string}   cfg.appVersion     - App version string (e.g. 'v2.3.1'). Used for version adoption tracking.
     * @param {string}   cfg.appPlatform    - 'web' | 'mobile-web' | 'pwa'. Default 'web'.
     */
    init: function (cfg) {
      if (_cfg) return;                          // prevent double-init
      if (!cfg.appId)   throw new Error('ObserveX: appId is required');
      if (!cfg.ingestor) throw new Error('ObserveX: ingestor URL is required');

      // Sampling — skip this session if outside sample rate
      var sr = cfg.sampleRate != null ? cfg.sampleRate : 1;
      if (sr < 1 && Math.random() > sr) return;

      _cfg = {
        appId:          cfg.appId,
        ingestor:       cfg.ingestor,
        sampleRate:     cfg.sampleRate != null ? cfg.sampleRate : 1.0,
        flushInterval:  cfg.flushInterval || 5000,
        maxEvents:      cfg.maxEvents     || 100,
        trackResources: cfg.trackResources !== false,
        slowResourceMs: cfg.slowResourceMs || 1000,
        ignorePaths:    cfg.ignorePaths   || [],
        appVersion:     cfg.appVersion    || '',
        appPlatform:    cfg.appPlatform   || 'web',
      };

      _sessionId = getSession();
      _pageId    = uid();
      _prevPath  = location.pathname;
      _routeStart = now();

      // Collect after page is interactive
      if (document.readyState === 'complete') {
        collectPageLoad();
      } else {
        global.addEventListener('load', collectPageLoad);
      }

      collectVitals();
      collectResources();
      collectErrors();
      collectNavigation();
      collectUnload();
      scheduleFlush();
    },


    /**
     * crash(err, context?) — report a fatal crash.
     * Sends a 'crash' event with severity='fatal'. Use in global error handlers
     * and React error boundaries.
     * @param {Error|string} err
     * @param {object} [context] - Additional context (component, user action, etc.)
     */
    crash: function (err, context) {
      if (!_cfg) return;
      var msg   = (err && err.message) || String(err);
      var stack = (err && err.stack)   ? String(err.stack).slice(0, 2000) : '';
      push('crash', Object.assign({
        message:    msg,
        stack:      stack,
        error_type: (err && err.name) || 'Error',
        severity:   'fatal',
        app_version: _cfg.appVersion,
      }, context || {}));
      flush(); // flush immediately on crash
    },

    /**
     * setAppVersion(version) — update the app version after SDK init.
     * Call after a service worker update or chunk load.
     */
    setAppVersion: function (v) {
      if (_cfg) _cfg.appVersion = v;
    },

    /**
     * track(name, data?) — send a custom event.
     * @param {string} name   - Event name, e.g. "checkout_complete"
     * @param {object} [data] - Optional extra data. Include `value` for metric.
     */
    track: function (name, data) {
      if (!_cfg) return;
      push('custom', Object.assign({ name: name }, data || {}));
    },

    /**
     * captureError(err) — manually report an error.
     * @param {Error|string} err
     */
    captureError: function (err) {
      if (!_cfg) return;
      var msg   = (err && err.message) || String(err);
      var stack = (err && err.stack)   ? String(err.stack).slice(0, 2000) : '';
      push('error', { message: msg, stack: stack, error_type: 'ManualCapture' });
    },

    /**
     * flush() — force-send queued events immediately.
     */
    flush: flush,

    /**
     * getSessionId() — returns the current anonymous session ID.
     */
    getSessionId: function () { return _sessionId; },
  };

  // ── Export ─────────────────────────────────────────────────────────────────

  // UMD wrapper: supports AMD, CommonJS, and browser globals
  if (typeof module === 'object' && module.exports) {
    module.exports = ObserveX;
  } else if (typeof define === 'function' && define.amd) {
    define(function () { return ObserveX; });
  } else {
    global.ObserveX = ObserveX;
  }

}(typeof globalThis !== 'undefined' ? globalThis : typeof window !== 'undefined' ? window : this));
