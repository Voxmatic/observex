# ObserveX — Canonical Baseline Proposal

> **PROPOSED — NOT APPROVED.** This proposes how the owner *could* establish a canonical governance and technical baseline.
> - It establishes nothing by itself.
> - Until the owner approves a canonical baseline, the project folder is the **working reference only**.
> - The 16 September source set and every competing copy are preserved unchanged.

| Field | Value |
|---|---|
| Document | `docs/governance/proposed-baseline/ObserveX-Canonical-Baseline-Proposal.md` |
| Version | v0.1.0 (proposed) |
| Date generated | 2026-09-28 |
| Status | Proposed; every acceptance field is blank |

**Sources inspected.** Every source listed in `ObserveX-Governance-Source-Reconciliation.md` §2, with its hashes. Also the project `README.md`, `Chart.yaml`, `scripts/db-migrate.sh` and migrations `001`–`009`.

---

## 1. What a canonical baseline must settle

A baseline has two parts. The owner can accept each one separately.

1. **Governance baseline.** Which copy of each record is the record of record:
   - `03` decision register;
   - `05` stabilization plan;
   - `06` approval checklist;
   - `07` approval log;
   - `11` remediation plan;

   and where each lives.
2. **Technical baseline.** Which code state and which evidence are the reference for further work, and in which evidence category each item sits.

---

## 2. Governance baseline: options per record

**Nothing below is chosen.** Hash prefixes are from the reconciliation record, §2.

| Record | Copies found | Option A | Option B | Option C | Evidence the owner needs |
|---|---|---|---|---|---|
| `03` register | G1 = G2 (identical, `81441f35…`) | Accept the identical content as canonical; keep it at its current project path | Move the canonical copy to a governance folder **as a new copy**, recording the hash | Owner re-issues | None beyond the hash; no conflict |
| `07` approval log | G3 (project, `681c147c…`), a byte-prefix of G4 (session store, `fb2beb0c…`) | G4 canonical: it is G3 plus the 2026-09-23 entry | G3 canonical; the owner decides separately whether to append G4's extra entry | Neither; the owner re-issues | Whether the 2026-09-23 detector entry was accepted; where the log of record lives (C-4) |
| `11` plan | G5 (project, `812c26b2…`) vs G6 (session store, `9f961d90…`); one row differs | G6 canonical (includes the S1-04 amendment) | G5 canonical | Owner re-issues | Whether the SEC-2 amendment of 17 Sep 2026 stands |
| `05` plan | G7 only (session store, `dd7923d8…`, unversioned) | Import G7 **unchanged** into the project as an unversioned original, with its hash | Owner supplies an authoritative version | Declare `05` superseded | Whether S1-11/S1-12 scope (`008_reconcile.sql`, `009_bootstrap_admin.sql`, "prints it once to logs") is the intended A7/A8 scope |
| `06` checklist | G8 only (session store, `2036e28a…`, unversioned; all 34 answer boxes unticked) | Import G8 unchanged, with its hash | Owner supplies an authoritative version | Declare `06` superseded by `07` | Whether A1–A10 and the B/C/D items are defined by G8 |

**Proposed invariants,** whichever option is chosen. These are proposals for the owner to accept or reject.
- **One approval log of record.** All other copies are marked "copy, not of record".
- **Append-only.** Records of record are append-only. Corrections are dated notes that keep the original text; this follows the existing D-14 amendment and KB-SEED-2 correction conventions.
- **Hash manifest.** Each canonical record's SHA-256 is recorded in a manifest when it is adopted.
- **Generated documents never become records.** Proposals, packets, checklists and briefings from Claude remain "PROPOSED" until the owner enters a decision in the log of record.

---

## 3. Technical baseline: current state and evidence categories

### 3.1 Code state

**[VERIFIED]**

| Layer | State | Governance status |
|---|---|---|
| Original code | `version1.zip` (`87351b55…b4d6`), which `07` records as the source of the "owner-approved copy" in the project folder | Recorded as owner-approved copy (16 Sep 2026) |
| A1 items applied | S1-04, S1-06, S1-08 and S1-09 files are present in the folder | A1 "Approved with conditions"; items "not validated" (see reconciliation §4) |
| GO-7, GO-8, GO-9, go.mod restore | Present | Recorded "approved" in `07` (17 Sep 2026) |
| `internal/knowledge` (catalog) | Present | Decisions D-INT-2, D-INT-3, KB-FMT-1 and KB-SEED-2 recorded "approved" (22 Sep 2026); implementation "recording approved" |
| `internal/detect/certexpiry` | Present | Recorded only in the session-store copy of `07` (G4). Implementation built on approved inputs |
| F6.1 Increments 1–3 (probe, intake, results, migration `009`, Helm wiring, OPS-1 remediation D1/D2) | Present | **All PROPOSED — NOT APPROVED** (Increment-3 decision record v0.1.1) |
| Migrations `001`/`005` | Edited in place relative to `version1.zip` | Unresolved (A7 conflict) |
| Protected files (`Makefile`, `ci.yml`, `cd.yml`) | Original, unpatched | `PROTECTED-FILES.patch` **not applied** |

