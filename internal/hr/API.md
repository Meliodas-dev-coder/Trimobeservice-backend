# Native HR API contract (v1)

Base path: `/api/v1/admin/hr`. Every endpoint requires an active admin account.
The umbrella permission `hr` passes every HR guard; restricted roles use the
domain capabilities documented below. JSON dates are `YYYY-MM-DD`, timestamps
are RFC3339 on input, and `DECIMAL` money/day/hour values are JSON strings.

Collection responses use `{ "<plural>": [], "meta": {"total", "page",
"limit"} }`; item responses use `{ "<singular>": {} }`. Lists accept `q`,
`page`, `limit` (max 500), and the resource's obvious foreign-key/status filters.

## Resources

All rows support `GET /resource` and `GET /resource/{id}`. Unless marked
otherwise, configuration rows support `POST`, `PUT|PATCH /{id}`. Historical
rows are retired by status/workflow instead of hard deletion.

| Capability | Resources |
|---|---|
| `hr_organization` | `departments`, `positions` |
| `hr_employees` | `employees`, `emergency-contacts` |
| `hr_documents` | private `documents` upload/download/metadata |
| `hr_compensation` | `contracts`, append-only `compensation`, `benefits`, `benefit-enrollments` |
| `hr_lifecycle` | append-only `lifecycle-events` |
| `hr_leave` | `leave-policies`, `leave-balances`, append-only `leave-requests`, `leave-calendar` |
| `hr_attendance` | `shifts`, `shift-assignments`, `attendance`, `timesheets` |
| `hr_performance` | `performance-reviews`, `goals`, `feedback`, `one-to-ones` |
| `hr_recruitment` | `vacancies`, `candidates`, `interviews`, `offers` |
| `hr_expenses` | `expenses` |
| `hr_dashboard` | workforce-only aggregate dashboard |

`employees.user_id` is the unique bridge to the employee's back-office login.
Only super-admin can link/provision it through `POST|PATCH
/employees/{id}/account`; generic employee creation never auto-links an email.
Department, position, manager, status, and end-date changes after creation must
use a lifecycle event.

## Workflow endpoints

- `POST /lifecycle-events`; `POST /lifecycle-events/{id}/complete`.
  Types: `onboarding`, `probation_started`, `probation_completed`, `transfer`,
  `promotion`, `offboarding`. Completion applies the movement atomically;
  offboarding closes contracts, shift assignments and benefits and releases
  future leave. Future-dated events cannot complete early.
- `POST /leave-requests` accepts `employee_id`, `policy_id`, `start_date`,
  `end_date`, optional `start_portion|end_portion` (`full|half`), `reason`, and
  private `document_id`. The server calculates working-day units, snapshots the
  policy approval chain, checks overlap/accrual/balance under row locks, and
  updates pending units. Actions: `/{id}/approve`, `reject`, `cancel`, body
  `{ "note": "..." }`. Manager/specific-user/HR approval steps are enforced in
  order. Item reads contain structured `approvals` history.
- `POST /attendance` and `PUT|PATCH /attendance/{id}` calculate worked, late,
  and overtime minutes from the assigned shift in the org timezone. `worked` is
  the clock-in→clock-out span minus the shift break (the break is only deducted
  once the span exceeds it, so a short session is not zeroed). `late` is arrival
  past the shift start + grace, on scheduled `work_days` only. `overtime` is time
  **worked beyond a standard 8-hour day** (not time past the shift's end clock).
  Lists accept `late=true` and `overtime=true`.
- **Self-service clock** (any account with a linked HR employee — no attendance
  create/update policy required, since it only ever records the caller's own
  time): `GET /attendance/me/today` returns the caller's record for today (org
  timezone) or `null`; `POST /attendance/clock-in` and `POST /attendance/clock-out`
  stamp `clock_in`/`clock_out` with the **server** time (so the numbers cannot be
  faked) and recompute worked/late/overtime via the same shift logic. One session
  per day; re-clocking returns `409`.
