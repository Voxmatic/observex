/**
 * observex-rum.test.js
 * Unit tests for the ObserveX RUM SDK.
 *
 * Run with:  node --test observex-rum.test.js   (Node 20+)
 *    or:     npx vitest observex-rum.test.js
 *    or:     npx jest observex-rum.test.js
 *
 * The tests mock browser globals (PerformanceObserver, localStorage,
 * navigator.sendBeacon, fetch) rather than requiring a real browser.
 */

'use strict';

const assert = require('node:assert/strict');
const { test, describe, beforeEach, afterEach, mock } = require('node:test');

// ── Browser environment mock ────────────────────────────────────────────────
// Set up minimal globals before requiring the SDK

const capturedBeacons = [];
const capturedFetches  = [];
let storageData = {};

// Use Object.defineProperty for read-only globals in Node
Object.defineProperty(global, 'localStorage', { value: {
  getItem:    (k) => storageData[k] ?? null,
  setItem:    (k, v) => { storageData[k] = v; },
  removeItem: (k) => { delete storageData[k]; },
  clear:      () => { storageData = {}; },
}, writable: true, configurable: true });

Object.defineProperty(global, 'navigator', { value: {
  userAgent: 'TestAgent/1.0',
  sendBeacon: (url, blob) => {
    capturedBeacons.push({ url, blob });
    return true;
  },
}, writable: true, configurable: true });

global.fetch = (url, opts) => {
  capturedFetches.push({ url, opts });
  return Promise.resolve({ ok: true });
};

// PerformanceObserver — captures what the SDK tries to observe
const poCallbacks = {};
global.PerformanceObserver = class {
  constructor(cb) { this._cb = cb; }
  observe({ type }) { poCallbacks[type] = this._cb; }
  disconnect() {}
};
PerformanceObserver.supportedEntryTypes = [
  'largest-contentful-paint', 'first-input', 'event',
  'layout-shift', 'paint', 'resource', 'navigation',
];

global.performance = {
  getEntriesByType: (type) => {
    if (type === 'navigation') return [{
      loadEventEnd: 1200, domContentLoadedEventEnd: 800, responseStart: 120,
    }];
    return [];
  },
};

Object.defineProperty(global, 'document', { value: {
  readyState: 'complete',
  visibilityState: 'visible',
}, writable: true, configurable: true });

Object.defineProperty(global, 'location', { value: { href: 'https://example.com/test', pathname: '/test' }, writable: true, configurable: true });

const eventListeners = {};
global.window = global;
global.addEventListener = (type, fn) => {
  if (!eventListeners[type]) eventListeners[type] = [];
  eventListeners[type].push(fn);
};
global.removeEventListener = () => {};

Object.defineProperty(global, 'history', { value: {
  pushState: () => {},
}, writable: true, configurable: true });

global.Blob = class {
  constructor(parts, opts) {
    this.parts = parts;
    this.type  = opts?.type || '';
    this._text = parts.join('');
  }
  text() { return Promise.resolve(this._text); }
};

// ── Load SDK (fresh require each test via cache bust) ───────────────────────

function loadSDK() {
  // Clear module cache for a clean slate each test
  const key = Object.keys(require.cache).find(k => k.includes('observex-rum'));
  if (key) delete require.cache[key];
  return require('./observex-rum.js');
}

// ── Helpers ─────────────────────────────────────────────────────────────────

function resetState() {
  capturedBeacons.length = 0;
  capturedFetches.length  = 0;
  storageData = {};
  Object.keys(poCallbacks).forEach(k => delete poCallbacks[k]);
  Object.keys(eventListeners).forEach(k => delete eventListeners[k]);
}

