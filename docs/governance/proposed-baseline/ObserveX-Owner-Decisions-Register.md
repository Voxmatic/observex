# ObserveX — Owner Decisions Register (Proposed)

> **PROPOSED — NOT APPROVED.** This is a consolidated list of open owner decisions and required facts. It is **not** the approval log.
> - No decision here is taken, and every approval field is blank.
> - Recording a decision happens only in the approval log of record, by the owner.
> - Nothing here chooses a migration policy, credential ownership model, administrator recovery mechanism or security policy.

| Field | Value |
|---|---|
| Document | `docs/governance/proposed-baseline/ObserveX-Owner-Decisions-Register.md` |
| Version | v0.1.0 (proposed) |
| Date generated | 2026-09-28 |

**Sources inspected:**
- the reconciliation record's sources (§2 there);
- the six OPS-1 owner documents, O1–O6 (carried forward here, not replaced);
- `03`, `07` (both copies), `11` (both copies), `05`, `06`;
- MIG-1a v0.1.0 and the Increment-3 record v0.1.1;
- VR and the external runbook;
- `values.yaml`, `templates/secrets.yaml`, `templates/deployments.yaml`, `templates/_f61_helpers.tpl`, `templates/NOTES.txt`;
- `internal/db/store/store.go`, `services/db-monitor/main.go`, `services/processor/f61_intake.go`, `scripts/db-migrate.sh`, migrations `001`–`009`.

**Reference labels.**
- `GOV-n` and `CH-n` are local to this register. They are **not official decision IDs**.
- Existing IDs are used as they appear in the records: A1, A7, A8, C13, D-03, D-08, MIG-1, MIG-1a Q1–Q4, P3, D4, RP-1.
- `AQ-n` refers to the local labels of `ObserveX-F6.1-OPS-1-Owner-Decision-Packet-Addendum-v0.1.0.md`.

**Label key:**
- **[VERIFIED]** checked in files;
- **[RECORDED]** stated in a record;
- **[ANALYSIS]** inference;
- **[OWNER]** owner only.

---

## 1. The seven required facts

Only the owner can supply these. **Nothing is inferred from the existence of files.**

| # | Fact | Why it matters | Decisions that depend on it | Owner response |
|---|---|---|---|---|
| F-1 | Do persistent installations or databases exist, and what is their state (fresh, E1, E2, migrated by the interim runner, other, unknown)? | Decides whether any adoption or upgrade path is needed at all | MIG-1a Q1/Q2, AQ-2, AQ-3, P3, db-monitor | ______________________ |
| F-2 | Was any database built from a hand-edited migration `001` (or `005`)? | In-place edits and checksums cannot detect divergent histories | A7 conflict, MIG-1a Q1 (hash baseline), AQ-2 | ______________________ |
| F-3 | Which PostgreSQL versions are supported? | "The originals cannot have applied" was tested on PostgreSQL 16 only | A7 conflict, AQ-2 | ______________________ |
| F-4 | Does anything currently use the seeded administrator account? | Replacing or disabling it can lock users out | MIG-1a Q4, A8, AQ-4, AQ-5 | ______________________ |
| F-5 | How do existing Helm installations set the database password? | Decides the P3 migration step, and the D4 exposure | P3, D4 (AQ-10), db-monitor | ______________________ |
| F-6 | Does the database superuser require its own password? | P3 Option 1 key layout | P3, db-monitor | ______________________ |
| F-7 | Who may authorize migrations against a real database? | Adoption runs need a named authority | MIG-1a Q2, AQ-2 | ______________________ |

---

## 2. Governance decisions

### GOV-1: Canonical governance sources

- **Question.** Which copies of `03`, `05`, `06`, `07` and `11` are the records of record?
- **Verified facts:**
  - `07`: the project copy is an exact prefix of the session-store copy.
  - `11`: the two copies differ in one row.
  - `05` and `06` exist only in the session store, unversioned.
  - `03`: the two copies are identical.
- **Options.** Per record, Options A, B and C in `ObserveX-Canonical-Baseline-Proposal.md` §2.
- **Risks.** Deciding on the wrong copy silently drops or adds an approval-log entry (the 2026-09-23 detector record), or an amendment (SEC-2).
- **Dependencies.** None; this is first.
- **Evidence required.** The owner's own knowledge of which files they placed and accepted. The hashes are in the reconciliation record.
- **Authorization needed.** An owner statement per record, entered in the approval log of record.
- **Status.** Unresolved.
- **Owner fields.** Decision ______ · Conditions ______ · Approver ______ · Date ______ · Record ______

