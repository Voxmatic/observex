# ObserveX — Decision Dependency Matrix

> **Proposed — not approved.** This matrix shows which decisions must come before others. It records no decision.
> - It approves no change.
> - It proposes no migration number.
> - It authorizes no execution against any persistent or production environment.
> - Where it orders items, that is a dependency analysis for the owner. The owner may reorder.

| Field | Value |
|---|---|
| Document | `docs/governance/owner-decision-preparation/Decision-Dependency-Matrix.md` |
| Version | v0.1.0 (proposed) |
| Date generated | 2026-09-28 |

**Sources:**
- `07` (both copies): A1 conditions, the RP records, the P-0 clarification;
- `06` (A7, A8), `05` (S1-11, S1-12), `03` (D-03, D-08), `11` (DB-1, DB-3);
- MIG-1a v0.1.0; the Increment-3 decision record v0.1.1 (MIG-1);
- the Owner Decision Packet and its Addendum (AQ-1 … AQ-12, ADD-F1 … ADD-F12);
- the validation report (VR) §7 and §8.8; the external runbook;
- `docs/governance/proposed-baseline/` (register and dependencies).

**Column key:**
- **C13-gated.** "Yes" means the item is, or belongs to, A2–A10. Under the "Remaining decisions" reading of C13 it is not to be presented until the A1 results review, unless the owner amends or lifts C13 (GOV-3).
- **Independent now.** "Yes" means it needs no earlier approval or technical evidence. "Partly" says which part.

---

## 1. Matrix

