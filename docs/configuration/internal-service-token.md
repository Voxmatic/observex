# Internal service token and S1-06 authorization changes

**Status:** implemented in S1-06. Not production-ready until the A1 validation
conditions (build, vet, tests, security scans, independent validation and
staging) are met.

## 1. Purpose

ObserveX services authenticate calls to each other with a shared **internal
service token** sent in the `X-ObserveX-Internal-Token` HTTP header. In S1-06:

| Component | Role | Behaviour |
|---|---|---|
| API gateway (`services/api-gateway`) | Receiver (`InternalOnly` middleware) | Fails closed. No gateway route uses `InternalOnly` yet. |
| API gateway | Sender | Adds the header **only** to requests for the `QUERY_ENGINE_URL` origin (same scheme, host and port). Never to the ingestor, AI agent, processor, or external hosts such as Slack, PagerDuty or identity providers. |
| Processor (`services/processor`) | Sender | Adds the header to its gateway notification call when a token is configured. |
| Query engine | Receiver | Planned in S1-04 (not part of S1-06). |
| Ingestor `/internal/*` | — | **Not protected by S1-06** (out of scope). |

## 2. Configuration

| Variable | Meaning |
|---|---|
| `OBSERVEX_INTERNAL_TOKEN_FILE` | Path to a file containing the token. **Preferred.** |
| `OBSERVEX_INTERNAL_TOKEN` | The token value. Used only when `OBSERVEX_INTERNAL_TOKEN_FILE` is unset or empty. |

Legacy names (`INTERNAL_TOKEN`, `X-Internal-Token`) are **not** supported.

### Precedence and fail-closed rules

1. If `OBSERVEX_INTERNAL_TOKEN_FILE` is set to a non-empty path, the file is the
   only source. **There is no fallback** to `OBSERVEX_INTERNAL_TOKEN`, even if the
   file is missing, unreadable, empty or too short. In those cases the token is
   *not configured*.
2. Otherwise `OBSERVEX_INTERNAL_TOKEN` is used.
3. The value is trimmed of surrounding whitespace (a trailing newline in the
   file is fine).
4. **Minimum length: 32 bytes after trimming.** Length is measured in **bytes of
   the UTF-8 string, not characters** (Go `len`). A multi-byte character counts
   as 2–4 bytes. `openssl rand -hex 32` produces 64 ASCII characters (64 bytes).
5. Shorter, empty or unreadable values are treated as **not configured**.

### Generating a token

```bash
openssl rand -hex 32
```

Use one token per environment, shared by every service in that environment.
Never commit a real token; never reuse it for anything else.

### Kubernetes (Helm)

- The chart Secret (`<release>-observex-secrets`) gets the key `internal-token`.
- Set `secrets.internalToken`, or leave it empty to reuse the value already in the
  cluster Secret (Helm `lookup`) or generate 48 random characters on first install.
- `helm template` and GitOps renderers cannot use `lookup`, so they would generate
  a new token on every render. **Set `secrets.internalToken` or
  `secrets.existingSecret` in those setups.**
- With `secrets.existingSecret`, that Secret must contain the `internal-token` key.
- The API gateway and processor mount the key read-only at
  `/var/run/secrets/observex/internal-token` and set
  `OBSERVEX_INTERNAL_TOKEN_FILE` to that path.
- Rendering fails if the token is shorter than 32 bytes after trimming.

### Docker Compose

Both `docker-compose.yml` (repository root) and
`deployments/docker/docker-compose.yml` use a Compose secret:

```bash
# from the directory that holds the compose file
mkdir -p secrets && openssl rand -hex 32 > secrets/internal-token
```

The file is mounted at `/run/secrets/observex_internal_token`, and the services
set `OBSERVEX_INTERNAL_TOKEN_FILE` to that path. `secrets/` is git-ignored.
`docker compose config` does not check that the file exists; if it is missing,
Compose may refuse to start the service or mount an unusable path, depending on
the Compose version. Either way the services treat the token as not configured
and fail closed.