**[ANALYSIS]** The working tree therefore mixes approved, conditionally approved and unapproved changes. A canonical technical baseline has to say which of these the owner accepts as the starting point for further work.

### 3.2 OPS-1 evidence baseline

**OPS-1 F6.1 Increment 3 validation: INCOMPLETE.**

Checked against `docs/validation/f61-ops-1-kind-validation.md` ("VR").

| Item | Evidence category | Status and exact figure | Source | Discrepancy with the owner's stated baseline |
|---|---|---|---|---|
| F: migration tests | Runtime test, **outside Kubernetes** (disposable PostgreSQL 16) | **25 passed, 0 failed** | VR §3.F | None |
| G: cleanup | Runtime check | PASS | VR §3.G | None |
| Chart schema checks | **Static**: `kubeconform -strict` on a **non-Helm** render (local text/template harness) | **26/26** (defaults) and **39/39** (F6.1 enabled) | VR §3.0 | **Clarification:** the stated baseline gives only 39/39. VR also reports 26/26, and both are on a stand-in render, not Helm output |
| Design checks | **Static** | **24 passed** | VR Appendix C | None |
| Revocation and rotation | Runtime test with real processor and probe binaries, **outside Kubernetes** | PASS | VR §3.C, §3.D | None ("replacement" = rotation in VR) |
| Remediation regression (2026-09-27) | Runtime unit and integration tests, **outside Kubernetes** | **277 passed, 0 failed, 0 skipped** (`go test -race`, disposable PostgreSQL 16.13) | VR §8.4 | **Not in the stated baseline.** Added here for completeness |
| Image builds, Helm dependency build/lint/template, kind cluster | — | **BLOCKED** by sandbox egress. kubectl and Helm unavailable; kind obtained and checksum-verified, but the node image pull was refused | VR §1, §3.0 | None (VR is more specific about which tools were missing) |
| Tests A–E | Kubernetes validation | **NOT RUN in Kubernetes** | VR §0, §8.7 | None |
| D1 TLS hostname | Runtime (Go TLS tests, real binaries), outside Kubernetes; plus static render checks | Fixed; **Kubernetes validation pending** | VR §8.2 | None |
| D2 migration Job egress policy | **Static only** (policy evaluation of a stand-in render) | Fixed; **CNI enforcement untested** | VR §8.2 | None |
| D4 connection-string exposure | Processor: runtime reproduction outside Kubernetes. Gateway: **source inspection only** | Processor reproduced; **gateway unconfirmed** | VR §8.6; Packet Addendum ADD-F6/F7 | None |
| P3 credential mismatch | **Source inspection** of the sub-chart | Confirmed from source; **not runtime verified** | VR §8.5 | — |
| Migration runner, seeded admin, migration history | Runtime characterization tests plus source inspection | **Unresolved governance questions** | MIG-1a; Packet and Addendum | None |
| Migration Job | Configuration (`f61.migrations.job.enabled: false`) | **Disabled**; must stay disabled in persistent environments | `values.yaml`; MIG-1a §6 | None |
| External runbook | **Unexecuted** steps | Not run | `docs/validation/f61-ops-1-external-runbook.md` | None |

**Evidence categories.** These are kept distinct in every document of this package:
1. Verified runtime tests outside Kubernetes.
2. Static checks.
3. Source inspection.
4. External runbook steps (unexecuted).
5. Kubernetes validation (**not executed**).

No unrun test is reported as passed.

---

## 4. Proposed adoption procedure (for owner approval)

1. The owner answers GOV-1 … GOV-5 in `ObserveX-Owner-Decisions-Register.md`.
2. The owner records the chosen baseline in the approval log of record:
   - one dated entry;
   - the chosen copy per record, with its SHA-256;
   - the code baseline;
   - the evidence baseline in §3.2.
3. Only then, and only on the owner's instruction, place the chosen copies in the chosen governance location **as new files**. Originals and competing copies stay where they are, unchanged, marked "not of record" in a separate note.
4. Re-verify every hash after placement.

---

## 5. Owner acceptance

Blank; the owner completes this.

- Governance baseline choice per record (`03` / `05` / `06` / `07` / `11`): ______________________
- Approval-log-of-record location: ______________________
- Technical baseline accepted (code state and evidence categories): ______________________
- Conditions: ______________________
- Approver: ______________________
- Date: ______________________
- Recorded in (log entry reference): ______________________

---

## 6. Decisions reserved for the owner

- The choice per record in §2, and the invariants.
- Acceptance of the technical baseline in §3.
- The adoption procedure in §4.

## 7. Known conflicts and unresolved evidence

- C-1 … C-8 in the reconciliation record.
- C13 is not shown to be satisfied.
- A7, A8, D-03, MIG-1 and MIG-1a are unapproved.
- OPS-1 A–E are not run in Kubernetes.

## 8. Exact next action required

**[OWNER]** Choose Option A, B or C for `07` (and for `11`), and name the approval log of record (GOV-1, GOV-2). No other baseline step can proceed without it.

*Proposed — not approved. Generated 2026-09-28. OPS-1 remains INCOMPLETE.*