- **Attendance calendar**: `GET /attendance/calendar?start=&end=&department_id=`
  returns the employee × day grid the admin screen draws (`attendance_calendar`:
  `start`, `end`, `days[]`, and one `employees[]` row per person **in attendance
  view scope**, each carrying a cell for **every** day of the window). A cell has
  `scheduled` (from the shift assignment covering that date, falling back to a
  Monday–Friday week when the employee has none) plus the attendance facts when a
  record exists — so a scheduled past day with no record reads as a gap instead of
  disappearing. Rows come from `hr_employees` (offboarded excluded), not from the
  attendance table, so someone who never clocked in still has a line. Defaults to
  the fortnight ending today; ranges longer than 62 days are rejected `422`.
  Booked leave (`approved` and `pending`, past **and** future) rides along as a
  `leave` object on each covered scheduled day — `{status, policy, portion}`,
  `portion` being `half` on a half-day first/last day. It is resolved on its own
  authorization axis: the caller's **leave** scope is read separately and
  intersected with the attendance rows, so an admin who may not read leave simply
  gets a grid without it. A rest day inside a leave span stays a rest day (it
  consumes nothing), and an attendance record always outranks the plan.
- Timesheets: `POST /timesheets/{id}/submit|approve|reject`.
- Reviews: `POST /performance-reviews/{id}/complete`.
- One-to-ones and interviews: `POST /.../{id}/complete|cancel`.
- Vacancies: `POST /vacancies/{id}/submit|pause|complete|cancel`; complete means
  filled, cancel means closed.
- Candidates: repeated `approve` advances applied -> screening -> interview ->
  offer; `reject|cancel` terminate the application. `POST /candidates/{id}/hire`
  requires an accepted offer plus employee number/work email, then creates the
  employee, active contract, current compensation and vacancy counters in one
  transaction. Response: `{ "candidate": {}, "employee": {} }`.
- Offers: `POST /offers/{id}/submit|approve|reject|cancel`. Only offer-stage
  candidates may have one live offer; acceptance withdraws competing drafts.
- Expenses: `POST /expenses/{id}/approve|reject|reimburse`. Rejection requires
  `note`; reimbursement requires `payment_reference`. Every transition is
  append-only in the HR workflow history.

Terminal workflow records are frozen against generic edits.

## Private documents

`POST /documents/upload` is multipart with fields `file`, `employee_id`,
`document_type`, `name`, optional `expires_at`, `notes`, `is_confidential`.
PDF, DOCX, JPEG, PNG and plain text/CSV are accepted up to 5 MiB and stored in
the database, never through the public image bucket. Download:
`GET /documents/{id}/download` (`private, no-store`, nosniff). Effective
document policies and employee scope are enforced for every operation.

## Dashboard, lookups, reports and audit

- `GET /dashboard` -> `{dashboard:{...}}` with workforce totals and headcount by
  department/status. Other domains stay behind their own capabilities and reports.
- Cross-domain selectors (reduced fields): `/lookups/employees`,
  `/lookups/departments`, `/lookups/positions`. `hr_employees` additionally
  grants `/lookups/admin-users` for the optional login bridge.
- `GET /notifications?unread=true`; `PATCH /notifications/{id}/read`.
  Notifications are recipient-scoped; there is no HR traffic on the shared
  any-admin SSE stream.
- Reports: `workforce`, `leave`, `attendance`, `performance`, `recruitment`,
  `expenses`, `compensation` at `/reports/{name}?start=&end=`. CSV is the same
  name under `/exports/{name}`, with UTF-8 BOM, safe cells and Content-Disposition.
  Every report requires both `hr_reports` and its domain capability; workforce
  requires `hr_employees`.
- `GET /audit-history` requires `audit.view` with `all` scope (legacy `hr` and
  super-admin also qualify) and merges safe HTTP audit metadata with append-only
  workflow, expense, leave, and sensitive-read metadata. Tree/self audit scopes
  are rejected. Sensitive HR request bodies are never stored in global audit.

Seeded system role presets: HR Admin, HR Manager, HR Finance, HR Recruiter, and
HR Employee. Every linked active employee receives read-only self scope by
default, including complete own contracts and document downloads. Submission
or modification actions are not hard-coded self-service: super-admin must grant
them explicitly through a position policy when that workflow is enabled.