### GOV-2: Location of the approval log of record

- **Question.** Which file and location is the approval log of record?
- **Verified facts.** `PROPOSED-approval-log-entry.md` (2026-09-22) names `observex-plan/07_Approval_Log.md`, "outside the repository", under a standing instruction not to modify it. A copy also sits in the project folder's `Claude outputs/`.
- **Options:**
  - the project-folder copy;
  - the session-store copy;
  - a new governance location in the repository, seeded from an owner-chosen copy.
- **Risks.** Split records. An approval written to a non-record copy.
- **Dependencies.** GOV-1.
- **Evidence required.** An owner statement.
- **Authorization needed.** Owner.
- **Status.** Unresolved.
- **Owner fields.** Decision ______ · Approver ______ · Date ______

### GOV-3: A1 condition C13 (= Packet Addendum AQ-12)

- **Question.**
  - Does C13 block A2 only (its text), or A2–A10 (the "Remaining decisions" table)?
  - Has the A1 results review occurred?
  - Will the owner keep, amend or lift C13?
- **Verified facts.**
  - C13: "Don't proceed to A2 until A1 implementation and validation results are reviewed".
  - "A2–A10 | Not yet presented (blocked by C13 until A1 results are reviewed)".
  - **No review is recorded.** A1 item status is in the reconciliation record §4.
- **Options:**
  - keep C13 and perform the review first;
  - amend C13's scope;
  - lift C13 for specific items (for example A7/A8).
- **Risks.**
  - Presenting A7/A8 in breach of the owner's own gate.
  - Or blocking needed decisions indefinitely.
- **Dependencies.** GOV-1, GOV-4.
- **Evidence required.**
  - The A1 per-item implementation and validation evidence, against C6, C7, C10 and C11;
  - a dated owner review entry.
- **Authorization needed.** Owner.
- **Status.** C13 in force per `07`; not shown satisfied.
- **Owner fields.** Decision ______ · Scope ______ · Approver ______ · Date ______

### GOV-4: Unrecorded work items

- **Question.** What is the status of the artifacts with no `07` implementation or validation record?
  - S1-01 (SAML containment): artifacts in `Claude outputs/`, not in the repository.
  - S1-03 (A3, not approved): diffs only.
  - S1-08: in the repository, no `07` entry.
  - SEC-6b diffs.
  - QE-DEP scripts.
- **Options.** Per item: record as implemented; record as abandoned; or investigate.
- **Risks.**
  - The C13 review cannot be complete.
  - Unapproved A3 work may be mistaken for approved work.
- **Dependencies.** GOV-1.
- **Evidence required.** Per-item file lists and hashes, and the owner's account.
- **Authorization needed.** Owner.
- **Status.** Unresolved.
- **Owner fields.** Decision ______ · Approver ______ · Date ______

### RP-1: Hosted repository baseline (existing ID, "Pending clarification" in `07`)

- **Question.** What are the hosting platform, repository URL, default branch, and the commit matching `version1.zip`?
- **Verified fact.** `version1.zip` hash `87351b55…b4d6` matches `07`.
- **Risks.** No git history. Changes cannot be traced beyond hashes.
- **Authorization needed.** Owner answer.
- **Status.** Pending clarification.
- **Owner fields.** Answer ______ · Date ______

### GOV-5: Canonical baseline acceptance

- **Question.** Does the owner accept the technical baseline and evidence categories in `ObserveX-Canonical-Baseline-Proposal.md` §3?
- **Options.** Accept; accept with conditions; reject.
- **Dependencies.** GOV-1, GOV-2.
- **Status.** Unresolved.
- **Owner fields.** Decision ______ · Conditions ______ · Approver ______ · Date ______

---

## 3. Migration decisions

Full analysis is in Packet O1 §5 and §8, and Packet Addendum O4 §C.