## 3. Behaviour and troubleshooting

| Situation | Receiver response | Gateway log (at startup) |
|---|---|---|
| Token configured, header correct | Request continues | `internal service token loaded` with `source=file` or `source=env` |
| Token configured, header missing or wrong | **401** `{"error":"unauthorized"}` | — |
| Token not configured on the receiver | **503** `{"error":"internal authentication not configured"}` | ERROR `internal service token not configured; internal authentication fails closed` with `source`, `reason` (`not_set`, `file_missing`, `file_unreadable`, `empty`, `too_short`) and, for files, `path` |

- **401** means the caller sent no token or a different token: check that both
  services read the same secret.
- **503** means the receiving service has no usable token: check the file path,
  permissions and length.
- Token values are **never** written to logs, error responses or test output.
  Only the source, reason and file path are logged.
- The token comparison is constant-time.

## 4. Rotation

Only one token is accepted at a time. To rotate: update the secret, then restart
all services that use it together (rolling restart). Requests between services
running different tokens fail with 401 during the change. Dual-token rotation is
future work (D-10).

## 5. S1-06 authorization changes (audited routes only)

| Route | Before | After |
|---|---|---|
| `POST/PUT/DELETE /api/v1/postmortems[/:id]` | Any authenticated user | **Editor** (existing namespace check kept) |
| `POST /api/v1/integrations`, `DELETE /api/v1/integrations/:id`, `POST /api/v1/integrations/:id/test` | Any authenticated user | **Admin** (existing namespace check kept) |
| `DELETE /api/v1/problems/:id/comments/:cid` | Any authenticated user | **Editor**; author-only (unchanged); **404** when nothing was deleted; database errors return a generic 500 |
| `POST /api/v1/llm/ingest` | Any user; body `org_id` trusted | **Editor**; the organization always comes from the caller; 403 if the caller has no organization |
| `GET /api/v1/remediations` | Any user; all organizations' data | **Editor**, then **403** `remediation data is not organization-scoped` until the AI agent records organization IDs |
| `GET /api/v1/agents` | Any user; all organizations' agents | **Editor**; only agents whose `org_id` matches the caller; agents without an `org_id` are hidden; malformed upstream data returns 502 |
| `GET /ws` | No authentication | `Authorization: Bearer` required before upgrade (see §6) |

Other routes are unchanged. About 22 other write routes without role checks are
known gaps that need separate approval; they were deliberately not changed.

## 6. WebSocket (`/ws`)

- **Authentication:** the same `Authorization: Bearer <JWT>` header (or
  `X-API-Key`) accepted by the REST API, checked **before** the upgrade.
  - No credentials or an invalid token: **401**.
  - Authenticated caller without an organization: **403**.
  - Not a WebSocket upgrade request: **426**.
- **Not accepted:** tokens in the query string (`?token=`, `?access_token=`),
  tokens in `Sec-WebSocket-Protocol`, and cookies.
- **Organization scoping:** a connection receives only events for the caller's
  organization. Events published without an organization (for example
  `deployment_result`, `synthetic_result`, `gpu_alert`) are **no longer delivered**
  to any client.
- **Browser clients are not supported.** Native browser WebSocket clients cannot
  set the `Authorization` header, so they cannot connect until a separate
  browser authentication method (for example a short-lived ticket or a session
  cookie) is approved. The frontend in this repository does not use `/ws`.
- **External clients:** the status of WebSocket clients **outside this
  repository is unknown**. Any such client must send the `Authorization` header;
  clients that relied on unauthenticated access, query-string tokens or org-less
  events will stop working.

## 7. Security test script (staging)

`tests/security/a1/s1-06-authz.sh` checks the expected status codes for the
audited routes against a running gateway using viewer, editor and admin
accounts. It reads tokens from environment variables and never prints them.
