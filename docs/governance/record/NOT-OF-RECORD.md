# ObserveX — Copies Not of Record

**Established by:** approval log of record, entries OD-01 … OD-04 (2026-09-28).
**Rule (OD-01):** every copy below is "not of record". Do not edit or delete it; it is kept unchanged as history. The records of record are the files in `docs/governance/record/` listed in `MANIFEST.md`.

Paths:
- **[P]** is the project folder `C:\Users\91832\Documents\observex\`.
- **[S]** is Claude's session output store, folder `observex-plan/` (files delivered in earlier Claude sessions; outside the project folder).

## 1. Copies of records now held of record elsewhere

| Record | Copy (not of record) | SHA-256 | Bytes | Relation to the record of record |
|---|---|---|---|---|
| `07` approval log | [P] `Claude outputs\07_Approval_Log.md` (G3) | `681c147c1f82310f36c1f299a33c1a5ec0bc507283ec9291c11ae815555a916c` | 68,383 | Exact byte-prefix of G4; lacks the 2026-09-23 entry and every entry from 2026-09-28 |
| `07` approval log | [S] `observex-plan/07_Approval_Log.md` (G4) | `fb2beb0c485120cb9ab9eae7061c5f51758cc3e668763b949e8ecf703497b2c8` | 75,923 | The seed. Identical to the first 75,923 bytes of the record of record |
| `11` remediation plan | [P] `Claude outputs\11_Remediation_Plan.md` (G5) | `812c26b24b732a12eda71e715ff2cb2b7dd0119b118c27085dd35b3c73572205` | 19,329 | Differs in the SEC-2 row (pre-amendment) |
| `11` remediation plan | [S] `observex-plan/11_Remediation_Plan.md` (G6) | `9f961d90a01f265c1fe275c6db70740f604cdd8241a2d1a379c1bda4b3c3252e` | 19,395 | Source of the record copy; byte-identical to it |
| `03` decision register | [P] `Claude outputs\03_Engineering_Decision_Register.md` (G1) | `81441f356e5ba4f9866cf372cd544680d9d1fc45cfd60e238b3e35d291f39100` | 27,137 | Byte-identical to the record copy |
| `03` decision register | [S] `observex-plan/03_Engineering_Decision_Register.md` (G2) | `81441f356e5ba4f9866cf372cd544680d9d1fc45cfd60e238b3e35d291f39100` | 27,137 | Source of the record copy; byte-identical |
| `05` stabilization plan | [S] `observex-plan/05_Stabilization_Plan.md` (G7) | `dd7923d89f132f8be3516d5bac645a9a330eba05ff726dd91c003bc3aeac716d` | 16,484 | Source; byte-identical to the record copy |
| `06` approval checklist | [S] `observex-plan/06_Decisions_Requiring_Approval.md` (G8) | `2036e28a3ceea87179c885103c62f586897151e6ccac86ebea850654f7d651bf` | 5,884 | Source; byte-identical |
| A1 implementation plans | [S] `observex-plan/A1_Implementation_Plans.md` | `dcf61e005a55f784ccbe8365e5f809fbf1142ccd8f0a4101c79d62399d4514d2` | 34,332 | Source; byte-identical |
| `02` roadmap | [S] `observex-plan/02_Implementation_Roadmap.md` | `b5c827339d314a247acc082c9e28274d4bd2d2cd2a1b4179aaad4834c4500d9d` | 12,112 | Source; byte-identical |
| `08` staging checklist | [S] `observex-plan/08_Staging_Readiness_Checklist.md` | `e609daffe4b400137aad257576b3c32094b97935a5c511208859c514dbe6b1bc` | 4,702 | Source; byte-identical |
| `09` staging inventory form | [S] `observex-plan/09_Staging_Inventory_Form.md` | `1f10ac905ed7e5ef216b7e5efbeae32f283825a9e5ecfe6a2cb8efaed8f798da` | 7,045 | Source; byte-identical |
| `10` remote validation evaluation | [S] `observex-plan/10_Remote_Validation_Evaluation.md` | `8923e63d4bd34cdde4b117cee076f66a5ea0d2fd883fc5b9de5241dd003734b0` | 14,972 | Source; byte-identical |

## 2. Related files that are not copies and not records

| File | SHA-256 | Status |
|---|---|---|
| [P] `Claude outputs\PROPOSED-approval-log-entry.md` | `524ac089331792ebfead3edd7f503b1df0da349d75a4ccb48418e6cc378647a7` | A 2026-09-22 proposal for `07`. Its content was later recorded in `07` (D-14-E … KB-FMT-1 entries). Not a record |
| `docs/governance/proposed-baseline/*` (six files) | See that folder's `README.md` | Generated proposals. Not records |
| `docs/governance/owner-decision-preparation/*` (five files) | See that folder's `README.md` | Preparation documents. The owner fields in `Owner-Decision-Sheet.md` and `Owner-Facts-Questionnaire.md` now reference entries in the log of record; the log remains the record |

*Created 2026-09-28 under OD-01. No file listed here was edited, moved or deleted.*
