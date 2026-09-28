# ObserveX — Unapplied Drafts (Quarantine List)

**Established by:** approval log of record, entry OD-08 (2026-09-28).
**Rule:** the files below are **not applied, not approved for application** (except S1-01, §3) and **not part of the canonical baseline** (OD-11). They stay where they are, unchanged: **do not apply, move or delete them.**

All files are in the project folder `C:\Users\91832\Documents\observex\Claude outputs\`. None of them is present in the repository tree. SHA-256 and bytes were computed from copies taken on 2026-09-28.

## 1. S1-03 — decision A3, NOT APPROVED, NOT APPLIED (quarantined)

| File | Bytes | SHA-256 | Note |
|---|---|---|---|
| `s1-03a-main.go.diff` | 8,994 | `aec88491fcd70d5a02a2d5400098f65822935ac39f235a38f0d37b7bef2eb528` | Diff |
| `s1-03b1-main.go.diff` | 7,902 | `2aa74c3d16fa496a5e4595976e7d430b1634dc1e4a050fde57bf50b4ddb4ce09` | Diff |
| `s1-03c-wiring.diff` | 4,717 | `e6a18641adc7c081668089ceb8f082f5fd67c9fffea71b0150a2058ba5518bfe` | Diff |
| `safe_defaults_test.go` | 10,455 | `00ea24d0565b876ad7520791122c528ae89c6c2f6620409d656770cc4dd19235` | Header "S1-03-a tests" |
| `approval_gate_test.go` | 12,432 | `b32a6b9cab78cb9939d8d919c7461680501ef9105ff5545933d4ecf274173821` | Header "S1-03-b1 tests" |
| `auth_test.go` | 9,846 | `3de67db07bd1f7946e2f68717972f050aeb01fe588d99bd80bd4c7bafe3c0c18` | Header "S1-03-c tests" |
| `actor.go` | 2,219 | `09dc1deb387875cd87b40663943db8b891ed310cf8818a9ac824d787d6f2753f` | Header "Actor headers (S1-03-c)" |
| `actor_headers_test.go` | 5,610 | `f4a39b033fc9764221548e7c668b294d6012129342eb137411042441a8abb9cc` | Test for `actor.go` |
| `transport.go` | 2,646 | `cb695e8583bad882ae0ccfc9f3fa9250497dde28c51c8ed18c0f4539abc436bd` | Differs from the repository's `internal/servicetoken/transport.go` (1,917 bytes) |
| `transport_multi_test.go` | 4,699 | `dc2e6848939fa7518b0cd5a776865c6fbe0b62218948d376534c7df1a98dc97f` | Test |
| `d-py1.diff` | 1,956 | `58048ff099d17b8c947e5f3f04383fb911116b3bf56f2c50c4c4581e21cc1ce5` | Diffs the S1-03c compose wiring |
| `d-py2.diff` | 2,159 | `d6f59c9a96d413ee0a37bd6da3ca7d1e81d37406aa5bc88124f778fe48ccca78` | Diff |
| `d-py3.diff` | 2,783 | `0fb4e5016daa3fc891ea24dfd84ab1ce25f797be185631a9a9afe7e500594102` | Diff |
| `agent.py` | 18,519 | `a2c4e70cb4ce24ee8f65e27beaa4e727afc434ca29fcf621e289f3914a12c29a` | Grouped with S1-03 by timing and neighbouring files (analysis, not confirmed) |
| `test_execution_guards.py` | 8,667 | `eaad49af6175d4cd56491a6db751ae4dd78d1d9b74f2ce74a64c2d24e4f41aa9` | As above |

## 2. SEC-6b — NOT APPROVED, NOT APPLIED (quarantined)

| File | Bytes | SHA-256 | Note |
|---|---|---|---|
| `sec-6b-main.go.diff` | 7,082 | `270ed1f6ca6909d706458dac512411769e175f05893c53fe9ff4430fb92d7f82` | Diff |
| `sec-6b-stores.go.diff` | 5,560 | `fe7f28edef008ad82b36384b32f6273c107df8449f45aa568a77590904185dac` | Diff |
| `users_org_scope_test.go` | 9,869 | `5246ba54846d93d1d4c71233fdee871395d84717aadf8d3c27978932a75c5989` | Header "SEC-6b regression tests" |

## 3. S1-01 — unapplied drafts; implementation authorized under A1 (OD-08)

These are drafts only. OD-08 authorizes implementation of S1-01 under A1, "using the drafts as a starting point". The drafts themselves stay here unchanged; any implementation is recorded in its own log entry with before/after hashes.

| File | Bytes | SHA-256 |
|---|---|---|
| `saml_s101_test.go` | 15,466 | `6371627e14ba291c4d7f4ba09717afd5aad13f4eca9c82930bf54d885d75b4b3` |
| `s101-main.go.diff` | 3,963 | `b027550f95670dc7e6ac9deac1a0bf389ff64ff883f2023b6683dd8b8aa8d0bc` |
| `S1-01-local-validation.ps1` | 16,726 | `074fabbf8eadfccad8505ea448259f06688f21099231c1114c9f2e17d576256d` |

## 4. QE-DEP — tooling scripts, archived (OD-08); no product change

| File | Bytes | SHA-256 |
|---|---|---|
| `QE-DEP-trial.ps1` | 49,205 | `57666afc1c11a1131657175a01b3e21660040770bc6f423e401b2eabbc0d4dc3` |
| `QE-DEP-evidence-closure.ps1` | 80,014 | `49a9089ff9649db203e6e402a82b5ede345bbf698f61cb4ab90b652b7fe8fecd` |

*Created 2026-09-28 under OD-08. Nothing listed here was applied, moved or deleted.*