| ID (source) | Question | Verified facts (summary) | Documented options | Key risks | Dependencies | Evidence required | Authorization needed | Status |
|---|---|---|---|---|---|---|---|---|
| **MIG-1a Q1** (MIG-1a §6) | Switch the interim runner from re-applying every file to once-only with a durable, atomic ledger, as a bridge until D-03? | The runner re-applies all files, with no lock. A re-run restores a deleted admin, reassigns org-less users, restores removed memberships and re-seeds dashboards (5 characterization tests executed) | (a) keep re-apply-all; (b) once-only atomic ledger; (c) defer to D-03 | Access-control and data-integrity effects on every run; the ledger alone does not establish history (AQ-2) | D-03, Q2, Q3, the A7 conflict, GOV-3 | MIG-1a §5 tests 3–12 (currently design only) | Owner entry in the log of record | PROPOSED — UNRESOLVED |
| **MIG-1a Q2** | May the one-time adoption run the existing 001/002 backfills once, behind a flag and a dry-run count, or must they be rewritten first (A7)? | E1/E2 upgrades lost 0 rows (executed, VR §3.F); adoption is design only | (a) run once behind flag and count; (b) rewrite first (A7) | The seed runs during adoption; irreversible without a backup | Q1 = (b), Q4, A7, F-1, F-7 | Dry-run count test; backup evidence | Owner, plus a named run authority (F-7) | PROPOSED — UNRESOLVED |
| **MIG-1a Q3** | How should `008` (A7) be handled once `009` is recorded? | No `008` file exists. `008` is reserved for A7 in `03`, `05` and `06` | Explicit lower-version apply; renumber A7; decide in D-03 | A gap fails closed under the once-only rule | A7, D-03, AQ-3 | — | Owner | PROPOSED — UNRESOLVED |
| **MIG-1a Q4 / A8** | Retain, replace or remove the seeded admin? | `001` seeds an admin with a fixed hash, owner of `org-default`; no code consumes `ADMIN_PASSWORD` | Retain; replace (A8/S1-12, `05`); remove (**no documented design**) | Lock-out; known-hash admin; logging of generated passwords (S1-12 as written) | A8, GOV-3, F-4, AQ-8 | Bootstrap tests; no-secret log tests | Owner (A8 is gated by C13) | Unresolved; A8 not presented |
| **A7 conflict** (MIG-1 item 1 vs A7 / DB-1 / DB-3) | Do the in-place edits to `001` and `005` stand, get replaced by corrective migrations, or wait for A7? | Three edits verified against `version1.zip`. `11` says the approach "is itself part of A7" | `11` DB-1: edit in place or add corrective migrations; DB-3: key change or drop partitioning | Undetected divergence; a hash baseline built on edited files | F-2, F-3, GOV-3, AQ-2 | Compatibility plan; tests on each supported PostgreSQL version | Owner approval of edits **and** of A7 | Unresolved; A7 not approved |
| **AQ-2** (label only) | Which migration histories are supported, and is a compatibility plan authorized? | E1/E2 have no ledger rows; hand-edited and unknown histories are undetermined | Partial only (MIG-1a §3.6; DB-1 corrective) | A ledger alone cannot establish identity | F-1 … F-3 | Plan and fixtures | Owner | Unresolved |
| **AQ-3** (label only) | How does A8's planned migration (`05`: `009_bootstrap_admin.sql`) relate to the existing `009_f61_tls_certificates.sql`? | Content independent; number collides; the runner keys on full basename | MIG-1a Q3 options | Identity and order ambiguity | AQ-1/GOV-1, AQ-2, Q4 | — | Owner | Unresolved |
| **D-03** (register) | Migration framework and canonical schema directions | PROPOSED in `03` (recommendation A: goose) | A goose; B golang-migrate; C Atlas; D switch database | The interim runner constrains the choice | A7 | — | Owner | PROPOSED |
| **PROTECTED-FILES.patch** | Apply the unapplied Makefile/CI/CD patch? | Not applied; the protected files are the original versions (verified) | Apply; revise; discard | It routes `make db-migrate` through the re-apply-all runner (MIG-1a §6 advises not to apply while Q1 is open) | MIG-1a Q1 | CI evidence | Owner | Not authorized |

**Owner fields for each row above.** Record in the log of record:
- decision;
- conditions;
- approver;
- date;
- record reference.

This register holds no approvals.

---

## 4. Credential, security and operations decisions

