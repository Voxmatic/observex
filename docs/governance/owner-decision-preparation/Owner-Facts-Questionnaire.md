# ObserveX — Owner Facts Questionnaire (F-1 … F-7)

> **Owner decisions recorded 2026-09-28.** The owner, akash, answered F-1 … F-7 on 2026-09-28: F-1, F-2, F-4 and F-5 as confirmed facts; F-3, F-6 and F-7 as policy.
> - Each answer is recorded in the approval log of record, `docs/governance/record/07_Approval_Log.md`, entry **OD-05**. **The log is the record; this questionnaire only points to it.**
> - **No answer was inferred from the code or the records.** The answers below are the owner's words.
> - The "context" rows still quote the records. They are **not answers**.

| Field | Value |
|---|---|
| Document | `docs/governance/owner-decision-preparation/Owner-Facts-Questionnaire.md` |
| Version | v0.1.0 (proposed) |
| Date generated | 2026-09-28 |
| Origin of the seven facts | Owner Decision Packet v0.1.0 §13 (M1–M6, M8); Packet Addendum §E; proposed-baseline Owner Decisions Register §1 |

**Sources quoted for context:**
- `11_Remediation_Plan.md` (the item 2 question);
- MIG-1a v0.1.0;
- the Increment-3 decision record v0.1.1 (MIG-1);
- the validation report (VR) §3.F, §6, §8.5 and §8.6;
- Packet Addendum ADD-F3 … ADD-F7;
- `07` (the P-0 clarification, CI-5).

---

## Ground rules for collecting evidence

These protect your environments. They are not decisions.

1. **Read-only only.**
   - Do **not** run `scripts/db-migrate.sh`, `make db-migrate`, the Helm migration Job, or any migration against a database you intend to keep.
   - The recorded re-run effects re-create a deleted administrator, reassign users to `org-default`, and restore removed memberships (VR §3.F, F5).
2. **Never copy secret values.**
   - Record key **names**, never values: passwords, DSNs, tokens, password hashes.
   - Tools such as `helm get values` print secret values. Redact every value before saving any output.
3. **Authorized people only.** Only someone you authorize may query a persistent database, and only with read-only statements.
4. **Keep the evidence in one place.** Store it where you choose, and write that location in the "Evidence location" field.

---

## F-1: Persistent installations and database state

| Item | Content |
|---|---|
| **Information needed** | For **each** installation outside a throwaway test: <br>• environment name and purpose; <br>• deployment method: Helm, Docker Compose or other; <br>• database: bundled PostgreSQL sub-chart, Compose PostgreSQL, or external; <br>• database state: *fresh-complete*, *E1*, *E2*, *migrated by the interim runner*, *other* or *unknown*; <br>• whether it holds data that must be kept; <br>• whether a backup exists and a restore has been tested. <br>*E1* means a Docker init that stopped inside `001`. *E2* means the old tolerant `psql -f … \|\| true` loop. If there is no installation, say so explicitly. |
| **Why it matters** | It decides whether any adoption or upgrade path is needed at all, and for which history classes. The fresh, E1 and E2 upgrade tests passed only on disposable PostgreSQL 16 (VR §3.F). Any other state is untested. |
| **Decisions that depend on it** | MIG-1a Q1/Q2; AQ-2 (supported histories); AQ-3; P3; db-monitor (AQ-9); the A7 conflict |
| **Context from the records (not an answer)** | VR §3.F describes the two reproduced states: <br>• **E1:** "docker init stopped at 001:217 … 6 tables". <br>• **E2:** "17 ERROR lines; 27 tables; `slos` and `network_flows` missing". <br>A database migrated by the interim runner has `schema_migrations` rows `001…007, 009`. |
| **Evidence that would support the answer** | • An inventory of installations: Helm release names and namespaces, Compose hosts, database hosts. <br>• Per database, read-only: the list of `public` table names; the `schema_migrations` rows, if the table exists; the PostgreSQL version. Compare with the E1/E2 descriptions. <br>• Backup and restore records. |
| **Current status** | **Answered by the owner on 2026-09-28** (fact confirmed by the owner). Recorded in `docs/governance/record/07_Approval_Log.md`, entry **OD-05** |
| **Owner answer** | "There are NO persistent installations. ObserveX has only run on throwaway local environments. No database holds data that must be kept." |
| **Evidence location** | The owner's statement of 2026-09-28, recorded in entry **OD-05** of the approval log of record. No separate evidence artifact was supplied |
| **Owner confirmation (name, date)** | akash, 2026-09-28 |

