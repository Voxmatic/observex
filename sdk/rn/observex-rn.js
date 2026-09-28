/**
 * ObserveX React Native SDK  (M1)
 * ================================
 * Drop-in performance monitoring for React Native applications.
 * Tracks: app lifecycle, crashes, network requests, JS errors,
 * custom events, screen views, and device metrics.
 *
 * Usage:
 *   import ObserveX from 'observex-rn';
 *
 *   ObserveX.init({
 *     appId:    'my-app',
 *     appVersion: '2.3.1',
 *     ingestor: 'https://observex.company.io:4318',
 *     platform: 'ios', // or 'android'
 *     sampleRate: 1.0,
 *   });
 *
 * IMPORTANT: Call ObserveX.init() as early as possible, ideally
 * in your root index.js before registering the app component.
 */

'use strict';

const INGESTOR_PATH = '/v1/rum/batch';
const FLUSH_INTERVAL_MS = 5000;
const MAX_BATCH = 50;

var _cfg = {};
var _queue = [];
var _sessionId = '';
var _flushTimer = null;
var _startTime = Date.now();
var _currentScreen = '';
var _screenStartTime = Date.now();

// ── Utilities ─────────────────────────────────────────────────────────────────

function uid() {
  var chars = '0123456789abcdef';
  var id = '';
  for (var i = 0; i < 16; i++) {
    id += chars[Math.floor(Math.random() * chars.length)];
  }
  return id;
}

function now() { return Date.now(); }

function push(type, data) {
  if (!_cfg.ingestor) return;
  var event = Object.assign({
    type: type,
    app_id: _cfg.appId,
    app_version: _cfg.appVersion,
    platform: _cfg.platform || 'unknown',
    session_id: _sessionId,
    ts: now(),
  }, data);
  _queue.push(event);
  if (_queue.length >= MAX_BATCH) flush();
}

function flush() {
  if (!_cfg.ingestor || _queue.length === 0) return;
  var batch = _queue.splice(0);
  var body = JSON.stringify({ events: batch, app_id: _cfg.appId });

  // Use fetch (available in React Native)
  fetch(_cfg.ingestor + INGESTOR_PATH, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-ObserveX-SDK': 'rn-1.0' },
    body: body,
    keepalive: true,
  }).catch(function() {
    // Re-queue failed events (max 200 to avoid memory pressure)
    if (_queue.length < 200) {
      _queue.unshift.apply(_queue, batch.slice(0, 20));
    }
  });
}

// ── Network interceptor ────────────────────────────────────────────────────────

function patchFetch() {
  var originalFetch = global.fetch;
  global.fetch = function(url, options) {
    var startMs = now();
    var method = (options && options.method) || 'GET';
    var urlStr = typeof url === 'string' ? url : url.toString();

    // Skip ObserveX ingestor calls
    if (urlStr.indexOf(_cfg.ingestor) !== -1) {
      return originalFetch.apply(this, arguments);
    }

    return originalFetch.apply(this, arguments).then(function(response) {
      var durationMs = now() - startMs;
      push('network_request', {
        url:         urlStr,
        method:      method,
        status:      response.status,
        duration_ms: durationMs,
        ok:          response.ok,
      });
      return response;
    }, function(error) {
      push('network_error', {
        url:         urlStr,
        method:      method,
        duration_ms: now() - startMs,
        error:       error && error.message,
      });
      throw error;
    });
  };
}

// ── JS Error handler ──────────────────────────────────────────────────────────

function patchErrorHandler() {
  var orig = global.ErrorUtils && global.ErrorUtils.getGlobalHandler();
  if (global.ErrorUtils) {
    global.ErrorUtils.setGlobalHandler(function(error, isFatal) {
      push('js_error', {
        message:   error && error.message,
        stack:     error && error.stack && error.stack.slice(0, 2000),
        is_fatal:  isFatal,
        screen:    _currentScreen,
      });
      flush(); // flush immediately on fatal
      if (orig) orig(error, isFatal);
    });
  }
}