| ID (source) | Question | Verified facts | Documented options | Key risks | Dependencies | Evidence required | Authorization needed | Status |
|---|---|---|---|---|---|---|---|---|
| **P3** (VR §8.5) | Who owns the PostgreSQL credential: the sub-chart using the ObserveX Secret, or ObserveX consumers using the sub-chart Secret? | The sub-chart (`postgresql` 15.5.4) generates its own password; consumers read `postgres-password` from the ObserveX Secret. Source inspection only; **not runtime verified** | Option 1 `auth.existingSecret` + `secretKeys`; Option 2 consumers read `<release>-postgresql`/`password` | Authentication failure on fresh installs; existing-install migration step; shared superuser password | F-5, F-6, D-08, D4 | Runbook W-D in a real cluster; upgrade test | Owner | No decision record |
| **D4** (VR §8.6) + gateway (AQ-10) | Is D4 remediation in scope for the processor, the gateway (`store.DefaultConfig`), `maskDSN`, and the `POSTGRES_DSN` paths? | Processor leak reproduced locally. The gateway composes the URL without escaping; pgx `ConnectError` echoes the parsed host, user and database; **gateway not reproduced**. `maskDSN` logs the first 20 DSN characters | No governance options; engineering proposals only (VR §8.6) | Password fragments in logs and DNS | AQ-8, P3, F-5 | Special-character tests without external DNS; log-scan tests | Owner scope per path, **plus a security review**. A processor decision does not cover the gateway | No decision record |
| **db-monitor** (AQ-9) | Is db-monitor part of the supported Helm install? If so, what credential contract does it have? | Its Deployment is always rendered; it reads `POSTGRES_DSN` from `<fullname>-secrets` key `postgres-dsn`, which the chart never creates. The name ignores `secrets.existingSecret`. The code falls back to a built-in default DSN. **No documented contract** | None documented | The Pod cannot start (expected; unverified); undefined privileges | P3, D-08, F-5, F-6 | Chart checks; cluster run | Owner **plus** architecture and security review | Unresolved |
| **Seeded admin authentication and password ownership** (A8, AQ-4 … AQ-7) | Lifecycle, first credentials, recovery and rotation, and ownership of `secrets.adminPassword` | Chart requires `secrets.adminPassword`, injected as `ADMIN_PASSWORD`; **no code reads it**. `NOTES.txt` tells operators to use it; the README and the `001` comment name other passwords; the hash matches neither | A8/S1-12 design (`05`); status quo | Unknown working credential; false assurance | F-4, GOV-3, D-08, AQ-8 | Bootstrap and lifecycle tests | Owner (A8 gated by C13) | Unresolved |
| **No-secrets-in-logs** (AQ-8) | Is a broad prohibition on logging secrets (admin passwords, database passwords, DSN fragments) approved? | The only approved rule found: `07` P-1 condition 3, "Never log token values" (**internal service token scope**). `04` L2 "No secrets, tokens or PII in logs" is a baseline target, **not recorded as approved** | Adopt broad rule; keep scoped rule | S1-12 as written logs a generated password | — | Log-capture tests | Owner | **Not approved** |
| **Operator guidance** (AQ-11) | Correct `NOTES.txt`, the README login line and the `001` comment, and when? | Three contradictory statements (verified) | — | Operators misled | AQ-4 … AQ-6 | — | Owner | Unresolved |
| **D-08** (register) | Secrets management approach | PROPOSED in `03` | A External Secrets Operator; B Vault; C cloud KMS SDKs; D SOPS | — | — | — | Owner | PROPOSED |

---

## 5. Product-charter decisions

These relate to `ObserveX-Product-Charter-Full-Stack-Observability.md`.

| ID | Question | Status |
|---|---|---|
| **CH-1** (label only) | Does the owner adopt the charter as the *direction* for planning? This is **not** approval of any architecture choice in it. | Unresolved |
| **CH-2** (label only) | Does the owner accept the Phase 0–5 structure as the successor to, or as a mapping onto, the existing proposed Stages 1–7 in `02_Implementation_Roadmap.md` (session store)? | Unresolved |
| Existing register decisions the charter depends on | D-01 deployment model; D-02 service architecture; D-04 telemetry storage and query language; D-05 streaming; D-07 tenancy; D-09 sessions/auth; D-10 service identity; D-11 remediation model; D-12 mocked features; D-13 agent architecture; D-15 self-observability; D-16 retention; D-17 naming/claims; D-18 cost controls; D-19 availability; D-20 support matrix; D-21 compliance; D-22 cleanup | **All PROPOSED** (`03`) |
| Approved constraint | **D-14-E**: no external LLM runtime (approved 2026-09-22, `07`); also D-INT-2, D-INT-3, KB-FMT-1, KB-SEED-2 | Approved; the charter must comply |

---

## 6. Decisions reserved for the owner

All of §1–§5. **None is taken here.**

## 7. Known conflicts and unresolved evidence

- The reconciliation record, C-1 … C-8.
- C13 is not shown to be satisfied.
- Gateway D4 is unconfirmed.
- P3 is not runtime verified.
- OPS-1 A–E are not run in Kubernetes.

## 8. Exact next action required

**[OWNER]** Answer GOV-1 and GOV-2, then GOV-3. Supply facts F-1 … F-7.

*Proposed — not approved. Generated 2026-09-28. OPS-1 remains INCOMPLETE.*