---

## F-2: Was any database built from a hand-edited migration `001` (or `005`)?

| Item | Content |
|---|---|
| **Information needed** | Whether anyone ever edited a local copy of `internal/db/migrations/001_initial.sql` or `005_integrations_postmortems.sql` before applying it to a database that still exists. If so: which database, which edit, when, and whether the edited copy survives. |
| **Why it matters** | MIG-1 corrected `001` (the `window` column) and `005` (the `network_flows` key) **in place**. It relied on the claim that the original statements could not have succeeded. A hand-edited copy is exactly the case where they could have, possibly with different definitions. Neither in-place edits nor MIG-1a's hash adoption can detect that divergence. |
| **Decisions that depend on it** | The A7 conflict (in-place edits vs corrective migrations, `11` DB-1/DB-3); the MIG-1a Q1 hash baseline; AQ-2 |
| **Context from the records (not an answer)** | `11`, item 2: "The open question is whether **any environment has a database created from a manually edited copy**, which would make in-place edits unsafe. That's unknown, so please confirm." |
| **Evidence that would support the answer** | • Operators' accounts. <br>• Any surviving edited copies of `001`/`005`, hashed. <br>• Per persistent database, read-only catalogue queries: whether `slos` and `network_flows` exist, and their column and primary-key definitions. Compare them with the files in the repository. |
| **Current status** | **Answered by the owner on 2026-09-28** (fact confirmed by the owner). Recorded in `docs/governance/record/07_Approval_Log.md`, entry **OD-05** |
| **Owner answer** | "No existing database was built from a hand-edited 001 or 005." |
| **Evidence location** | The owner's statement of 2026-09-28, recorded in entry **OD-05** of the approval log of record. No separate evidence artifact was supplied |
| **Owner confirmation (name, date)** | akash, 2026-09-28 |

---

## F-3: Which PostgreSQL versions are supported?

| Item | Content |
|---|---|
| **Information needed** | • The list of PostgreSQL major versions ObserveX supports, for new installs and for existing installs. <br>• Whether the bundled Helm sub-chart's version is the only supported one. <br>• The PostgreSQL version of each persistent installation (links to F-1). |
| **Why it matters** | The migration evidence, and the claim that "the originals cannot have applied", were **executed on PostgreSQL 16 only**. Claims about other versions are untested. Tests required by A7 and MIG-1a would have to run on every supported version. |
| **Decisions that depend on it** | The A7 conflict; AQ-2; MIG-1a Q1 test matrix; the D-20 support matrix (PROPOSED) |
| **Context from the records (not an answer)** | • VR §1: PostgreSQL 16.13 was used for all executed tests. <br>• MIG-1 (Increment-3 record) cites behaviour "since PostgreSQL 8.4" and "since PostgreSQL 11" (**not executed**). <br>• VR §8.5: Bitnami `postgresql` 15.5.4, appVersion 16.3.0. |
| **Evidence that would support the answer** | • Your support statement. <br>• Per installation, a read-only `SELECT version();`. |
| **Current status** | **Answered by the owner on 2026-09-28** (policy set by the owner). Recorded in `docs/governance/record/07_Approval_Log.md`, entry **OD-05** |
| **Owner answer** | "Supported PostgreSQL = 16 only, for new installs, until D-20 says otherwise." |
| **Evidence location** | The owner's statement of 2026-09-28, recorded in entry **OD-05** of the approval log of record. No separate evidence artifact was supplied |
| **Owner confirmation (name, date)** | akash, 2026-09-28 |

---

