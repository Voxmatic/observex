# ObserveX — Governance Record Manifest

**Folder:** `docs/governance/record/`, established 2026-09-28 by the approval log of record, entries OD-01 … OD-04.
**Rule:** these are the records of record. `07_Approval_Log.md` is **append-only**. The other records change only by an owner-instructed new version or a dated amendment recorded in `07`. When the repository exists (RP-1, OD-10), this folder is committed and versioned there.

**Source key.** [S] is Claude's session output store, folder `observex-plan/` (files delivered in earlier Claude sessions).
**Method.** Each record file was copied from its source and compared byte-for-byte (`cmp`) and by SHA-256. It was then read back from the project folder after delivery and compared again.

## 1. Records of record

| File | Kind | Source | Source SHA-256 | Record SHA-256 | Bytes | Decision |
|---|---|---|---|---|---|---|
| `07_Approval_Log.md` | **Approval log of record** (append-only) | [S] `07_Approval_Log.md` (G4) | `fb2beb0c485120cb9ab9eae7061c5f51758cc3e668763b949e8ecf703497b2c8` | See §2 | See §2 | OD-01 |
| `11_Remediation_Plan.md` | Remediation plan of record | [S] `11_Remediation_Plan.md` (G6) | `9f961d90a01f265c1fe275c6db70740f604cdd8241a2d1a379c1bda4b3c3252e` | identical | 19,395 | OD-02 |
| `03_Engineering_Decision_Register.md` | Decision register of record | [S] `03_Engineering_Decision_Register.md` (G2; G1 identical) | `81441f356e5ba4f9866cf372cd544680d9d1fc45cfd60e238b3e35d291f39100` | identical | 27,137 | OD-04 |
| `05_Stabilization_Plan.md` | Unversioned original (no version or date inside) | [S] `05_Stabilization_Plan.md` (G7) | `dd7923d89f132f8be3516d5bac645a9a330eba05ff726dd91c003bc3aeac716d` | identical | 16,484 | OD-03 |
| `06_Decisions_Requiring_Approval.md` | Unversioned original (no version or date inside; answer boxes unticked; decisions live only in `07`) | [S] `06_Decisions_Requiring_Approval.md` (G8) | `2036e28a3ceea87179c885103c62f586897151e6ccac86ebea850654f7d651bf` | identical | 5,884 | OD-03 |
| `A1_Implementation_Plans.md` | Unversioned original | [S] `A1_Implementation_Plans.md` | `dcf61e005a55f784ccbe8365e5f809fbf1142ccd8f0a4101c79d62399d4514d2` | identical | 34,332 | OD-03 |
| `02_Implementation_Roadmap.md` | Unversioned original (reference only; the charter's Phases 0–5 succeed it, OD-12) | [S] `02_Implementation_Roadmap.md` | `b5c827339d314a247acc082c9e28274d4bd2d2cd2a1b4179aaad4834c4500d9d` | identical | 12,112 | OD-03 |
| `08_Staging_Readiness_Checklist.md` | Unversioned original | [S] `08_Staging_Readiness_Checklist.md` | `e609daffe4b400137aad257576b3c32094b97935a5c511208859c514dbe6b1bc` | identical | 4,702 | OD-03 |
| `09_Staging_Inventory_Form.md` | Unversioned original | [S] `09_Staging_Inventory_Form.md` | `1f10ac905ed7e5ef216b7e5efbeae32f283825a9e5ecfe6a2cb8efaed8f798da` | identical | 7,045 | OD-03 |
| `10_Remote_Validation_Evaluation.md` | Unversioned original | [S] `10_Remote_Validation_Evaluation.md` | `8923e63d4bd34cdde4b117cee076f66a5ea0d2fd883fc5b9de5241dd003734b0` | identical | 14,972 | OD-03 |

## 2. The approval log of record

| Point in time | Bytes | SHA-256 | Note |
|---|---|---|---|
| Seed (= G4) | 75,923 | `fb2beb0c485120cb9ab9eae7061c5f51758cc3e668763b949e8ecf703497b2c8` | The record's first 75,923 bytes are byte-identical to G4 (verified with `cmp -n 75923`) |
| After entries OD-01 … OD-19 (2026-09-28) | 95,429 | `04d937c4a6d7886860ad987ec0c70fab40e768c8adf3a5aac57056ce4f4de957` | 19 appended entries; nothing before byte 75,924 changed |

Later appends change the file's hash. Each later state is identified by this folder's version history once the repository exists, and meanwhile by the hash recorded in the entry or report that appended it. Its prefix must always equal the previous state.

## 3. Companion files in this folder

| File | Purpose | SHA-256 | Bytes |
|---|---|---|---|
| `NOT-OF-RECORD.md` | Every other copy, with hashes (OD-01) | `aaaee90765ef53d5b0a57949e8839d62b2da02356a2d336d9b8227cbdade9e2d` | 4,111 |
| `UNAPPLIED-DRAFTS.md` | Quarantined S1-03 and SEC-6b drafts; S1-01 drafts; archived QE-DEP tooling (OD-08) | `113a892dc9c92f504c676c890da24c5783bde5a4455dc57d87a534a46b8a6dad` | 4,111 |
| `MANIFEST.md` | This file | not self-recorded | — |

## 4. Not in this folder

- The proposed baseline package and the owner-decision-preparation package are **not records** (see `NOT-OF-RECORD.md` §2).
- The approval log does not approve itself: every decision is an entry in `07`.

*Created 2026-09-28 under OD-01 … OD-04.*