// ── App state monitoring ──────────────────────────────────────────────────────

function trackAppState(AppState) {
  if (!AppState) return;
  AppState.addEventListener('change', function(nextState) {
    push('app_state', {
      state:      nextState,
      session_age_ms: now() - _startTime,
      screen:     _currentScreen,
    });
    if (nextState === 'background' || nextState === 'inactive') {
      flush();
    }
  });
}

// ── Public API ────────────────────────────────────────────────────────────────

var ObserveX = {

  /**
   * init(config) — initialize the SDK.
   * Must be called before any other method.
   */
  init: function(config) {
    if (!config || !config.ingestor || !config.appId) {
      console.warn('[ObserveX] init requires ingestor and appId');
      return;
    }

    _cfg = Object.assign({
      appVersion:  '0.0.0',
      platform:    'unknown',
      sampleRate:  1.0,
      flushMs:     FLUSH_INTERVAL_MS,
    }, config);

    // Sampling
    if (Math.random() > _cfg.sampleRate) {
      _cfg.ingestor = null; // disable
      return;
    }

    _sessionId = uid();
    _startTime = now();

    // Patch fetch for network monitoring
    try { patchFetch(); } catch(e) {}

    // Patch global error handler
    try { patchErrorHandler(); } catch(e) {}

    // Start flush timer
    _flushTimer = setInterval(flush, _cfg.flushMs);

    // Track app start
    push('app_start', {
      platform:    _cfg.platform,
      app_version: _cfg.appVersion,
      app_id:      _cfg.appId,
    });
  },

  /**
   * screen(name) — track a screen view.
   * Call whenever navigation changes.
   */
  screen: function(name) {
    var durationMs = now() - _screenStartTime;
    if (_currentScreen) {
      push('screen_leave', { screen: _currentScreen, duration_ms: durationMs });
    }
    _currentScreen = name;
    _screenStartTime = now();
    push('screen_view', { screen: name });
  },

  /**
   * track(name, properties?) — track a custom event.
   */
  track: function(name, props) {
    push('custom', Object.assign({ event: name, screen: _currentScreen }, props || {}));
  },

  /**
   * crash(error, context?) — report a crash or fatal error.
   * Use in React error boundaries and top-level try/catch.
   */
  crash: function(error, context) {
    push('crash', Object.assign({
      message:   error && error.message,
      stack:     error && error.stack && error.stack.slice(0, 3000),
      screen:    _currentScreen,
      severity:  'fatal',
    }, context || {}));
    flush();
  },

  /**
   * setUser(id, traits?) — identify the current user.
   * id is hashed before sending — never send PII directly.
   */
  setUser: function(id, traits) {
    // Simple hash of user ID (not cryptographic — for grouping only)
    var hash = 0;
    var s = String(id);
    for (var i = 0; i < s.length; i++) {
      hash = ((hash << 5) - hash) + s.charCodeAt(i);
      hash |= 0;
    }
    push('identify', Object.assign({ user_hash: Math.abs(hash).toString(16) }, traits || {}));
  },

  /**
   * metric(name, value, unit?) — record a custom metric.
   * Example: ObserveX.metric('checkout.duration', 312, 'ms')
   */
  metric: function(name, value, unit) {
    push('custom_metric', { name: name, value: value, unit: unit || '' });
  },

  /**
   * attachAppState(AppState) — wire React Native AppState.
   * Call with: ObserveX.attachAppState(require('react-native').AppState)
   */
  attachAppState: function(AppState) {
    trackAppState(AppState);
  },

  /**
   * flush() — force-flush the event queue.
   * Call before app termination.
   */
  flush: flush,

  /**
   * getSessionId() — returns the current session ID.
   */
  getSessionId: function() { return _sessionId; },
};

// CommonJS export (React Native)
if (typeof module !== 'undefined' && module.exports) {
  module.exports = ObserveX;
}

// ESM default export
if (typeof exports !== 'undefined') {
  exports.default = ObserveX;
}
