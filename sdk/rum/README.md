# ObserveX RUM SDK

Lightweight browser JavaScript library for Real User Monitoring.
Collects Web Vitals, JS errors, page load timing, SPA navigation events,
and user sessions — then sends them to the ObserveX ingestor.

**Size:** ~5.2 KB minified · ~2.2 KB gzipped · zero dependencies

---

## Quick start

### Option A — script tag (any framework)

```html
<!-- In <head>, before any other scripts -->
<script src="/observex-rum.min.js"></script>
<script>
  ObserveX.init({
    appId:    'my-app',
    ingestor: 'https://ingestor.example.com',
  });
</script>
```

### Option B — npm / bundler

```bash
cp observex-rum.js src/lib/observex-rum.js
```

```js
// src/observex.js
import ObserveX from './lib/observex-rum.js'

ObserveX.init({
  appId:    'my-app',
  ingestor: import.meta.env.VITE_INGESTOR_URL,
})

export default ObserveX
```

---

## Configuration

```js
ObserveX.init({
  // ── Required ──────────────────────────────────────────────────────
  appId:    'my-app',                // Identifies your application in dashboards
  ingestor: 'https://ingestor.io',  // ObserveX ingestor base URL (no trailing slash)

  // ── Optional ──────────────────────────────────────────────────────
  sampleRate:     1.0,       // 0–1.  Fraction of sessions to capture. Default: 1.0
  flushInterval:  5000,      // ms.   Batch send interval. Default: 5000 (5 seconds)
  maxEvents:      100,       // int.  Max queued events before forced flush. Default: 100
  trackResources: true,      // bool. Capture slow resource loads. Default: true
  slowResourceMs: 1000,      // ms.   Only report resources slower than this. Default: 1000
  ignorePaths:    [/^\/health/, /^\/admin/],  // RegExp[]. Skip these paths. Default: []
})
```

---

## Manual instrumentation

### Custom events

```js
// Track a business event (appears in Loki log explorer)
ObserveX.track('checkout_complete')

// Track a metric (appears in VictoriaMetrics / Metrics explorer)
ObserveX.track('checkout_complete', { value: 99.99 })

// Track with extra labels
ObserveX.track('feature_used', { value: 1, feature: 'dark_mode' })
```

### Manual error capture

```js
try {
  await processPayment()
} catch (err) {
  ObserveX.captureError(err)      // stack trace + error type sent to Loki
  showErrorToUser(err.message)
}
```

### Force flush (e.g. before navigation)

```js
ObserveX.flush()
```

### Get the anonymous session ID

```js
const sid = ObserveX.getSessionId()
// Pass to your backend for correlating user actions with RUM data
```

---

## Framework integrations

### React

```jsx
// src/main.jsx
import ObserveX from './lib/observex-rum.js'

ObserveX.init({ appId: 'react-app', ingestor: '/ingestor' })

// Error boundary to capture render errors
import { Component } from 'react'
export class RUMErrorBoundary extends Component {
  componentDidCatch(error) { ObserveX.captureError(error) }
  render() { return this.props.children }
}
```

```jsx
// App.jsx — wrap root
<RUMErrorBoundary>
  <Router>
    <App />
  </Router>
</RUMErrorBoundary>
```

### Next.js

```js
// pages/_app.js  (or app/layout.js for App Router)
import ObserveX from '../lib/observex-rum.js'
import { useEffect } from 'react'

export default function MyApp({ Component, pageProps }) {
  useEffect(() => {
    ObserveX.init({
      appId:    'nextjs-app',
      ingestor: process.env.NEXT_PUBLIC_INGESTOR_URL,
    })
  }, [])

  return <Component {...pageProps} />
}
```

### Vue 3

```js
// main.js
import { createApp } from 'vue'
import ObserveX from './lib/observex-rum.js'
import App from './App.vue'

ObserveX.init({ appId: 'vue-app', ingestor: import.meta.env.VITE_INGESTOR_URL })

const app = createApp(App)
app.config.errorHandler = (err) => ObserveX.captureError(err)
app.mount('#app')
```

### Angular

```ts
// src/main.ts
import ObserveX from './lib/observex-rum.js'
import { platformBrowserDynamic } from '@angular/platform-browser-dynamic'
import { AppModule } from './app/app.module'
import { ErrorHandler } from '@angular/core'

ObserveX.init({ appId: 'angular-app', ingestor: environment.ingestorUrl })

// Global Angular error handler
export class RUMErrorHandler implements ErrorHandler {
  handleError(error: unknown) { ObserveX.captureError(error as Error) }
}
```

---

## What data is collected

### Metrics (queryable with PromQL in the Metrics Explorer)

| Metric name | Labels | Description |
|---|---|---|
| `rum_web_vital_ms` | `vital`, `rating`, `app_id`, `session_id`, `url` | LCP, FID, INP, CLS, FCP per page load |
| `rum_page_load_ms` | `path`, `app_id`, `session_id` | Total page load time |
| `rum_dom_content_loaded_ms` | `path`, `app_id`, `session_id` | DOM ready time |
| `rum_ttfb_ms` | `path`, `app_id`, `session_id` | Time to first byte |
| `rum_js_errors_total` | `error_type`, `app_id`, `session_id` | JS error count |
| `rum_resource_duration_ms` | `resource_url`, `initiator_type` | Slow resource loads |
| `rum_navigation_ms` | `from_path`, `to_path` | SPA route change duration |
| `rum_custom_event` | `event_name`, `app_id` | Custom numeric events via `ObserveX.track()` |

### Logs (searchable in the Log Explorer)

| Level | Message pattern | When |
|---|---|---|
| `info` | `page_view url=... session=...` | Every navigation |
| `error` | `js_error: <message>` | JS error / unhandledrejection |
| `info` | `custom_event: <name>` | `ObserveX.track()` without a numeric value |

---

## Privacy

- **No PII collected.** Session IDs are random hex strings, not tied to user identity.
- **Session IDs** are stored in `localStorage`, scoped to the `appId`. Clear them with `localStorage.removeItem('oxs_<appId>')`.
- **User agents** are sent to the ingestor but not stored as PII — they are used for browser/device segmentation only.
- **IP addresses** are not captured by the SDK; the ingestor may log them per your server configuration.

---

## Ingestor configuration

The SDK posts to `POST /v1/rum` on your ingestor. Configure the allowed origin:

```yaml
# In your Helm values.yaml
ingestor:
  extraEnv:
    - name: RUM_CORS_ORIGIN
      value: "https://yourapp.example.com"   # restrict to your domain in production
                                              # use "*" for development only
```

Or with docker-compose:

```yaml
ingestor:
  environment:
    RUM_CORS_ORIGIN: "https://yourapp.example.com"
```

---

## Example PromQL queries for dashboards

```promql
# p75 LCP over the last hour, by app
histogram_quantile(0.75,
  sum(rate(rum_web_vital_ms_bucket{vital="LCP"}[1h])) by (le, app_id)
)

# % of sessions with Good LCP (< 2500ms)
sum(rum_web_vital_ms{vital="LCP", rating="good"}) /
sum(rum_web_vital_ms{vital="LCP"}) * 100

# JS error rate per minute
sum(rate(rum_js_errors_total[1m])) by (app_id, error_type)

# Slowest pages (p95 load time)
topk(10,
  histogram_quantile(0.95,
    sum(rate(rum_page_load_ms_bucket[1h])) by (le, path)
  )
)

# Average TTFB by path
avg(rum_ttfb_ms) by (path)
```