| Row | Decision (existing IDs) | Must be preceded by (decisions) | Needs facts or technical evidence | C13-gated | Independent now | Blocks | Status today |
|---|---|---|---|---|---|---|---|
| **M1** | **Canonical governance baseline:** copies of record for `07`, `11`, `05`/`06`, `03` (GOV-1a … 1d); log location (GOV-2); baseline acceptance (GOV-5) | GOV-1 and GOV-2: none. GOV-5: GOV-1, GOV-2, RP-1 | Your knowledge of which files you placed and accepted (hashes in the decision sheet) | No | **Yes** for GOV-1 and GOV-2. No for GOV-5 | Every later entry needs a log of record. A7/A8 definitions depend on GOV-1c | Unresolved |
| **M2** | **A1 condition C13:** scope, keep/amend/lift, satisfaction (GOV-3); unrecorded work (GOV-4) | Scope and keep/amend/lift: none. Satisfaction: GOV-1a (which log), GOV-4 | **Satisfaction needs documentary evidence:** a dated owner review entry; per-item A1 results against C6, C7, C11 (and C10, or a waiver); your statement on the recorded A1 holds (decision sheet §6.2) | n/a (this is the gate) | **Partly.** Scope and keep/amend/lift: yes. Satisfaction: no | Presenting A2–A10, including A7, A8 and D-03 | C13 in force; not shown satisfied |
| **M3** | **RP-1:** hosted repository baseline | None | Hosting facts; commit matching `version1.zip`, or a "no exact match" statement | No | **Yes** (a fact) | GOV-5; RP-3 CI; P-0 track 2 | Pending clarification |
| **M4** | **Migration `009` numbering conflict:** `05` S1-12 plans `009_bootstrap_admin.sql`; `009_f61_tls_certificates.sql` already exists (ADD-F1, AQ-3). Also: how a later `008` (reserved for A7) is applied once `009` is recorded (MIG-1a Q3) | GOV-1c (is `05` authoritative?); M2 (A8 and A7 are C13-gated); M5 (AQ-2); M10 (whether A8 needs a migration at all) | F-1 (whether any database has recorded `009_f61_tls_certificates`) | Yes (through A7 and A8) | No | A8 implementation; A7 implementation; D-03 adoption rules | Unresolved. **No number is proposed here** |
| **M5** | **A7 and migration-history authorization:** the in-place edits to `001`/`005` vs corrective migrations (`11` DB-1, DB-3); supported histories and compatibility plan (AQ-2) | GOV-1c; M2 (A7 is C13-gated) | **F-1, F-2, F-3.** A compatibility plan and fixtures per supported PostgreSQL version | **Yes** | No | MIG-1 item 1 (in-place edits) standing; MIG-1a Q2 and Q3; D-03 | Unresolved; A7 not approved |
| **M6** | **MIG-1** (PROPOSED, Increment-3 record) and **MIG-1a** Q1 (once-only atomic ledger), Q2 (one-time legacy adoption) | **MIG-1 item 1:** M5. **MIG-1 items 2–4** (interim runner, Job off by default): none, but D-03 stays open. **Q1:** MIG-1's status (it amends MIG-1). **Q2:** Q1 = once-only; Q4 (M10); M5; **F-1; F-7** | Q1: MIG-1a §5 tests 3, 4, 5, 6, 8, 9, 11 and 12 are design only. Tests 1, 2 and 7 exist in the repository; test 10 passes only in the validation harness. Q2: dry-run count evidence; backup evidence | Partly (Q2 touches A7 territory) | **Partly.** Q1 can be decided before Q2/Q3. Implementing Q1 alone leaves E1/E2 and a future `008` unhandled (Packet §5) | Enabling the migration Job anywhere persistent; `PROTECTED-FILES.patch`; OPS-1 test F re-run semantics | PROPOSED — UNRESOLVED |
| **M7** | **D-03** (migration framework, PROPOSED in `03`) and **`PROTECTED-FILES.patch`** (Makefile, CI/CD) | D-03: GOV-1c; M2 (`06` A7 = "S1-11 + D-03"); M5. Patch: M6 Q1 (MIG-1a §6: do not apply while Q1 is open); MIG-1 status; D-03 direction | Patch: CI evidence on a clean checkout (RP-3; P-0 CI-1 … CI-5) | **Yes** (D-03 through A7) | No | D-03: the long-term runner; MIG-1a is a bridge only. Patch: routes `make db-migrate` through the interim runner | D-03 PROPOSED; patch **not applied**, not authorized |
| **M8** | **P3:** PostgreSQL credential ownership. Option 1: sub-chart reads the ObserveX Secret. Option 2: consumers read the sub-chart Secret | None as decisions. Related: D-08 (PROPOSED) | **F-5, F-6.** Runtime confirmation of the mismatch (runbook §9, step W-D) is recommended **before** or with the decision; today it is source inspection only | No | **Partly.** The choice can be made. The existing-install migration step needs F-5 | M11 (db-monitor contract); D4 exposure on existing installs; OPS-1 install without the disposable workaround | No decision record |
| **M9a** | **D4, processor path:** password fragments in logs and DNS from the unescaped `OBSERVEX_F61_POSTGRES_DSN` | None | Reproduced locally (VR §8.6). F-5 (whether existing passwords carry URL-reserved characters) | No | **Yes** (scope). The fix design may follow M12 if a broad policy is adopted | Any D4 fix work on the processor | Not fixed; no decision record |
| **M9b** | **D4, gateway path:** `store.DefaultConfig` composition, `maskDSN`, `POSTGRES_DSN` paths (AQ-10; ADD-F6, ADD-F7) | Its own scope decision; a processor decision (M9a) **does not cover it** | **A security review.** Reproduction evidence (ADD-F6 is **unconfirmed**; ADD-F7 is static only). F-5 | No | No (needs a security review and evidence) | Any gateway D4 work | Unconfirmed; no decision record |
| **M10** | **Seeded administrator and A8:** retain, replace or remove (MIG-1a Q4); lifecycle (AQ-4); support (AQ-5); provisioning (AQ-6); recovery and rotation (AQ-7); operator guidance (AQ-11) | GOV-1c (`05` S1-12 authority); **M2** (A8 is C13-gated); **M12** (S1-12 as written prints a generated password to logs); AQ-4 before AQ-5 … AQ-7; AQ-11 after AQ-4 … AQ-6; D-08 | **F-4.** Bootstrap and lifecycle tests (none exist) | **Yes** (A8) | No | M4 (whether A8 needs a migration); M6 Q2 (adoption runs the seed) | Unresolved; A8 not presented |
| **M11** | **db-monitor credential provisioning** (AQ-9; ADD-F5): is db-monitor part of the supported Helm install, and what is its credential contract? | Inclusion: none. Contract: inclusion decision; M8 (P3); D-08 | **F-5, F-6.** A security and architecture review of the privilege scope | No | **Partly.** The inclusion decision can be made now. The contract cannot | Secret provisioning in real environments; the chart's unconditional db-monitor Deployment | Unresolved; no documented contract |
| **M12** | **No-secrets-in-logs policy** (AQ-8): a broad prohibition covering admin passwords, database passwords and DSN fragments | None | None | No | **Yes** | M10 (S1-12 as written conflicts with a broad rule); informs the M9a/M9b fix design | **Not approved.** Only `07` P-1 condition 3 ("Never log token values", internal-token scope) is approved |
| **M13** | **OPS-1 Kubernetes validation:** tests A–E, D1/D2 in-cluster, the in-cluster migration Job on a **throwaway** database | **For a run of the current code:** your authorization of an external disposable environment with the required egress or tooling (runbook §1). **For a run that validates post-decision code:** implementation of whichever of M6, M8 and M9 you approve | Authorized machine; CNI enforcement proof (runbook §5); real `helm template` output | No | **Partly.** Authorizing the environment is independent. Completion needs executed evidence | Declaring OPS-1 complete; staging (VR §7) | **INCOMPLETE:** A–E not run in Kubernetes |
| **M14** | **Enabling the migration Job in any persistent environment** | M6 Q1 (and Q2 for E1/E2 databases); **F-7** (named authority); F-1 | A verified backup (MIG-1a §3.7); D2 enforcement proven on that environment's CNI | Partly | No | — | **Disabled** (`f61.migrations.job.enabled: false`). Must stay disabled |
| **M15** | **Product charter direction** (CH-1, CH-2; local labels) | None | None | No | **Yes** | Planning only. Every architecture choice still needs its own D-decision | Unresolved |

