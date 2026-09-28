# ObserveX — Decision Dependencies and Authorization (Proposed)

> **PROPOSED — NOT APPROVED.** This shows how the open decisions depend on each other, and what each future authorization would unlock.
> - It authorizes nothing.
> - Preparing it grants no engineering permission.

| Field | Value |
|---|---|
| Document | `docs/governance/proposed-baseline/ObserveX-Decision-Dependencies-and-Authorization.md` |
| Version | v0.1.0 (proposed) |
| Date generated | 2026-09-28 |

**Sources inspected:**
- the reconciliation record's sources;
- `07` (both copies): A1 conditions C1–C13, the P-0, P-1 and P-7 records, RP-1 … RP-7, and the gate summary;
- `03`;
- the session-store `02_Implementation_Roadmap.md` (Stages 1–7, proposed);
- MIG-1a;
- the owner documents O1–O6;
- VR and the runbook.

---

## 1. Dependency order

This extends the order in O4 §F. Each step lists its gate.

```
0. Governance source authority ─────────────┐   GOV-1, GOV-2 (and RP-1)
                                             ▼
1. A1 / C13 status ─────────────────────────┐   GOV-3, GOV-4
                                             ▼
2. Seven owner facts F-1 … F-7 ─────────────┐   Owner answers (register §1)
                                             ▼
3. Migration history, numbering, adoption ──┐   A7 conflict, AQ-2, AQ-3, MIG-1a Q1–Q4, D-03
   Credentials and security scope ──────────┤   P3, D4 (+gateway), db-monitor, AQ-4…AQ-8, D-08
                                             ▼
4. Owner decisions recorded in the approval log of record (append-only)
                                             ▼
5. Governance records updated through the authorized process (new versions, amendments)
                                             ▼
6. Implementation of the approved scope only
                                             ▼
7. Regression tests (disposable databases; no persistent data)
                                             ▼
8. External runbook on an authorized, disposable machine (Helm, images, kind, CNI proof)
                                             ▼
9. Actual A–E results recorded; OPS-1 status updated by evidence
```

**[RECORDED]** Constraints from the records that shape this order:
- **C13** (`07`). A2–A10, which include A7 (migrations) and A8 (seeded admin), are not yet to be presented until the A1 results review. Step 1 therefore precedes any A7/A8 decision, unless the owner amends C13.
- **P-0 / C11** (`07`). No production-ready claim until build, vet, tests and security validation pass.
- **P-7 / C10** (`07`). Staging validation before production; P-7 is "Pending clarification".
- **P-0 standing restriction** (`07`). No bypass of module-proxy or checksum verification (`GOPROXY=direct`, `GOSUMDB=off`, and similar).
- **MIG-1a §6.** While Q1 is open, the Helm migration Job stays disabled and `PROTECTED-FILES.patch` should not be applied.
- **`02` (session store, proposed).** Stages run one at a time, with entry and exit gates. Stage 3 (core architecture) needs D-02 … D-10 and D-19.

**Conflict noted, not resolved.**
- O1 §11 placed the A7 conflict first among *decisions*.
- O4 §F and this document place source authority, C13 and the owner facts first, as *prerequisites*.
- These are consistent if prerequisites are not counted as decisions. The owner may reorder.

---

## 2. What each authorization would unlock

| Authorization (when recorded) | Unlocks | Still blocked afterwards |
|---|---|---|
| GOV-1, GOV-2 | Placing canonical copies as new files; citing one log of record | All engineering |
| GOV-3 (C13 satisfied, amended or lifted) | Presenting A2–A10 (including A7, A8) as the owner directs | Implementation until each item is approved |
| MIG-1a Q1 (+ Q2, Q3), the A7 conflict and AQ-2 | Designing and testing a runner change and a compatibility plan on **disposable** databases | Any run on a persistent database (needs F-7 and a named authority); enabling the Job |
| Q4 / A8, AQ-4 … AQ-8 | Designing an admin lifecycle within the approved scope | Logging any secret; cluster verification |
| P3 | Chart credential wiring change | Runtime verification (runbook W-D) |
| D4 scope per path (+ security review) | Processor and/or gateway DSN and logging fixes | Anything not named in the scope |
| db-monitor contract | Chart and service changes for db-monitor | Secret provisioning in real environments |
| `PROTECTED-FILES.patch` | Applying or revising the patch | — |
| CH-1, CH-2 | Planning against the charter and roadmap | **Every architecture choice.** Each still needs its own register decision (D-01 … D-22) |

---

## 3. Authorization procedure (proposed)

This mirrors the conventions already in the records.

1. **Record.** The owner writes a dated entry in the approval log of record. Status is one of: **Approved · Rejected · Approved with conditions · Pending clarification · Blocked** (`07` vocabulary). The entry holds the answer as given, the conditions and the scope. "Silence is never counted as approval" (`07`).
2. **Append only.** Prior entries are never edited or removed. Corrections are dated notes that keep the original text (precedents: the `03` D-14 amendment; the `07` KB-SEED-2 correction).
3. **New versions.** Decision records change by new version (for example MIG-1a v0.1.1). Earlier versions are preserved.
4. **Proposed text.** Proposed record text prepared by Claude is a separate file marked "PROPOSED — NOT RECORDED" (precedent: `PROPOSED-approval-log-entry.md`). The owner instructs any append.
5. **Two approvals, never substituted.** Engineering review (code review, tests, security review) records whether an implementation meets an approved scope. It never substitutes for step 1.

---

## 4. Evidence required before OPS-1 can be called complete

**[RECORDED]** These are the original A–E definitions, kept in the runbook §8.

| Test | Required evidence | Current status |
|---|---|---|
| Prerequisites | Real image builds; `helm dependency build`, `lint`, `template`; kind cluster with CNI enforcement proven (runbook §5) | BLOCKED (VR §3.0) |
| A. Work delivery over TLS | Probe `work refreshed` with assignments; listener TLS; SAN = rendered name | NOT RUN in Kubernetes |
| B. TLS observation and reporting | `expiring` → `opened` event → renewal → `resolved` | NOT RUN in Kubernetes |
| C. Revocation | 401 and drop-all on the probe; pod unready; direct 401 | NOT RUN in Kubernetes (supplementary pass outside Kubernetes) |
| D. Rotation | Ready again without restart; old credential 401, new 200 | NOT RUN in Kubernetes (supplementary pass outside Kubernetes) |
| E. NetworkPolicy | Allow/deny matrix under a proven-enforcing CNI | NOT RUN |
| F. Migrations | In-cluster Job log; ledger | Local PASS (25/25); in-cluster BLOCKED; semantics unresolved |
| G. Cleanup | No residue | PASS (VR) |

**OPS-1 remains INCOMPLETE** until A–E pass in Kubernetes under the conditions above, and the results are recorded as a new dated section of VR.

---

## 5. Decisions reserved for the owner

- Every authorization in §2.
- Any reordering of §1.
- Adoption of the procedure in §3.

## 6. Known conflicts and unresolved evidence

- The reconciliation record, C-1 … C-8.
- C13 is not shown satisfied; its scope wording is disputed.
- The seven facts are missing.
- Gateway D4 is unconfirmed.
- P3 is not runtime verified.
- CNI enforcement is untested.

## 7. Exact next action required

**[OWNER]** GOV-1 and GOV-2 (sources and log of record), then GOV-3 (C13).

*Proposed — not approved. Generated 2026-09-28. OPS-1 remains INCOMPLETE.*