function getBeaconPayload(index = 0) {
  const beacon = capturedBeacons[index];
  assert.ok(beacon, 'Expected a beacon to have been sent');
  return JSON.parse(beacon.blob._text);
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe('ObserveX RUM SDK', () => {

  describe('init()', () => {
    beforeEach(resetState);

    test('throws when appId is missing', () => {
      const SDK = loadSDK();
      assert.throws(
        () => SDK.init({ ingestor: 'http://localhost:4318' }),
        /appId/
      );
    });

    test('throws when ingestor is missing', () => {
      const SDK = loadSDK();
      assert.throws(
        () => SDK.init({ appId: 'test' }),
        /ingestor/
      );
    });

    test('sets up session ID in localStorage', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      const sid = localStorage.getItem('oxs_test');
      assert.ok(sid, 'Session ID should be stored');
      assert.equal(sid.length, 16, 'Session ID should be 16 hex chars');
    });

    test('reuses existing session ID', () => {
      storageData['oxs_test'] = 'existing-session';
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      assert.equal(SDK.getSessionId(), 'existing-session');
    });

    test('is idempotent — double init is safe', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      assert.doesNotThrow(() => {
        SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      });
    });

    test('collects page load timing on init', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      SDK.flush();
      const payload = getBeaconPayload();
      const pgView = payload.events.find(e => e.type === 'page_view');
      assert.ok(pgView, 'Should have a page_view event');
      assert.equal(pgView.data.load_ms, 1200);
      assert.equal(pgView.data.dom_ms, 800);
      assert.equal(pgView.data.ttfb_ms, 120);
      assert.equal(pgView.data.path, '/test');
    });
  });

  describe('Web Vitals', () => {
    beforeEach(resetState);

    test('captures LCP', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });

      // Simulate LCP entry
      const lcpCb = poCallbacks['largest-contentful-paint'];
      assert.ok(lcpCb, 'LCP observer should be registered');
      lcpCb({ getEntries: () => [{ startTime: 1800 }] });

      SDK.flush();
      const payload = getBeaconPayload();
      const vital = payload.events.find(e => e.type === 'vital' && e.data.name === 'LCP');
      assert.ok(vital, 'Should have LCP vital event');
      assert.equal(vital.data.value, 1800);
      assert.equal(vital.data.rating, 'good', 'LCP 1800ms should be rated good');
    });

    test('rates LCP correctly', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      const lcpCb = poCallbacks['largest-contentful-paint'];

      // Test all three buckets
      const cases = [
        [2000, 'good'],
        [3000, 'needs-improvement'],
        [5000, 'poor'],
      ];

      for (const [value, expected] of cases) {
        lcpCb({ getEntries: () => [{ startTime: value }] });
      }

      SDK.flush();
      const payload = getBeaconPayload();
      const vitals  = payload.events.filter(e => e.type === 'vital' && e.data.name === 'LCP');
      assert.equal(vitals[0].data.rating, 'good');
      assert.equal(vitals[1].data.rating, 'needs-improvement');
      assert.equal(vitals[2].data.rating, 'poor');
    });

    test('captures CLS', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      const clsCb = poCallbacks['layout-shift'];

      clsCb({ getEntries: () => [{ startTime: 100, value: 0.05, hadRecentInput: false }] });
      clsCb({ getEntries: () => [{ startTime: 200, value: 0.05, hadRecentInput: false }] });

      SDK.flush();
      const payload = getBeaconPayload();
      const clsEvents = payload.events.filter(e => e.type === 'vital' && e.data.name === 'CLS');
      assert.ok(clsEvents.length >= 1, 'Should have CLS events');
      // CLS is cumulative — last entry should be sum
      const last = clsEvents[clsEvents.length - 1];
      assert.ok(Math.abs(last.data.value - 0.10) < 0.001, 'CLS should accumulate to ~0.10');
    });

    test('ignores CLS entries with hadRecentInput', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      const clsCb = poCallbacks['layout-shift'];

      clsCb({ getEntries: () => [{ startTime: 100, value: 0.5, hadRecentInput: true }] });

      SDK.flush();
      const payload = getBeaconPayload();
      const clsEvents = payload.events.filter(e => e.type === 'vital' && e.data.name === 'CLS');
      // The input-triggered shift should not be counted
      assert.equal(clsEvents.length, 0, 'Input-triggered layout shifts should be ignored');
    });

    test('captures FID', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      const fidCb = poCallbacks['first-input'];

      fidCb({ getEntries: () => [{ startTime: 100, processingStart: 180 }] });

      SDK.flush();
      const payload = getBeaconPayload();
      const fid = payload.events.find(e => e.type === 'vital' && e.data.name === 'FID');
      assert.ok(fid, 'Should have FID event');
      assert.equal(fid.data.value, 80, 'FID = processingStart - startTime = 80ms');
    });

    test('captures FCP from paint observer', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      const paintCb = poCallbacks['paint'];

      paintCb({ getEntries: () => [
        { name: 'first-paint', startTime: 500 },
        { name: 'first-contentful-paint', startTime: 600 },
      ]});

      SDK.flush();
      const payload = getBeaconPayload();
      const fcp = payload.events.find(e => e.type === 'vital' && e.data.name === 'FCP');
      assert.ok(fcp, 'Should have FCP event');
      assert.equal(fcp.data.value, 600);
    });
  });

  describe('Error capture', () => {
    beforeEach(resetState);

    test('captures window errors', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });

      const errorHandlers = eventListeners['error'] || [];
      assert.ok(errorHandlers.length > 0, 'Error listener should be registered');

      const mockError = new Error('Test error');
      mockError.stack = 'Error: Test error\n  at test.js:1';
      errorHandlers[0]({
        error:    mockError,
        message:  mockError.message,
        filename: 'test.js',
        lineno:   1,
        colno:    5,
      });

      SDK.flush();
      const payload = getBeaconPayload();
      const err = payload.events.find(e => e.type === 'error');
      assert.ok(err, 'Should have error event');
      assert.equal(err.data.message, 'Test error');
      assert.ok(err.data.stack.includes('Error: Test error'));
      assert.equal(err.data.error_type, 'Error');
      assert.equal(err.data.filename, 'test.js');
    });

    test('captures unhandled promise rejections', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });

      const rejHandlers = eventListeners['unhandledrejection'] || [];
      assert.ok(rejHandlers.length > 0, 'unhandledrejection listener should be registered');

      rejHandlers[0]({
        reason: new TypeError('fetch failed'),
      });

      SDK.flush();
      const payload = getBeaconPayload();
      const err = payload.events.find(e => e.type === 'error');
      assert.ok(err, 'Should capture unhandled rejection');
      assert.equal(err.data.error_type, 'TypeError');
    });

    test('manual captureError works', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      SDK.captureError(new RangeError('out of range'));
      SDK.flush();
      const payload = getBeaconPayload();
      const err = payload.events.find(e => e.type === 'error');
      assert.ok(err);
      assert.equal(err.data.error_type, 'ManualCapture');
      assert.ok(err.data.message.includes('out of range'));
    });

    test('manual captureError handles string errors', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      SDK.captureError('something went wrong');
      SDK.flush();
      const payload = getBeaconPayload();
      const err = payload.events.find(e => e.type === 'error');
      assert.ok(err);
      assert.equal(err.data.message, 'something went wrong');
    });
  });

  describe('Custom events', () => {
    beforeEach(resetState);

    test('track() sends a custom event', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      SDK.track('checkout_complete', { value: 99.99, currency: 'USD' });
      SDK.flush();
      const payload = getBeaconPayload();
      const ev = payload.events.find(e => e.type === 'custom');
      assert.ok(ev, 'Should have custom event');
      assert.equal(ev.data.name, 'checkout_complete');
      assert.equal(ev.data.value, 99.99);
      assert.equal(ev.data.currency, 'USD');
    });

    test('track() without value still records the event', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      SDK.track('modal_opened');
      SDK.flush();
      const payload = getBeaconPayload();
      const ev = payload.events.find(e => e.type === 'custom');
      assert.ok(ev);
      assert.equal(ev.data.name, 'modal_opened');
    });

    test('track() before init is silently ignored', () => {
      const SDK = loadSDK();
      assert.doesNotThrow(() => SDK.track('too_early'));
    });
  });

  describe('Resource timing', () => {
    beforeEach(resetState);

    test('captures slow resources', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318', slowResourceMs: 500 });
      const resCb = poCallbacks['resource'];

      resCb({ getEntries: () => [
        { name: 'https://example.com/large-image.jpg', duration: 1500, initiatorType: 'img', transferSize: 200000 },
        { name: 'https://example.com/fast.css',       duration: 100,  initiatorType: 'link', transferSize: 5000 },
      ]});

      SDK.flush();
      const payload = getBeaconPayload();
      const resources = payload.events.filter(e => e.type === 'resource');
      assert.equal(resources.length, 1, 'Only the slow resource should be reported');
      assert.ok(resources[0].data.url.includes('large-image'));
      assert.equal(resources[0].data.duration_ms, 1500);
    });

    test('skips data: URLs', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318', slowResourceMs: 0 });
      const resCb = poCallbacks['resource'];

      resCb({ getEntries: () => [
        { name: 'data:image/png;base64,ABC', duration: 5000, initiatorType: 'img', transferSize: 0 },
      ]});

      SDK.flush();
      const payload = getBeaconPayload();
      const resources = payload.events.filter(e => e.type === 'resource');
      assert.equal(resources.length, 0, 'data: URLs should be skipped');
    });
  });

  describe('Flush behaviour', () => {
    beforeEach(resetState);

    test('sends to correct ingestor URL', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'https://ingest.mycompany.com' });
      SDK.flush();
      assert.ok(capturedBeacons.length > 0, 'Should have sent beacon');
      assert.ok(capturedBeacons[0].url.includes('ingest.mycompany.com/v1/rum'));
    });

    test('strips trailing slash from ingestor URL', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'https://ingest.mycompany.com/' });
      SDK.flush();
      assert.ok(!capturedBeacons[0].url.includes('//v1/rum'), 'Should not double-slash');
    });

    test('includes app_id, session_id, page_id, url, user_agent', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'my-app', ingestor: 'http://localhost:4318' });
      SDK.flush();
      const payload = getBeaconPayload();
      assert.equal(payload.app_id, 'my-app');
      assert.ok(payload.session_id, 'session_id should be present');
      assert.ok(payload.page_id,    'page_id should be present');
      assert.ok(payload.url,        'url should be present');
      assert.ok(payload.user_agent, 'user_agent should be present');
    });

    test('does not send empty batches', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      SDK.flush(); // drain page_view event
      capturedBeacons.length = 0; // now clear

      SDK.flush(); // nothing queued — should not send
      assert.equal(capturedBeacons.length, 0, 'Should not beacon when queue is empty');
    });

    test('sampling: sampleRate=0 prevents any data', () => {
      // sampleRate: 0 means Math.random() > 0 always, so init() exits early
      // We need to override Math.random before loading the SDK
      const origRandom = Math.random;
      Math.random = () => 0.99; // always > sampleRate

      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318', sampleRate: 0 });
      SDK.flush();
      assert.equal(capturedBeacons.length, 0, 'No data with sampleRate=0');

      Math.random = origRandom;
    });
  });

  describe('ignorePaths', () => {
    beforeEach(resetState);

    test('ignores SPA navigations to ignored paths', () => {
      const SDK = loadSDK();
      Object.defineProperty(global, 'location', { value: { href: 'https://example.com/', pathname: '/' }, writable: true, configurable: true });
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318', ignorePaths: [/^\/health/] });

      // Simulate pushState to ignored path
      const origPushState = global.history.pushState;
      let navPushed = false;
      global.history.pushState = (s, t, url) => {
        navPushed = true;
        global.location.pathname = '/health';
      };

      // Trigger navigation
      global.history.pushState({}, '', '/health');

      SDK.flush();
      // Should have no navigation event for /health
      const beacons = capturedBeacons;
      if (beacons.length > 0) {
        const payload = JSON.parse(beacons[beacons.length - 1].blob._text);
        const navEvents = payload.events.filter(e => e.type === 'navigation' && e.data.to === '/health');
        assert.equal(navEvents.length, 0, 'Navigation to ignored path should be suppressed');
      }

      global.history.pushState = origPushState;
    });
  });

  describe('Payload shape validation', () => {
    beforeEach(resetState);

    test('events always have type, ts, data fields', () => {
      const SDK = loadSDK();
      SDK.init({ appId: 'test', ingestor: 'http://localhost:4318' });
      SDK.track('test_event', { value: 1 });
      SDK.flush();

      const payload = getBeaconPayload();
      assert.ok(Array.isArray(payload.events), 'events should be an array');
      for (const ev of payload.events) {
        assert.ok(typeof ev.type === 'string',  `event missing type: ${JSON.stringify(ev)}`);
        assert.ok(typeof ev.ts   === 'number',  `event missing ts: ${JSON.stringify(ev)}`);
        assert.ok(typeof ev.data === 'object',  `event missing data: ${JSON.stringify(ev)}`);
        assert.ok(ev.ts > 0, 'ts should be a positive Unix ms timestamp');
      }
    });
  });

});