## F-4: Does anything use the seeded administrator?

| Item | Content |
|---|---|
| **Information needed** | Per installation: <br>• whether the account `admin@observex.io` exists and is enabled; <br>• whether any person, script, integration or automation signs in as it or depends on it, for example through ownership of `org-default` or of API keys; <br>• whether its password has been changed from the value seeded by `001`. |
| **Why it matters** | Retaining, replacing or removing the seeded admin (MIG-1a Q4, A8) can lock users out, or leave a known-hash administrator in place. The current runner re-creates this account after deletion on every run (F5). |
| **Decisions that depend on it** | MIG-1a Q4; A8 (S1-12; C13-gated); AQ-4, AQ-5; operator guidance (AQ-11) |
| **Context from the records (not an answer)** | • `001` seeds the account with a fixed bcrypt hash. <br>• The documented password does not match it (VR §6, P5). <br>• No code reads `ADMIN_PASSWORD` (Packet Addendum ADD-F3). <br>• ADD-F11: the seeded admin is referenced as an owner. <br>**None of this shows whether anyone uses the account.** |
| **Evidence that would support the answer** | • Per installation, read-only: whether the account exists and is enabled. <br>• Whether its stored hash still equals the seeded one: compare in place, and **do not copy hashes into reports**. <br>• Authentication and audit logs of its sign-ins. <br>• Operators' and integration owners' statements. |
| **Current status** | **Answered by the owner on 2026-09-28** (fact confirmed by the owner). Recorded in `docs/governance/record/07_Approval_Log.md`, entry **OD-05** |
| **Owner answer** | "The seeded admin (admin@observex.io) is not used or relied on by any existing database, person or integration." |
| **Evidence location** | The owner's statement of 2026-09-28, recorded in entry **OD-05** of the approval log of record. No separate evidence artifact was supplied |
| **Owner confirmation (name, date)** | akash, 2026-09-28 |

---

## F-5: How do existing Helm installations set the database password?

| Item | Content |
|---|---|
| **Information needed** | Per Helm release: <br>• bundled PostgreSQL sub-chart or an external database; <br>• **which value keys were set** (names only): `secrets.postgresPassword`, `secrets.existingSecret`, `postgresql.auth.password`, `postgresql.auth.existingSecret`, any `postgresql.auth.secretKeys.*`; <br>• whether a `<release>-postgresql` Secret exists (name only); <br>• whether the gateway authenticates today, and if so how that was achieved; <br>• yes or no: whether any database password contains URL-reserved characters. |
| **Why it matters** | • **P3.** The sub-chart generates its own password unless given one, and no ObserveX workload receives it (VR §8.5; source inspection, not runtime verified). Either P3 option changes how an existing release sources its password, so a migration step depends on this answer. <br>• **D4.** Only passwords with URL-reserved characters trigger the leak (VR §8.6). |
| **Decisions that depend on it** | P3; D4 scope (AQ-10); db-monitor credential contract (AQ-9) |
| **Context from the records (not an answer)** | VR §8.5: "ObserveX sets `postgresql.auth.password: ""` and `existingSecret: ""`". The chart consumers read `observex-secrets`/`postgres-password`. |
| **Evidence that would support the answer** | • The value **key names** per release, with every value redacted. <br>• Secret object names per namespace (names only). <br>• Gateway start-up logs, for success or failure only. |
| **Current status** | **Answered by the owner on 2026-09-28** (fact confirmed by the owner). Recorded in `docs/governance/record/07_Approval_Log.md`, entry **OD-05** |
| **Owner answer** | "There are no persistent Helm releases, so no existing install has a password configuration to migrate." |
| **Evidence location** | The owner's statement of 2026-09-28, recorded in entry **OD-05** of the approval log of record. No separate evidence artifact was supplied |
| **Owner confirmation (name, date)** | akash, 2026-09-28 |

---

## F-6: Does the database superuser require a separate password?