---

## 2. Classification

### 2.1 Decisions you can make now, with no earlier approval or technical evidence

- **GOV-1a … GOV-1d and GOV-2:** sources of record and the log location (M1).
- **GOV-3:** C13 scope and keep/amend/lift. **Not** its satisfaction (M2).
- **RP-1:** a fact (M3).
- **F-1 … F-7:** facts; investigation can start at once (`Owner-Facts-Questionnaire.md`).
- **AQ-8:** the no-secrets-in-logs policy (M12).
- **D4 processor scope** (M9a).
- **db-monitor inclusion** in the supported install (first half of M11).
- **Authorizing an external, disposable OPS-1 environment** (first half of M13).
- **CH-1 and CH-2** (M15).

### 2.2 Decisions that need earlier approval

- **GOV-5:** after GOV-1, GOV-2 and RP-1.
- **C13-gated** (after GOV-3, unless C13 is amended or lifted): A7 and AQ-2 (M5), D-03 (M7), A8 and the seeded admin (M10), the `009`/`008` numbering (M4).
- **MIG-1a Q2** after Q1; **MIG-1a Q3** after A7 and D-03.
- **`PROTECTED-FILES.patch`** after MIG-1a Q1 and D-03 direction.
- **db-monitor contract** after inclusion and P3.
- **AQ-5 … AQ-7** after AQ-4; **AQ-11** after AQ-4 … AQ-6.
- **Enabling the migration Job anywhere persistent** (M14).

### 2.3 Items that need technical evidence, not only a decision

| Item | Evidence needed | Where it would come from |
|---|---|---|
| C13 satisfaction | Per-item A1 results (C6, C7, C11), plus your review entry | Your or CI runs in the P-0 evidence format |
| P3 runtime confirmation | Gateway authentication failure; the generated `password` in `<release>-postgresql` | Runbook §9, step W-D, in a disposable cluster |
| D4 gateway | Reproduction without external DNS, and log capture | Needs M9b scope first; a disposable, network-isolated environment |
| NetworkPolicy enforcement; D2 in-cluster | CNI deny test; policy matrix | Runbook §5 and test E |
| OPS-1 A–E | Executed Kubernetes evidence | External runbook §8 |
| MIG-1a behaviour | Tests 3 … 12 of MIG-1a §5 (design only today) | After Q1 is decided and implemented, on disposable databases |

---

## 3. Order of work

This extends step order 0–9 in `proposed-baseline/ObserveX-Decision-Dependencies-and-Authorization.md` with parallel tracks.

```
Track G (governance)    GOV-1 + GOV-2 ──► GOV-3 (scope / keep-amend-lift) ──► C13 evidence + GOV-4 ──► C13 review recorded
                              │                                                                            │
                              └──► RP-1 ──► GOV-5                                                          ▼
Track F (facts)         F-1 … F-7 (start now) ─────────────────────────────────► A7 / AQ-2 ──► M4 (008/009) ──► D-03
                                                                                  │
Track M (migrations)    MIG-1 status ──► MIG-1a Q1 ──► Q2 (needs F-1, F-7, Q4) ────┘──► patch decision ──► (Job stays disabled
                                                                                                          until M14 is met)
Track S (security)      AQ-8 (now) ──► A8 / AQ-4 … AQ-7 (C13-gated) ──► AQ-11
                        P3 (needs F-5, F-6; W-D evidence) ──► db-monitor contract (after inclusion)
                        D4 processor scope (now)      D4 gateway scope (needs a security review and evidence)
Track V (validation)    authorize a disposable environment (now) ──► CNI proof ──► A–G of the current code
                        ──► re-run after any approved P3 / MIG-1a / D4 change ──► record the results in VR
```

**Sequencing choice for you (not decided here).**
- VR §8.8 orders the work as owner decisions → engineering → external environment → Kubernetes validation.
- The runbook can also run against the **current** code in a disposable cluster: it works around P3 there, and the migration Job touches only a throwaway database. That would produce A–E evidence sooner, but it must be repeated after any approved change.
- Choosing between them is your decision.

---

## 4. What this matrix does not do

- It does not decide, rank or approve any item.
- It does not propose a migration number or renumbering.
- It does not authorize running anything against a persistent or production database.
- It does not treat the generated packets' questions about A7/A8 as presenting them under `07`.

**Next action.** **[OWNER]** Take the §2.1 decisions, starting with GOV-1 and GOV-2, and start the F-1 … F-7 investigation in parallel.

*Proposed — not approved. Generated 2026-09-28. OPS-1 remains INCOMPLETE.*