| Item | Content |
|---|---|
| **Information needed** | • Your policy: a superuser password separate from the application user's; the same password; or the superuser account not enabled or used by ObserveX. <br>• Current practice in each existing installation (key names only). |
| **Why it matters** | P3 option 1 names separate keys for the admin password and the user password. The db-monitor credential scope (superuser, application user or a monitoring role) is undefined (ADD-F5). Credential management is D-08 (PROPOSED). |
| **Decisions that depend on it** | P3 (key layout); db-monitor (AQ-9); D-08 |
| **Context from the records (not an answer)** | VR §8.5 option 1: `secretKeys.userPasswordKey: postgres-password` "and a matching `adminPasswordKey`". |
| **Evidence that would support the answer** | • Your security policy or statement. <br>• Per-installation configuration key names. |
| **Current status** | **Answered by the owner on 2026-09-28** (policy set by the owner). Recorded in `docs/governance/record/07_Approval_Log.md`, entry **OD-05** |
| **Owner answer** | "The superuser password is separate from the application user's. ObserveX workloads never connect as superuser. db-monitor, if ever supported, uses a least-privilege monitoring role (pg_monitor), never superuser." |
| **Evidence location** | The owner's statement of 2026-09-28, recorded in entry **OD-05** of the approval log of record. No separate evidence artifact was supplied |
| **Owner confirmation (name, date)** | akash, 2026-09-28 |

---

## F-7: Who may authorize migrations against a real database?

| Item | Content |
|---|---|
| **Information needed** | • The named people or roles who may authorize running any migration, adoption run or migration Job against a persistent database. <br>• The environments each may authorize. <br>• The conditions required. For example: a verified backup and a tested restore; a dry-run count; a maintenance window. <br>• How each authorization is recorded (for example, one approval-log entry per run). |
| **Why it matters** | • MIG-1a §3.7 requires an operator to confirm that a backup exists before any persistent run. <br>• MIG-1a Q2 (adoption) needs a named authority. <br>• The Helm migration Job must stay disabled in persistent environments until an authority exists and the applicable decisions are recorded. |
| **Decisions that depend on it** | MIG-1a Q2; AQ-2; enabling the migration Job anywhere persistent |
| **Context from the records (not an answer)** | `07`, P-0 clarification, CI-5: "Rollback runs require the **approved per-run permission**". This is the nearest existing per-run rule, and it is scoped to rollback runs. |
| **Evidence that would support the answer** | Your statement, recorded in the approval log of record. |
| **Current status** | **Answered by the owner on 2026-09-28** (policy set by the owner). Recorded in `docs/governance/record/07_Approval_Log.md`, entry **OD-05** |
| **Owner answer** | "Only akash may authorize any migration, adoption run or migration Job against a persistent database. Each run needs: a verified backup and a tested restore; a dry-run count; its own approval-log entry. The migration Job stays disabled everywhere." |
| **Evidence location** | The owner's statement of 2026-09-28, recorded in entry **OD-05** of the approval log of record. No separate evidence artifact was supplied |
| **Owner confirmation (name, date)** | akash, 2026-09-28 |

---

## Summary

| Fact | Status | Blocks |
|---|---|---|
| F-1 | Answered (fact), OD-05 | MIG-1a Q1/Q2, AQ-2, AQ-3, P3, AQ-9 |
| F-2 | Answered (fact), OD-05 | A7 conflict, MIG-1a Q1, AQ-2 |
| F-3 | Answered (policy), OD-05 | A7 conflict, AQ-2 |
| F-4 | Answered (fact), OD-05 | MIG-1a Q4, A8, AQ-4, AQ-5 |
| F-5 | Answered (fact), OD-05 | P3, AQ-9, AQ-10 |
| F-6 | Answered (policy), OD-05 | P3, AQ-9 |
| F-7 | Answered (policy), OD-05 | MIG-1a Q2, AQ-2, migration Job |

**Next action.** None for the facts: all seven are answered (OD-05). The ground rules above still apply to any later evidence collection.

*Owner decisions recorded 2026-09-28 (approval log of record, entry OD-05). Questionnaire generated 2026-09-28. OPS-1 remains INCOMPLETE.*
