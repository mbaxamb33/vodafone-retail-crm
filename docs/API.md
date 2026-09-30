# HTTP API v1

Base path `/api/v1`. JSON in and out. Timestamps are UTC RFC 3339; calendar dates (`due`, report ranges) are `YYYY-MM-DD` in the store's time zone (Europe/Bucharest). IDs are UUIDs. Request bodies are strict: unknown fields, trailing data and bodies over 32 KB are rejected.

## Authentication and security

- `POST /auth/login` sets `vf_session`: HttpOnly, SameSite=Strict, Secure when `APP_ORIGIN` is HTTPS. The server stores only a SHA-256 digest of the token.
- Every request other than `GET`/`HEAD` must send an `Origin` header equal to `APP_ORIGIN`, otherwise `403 ORIGIN_REJECTED`.
- Five failed logins for one email, or thirty from one IP address, within 15 minutes return `429 RATE_LIMITED`. A successful login resets the per-email count.
- Unknown emails, wrong passwords and disabled accounts all return `401 INVALID_CREDENTIALS`.
- All data is scoped to the signed-in user's store. Actor IDs always come from the session.

## Errors

Every response has an `X-Request-ID` header. Errors look like this:

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Verifică datele introduse.",
    "fields": {
      "phone": "Introdu un număr de telefon valid, de exemplu 0722 345 678."
    },
    "requestId": "4f0c…"
  }
}
```

| Status | Codes                                                                                                                      |
| ------ | -------------------------------------------------------------------------------------------------------------------------- |
| 401    | `UNAUTHORIZED`, `INVALID_CREDENTIALS`                                                                                      |
| 403    | `FORBIDDEN`, `ORIGIN_REJECTED`                                                                                             |
| 404    | `NOT_FOUND`, `CUSTOMER_NOT_FOUND`, `EMPLOYEE_NOT_FOUND`, `FOLLOW_UP_NOT_FOUND`, `OPPORTUNITY_NOT_FOUND`, `VISIT_NOT_FOUND` |
| 409    | `CONFLICT`, `INVALID_STAGE_TRANSITION`, `EMAIL_TAKEN`                                                                      |
| 413    | `PAYLOAD_TOO_LARGE`                                                                                                        |
| 422    | `VALIDATION_FAILED`, with `fields`                                                                                         |
| 429    | `RATE_LIMITED`                                                                                                             |
| 500    | `INTERNAL_ERROR` (details are logged with the request ID, never returned)                                                  |
| 503    | `UNAVAILABLE`                                                                                                              |

Lists return a page: `{ "items": [...], "total": 42, "offset": 0, "limit": 24 }`.

## Endpoints

### Session and reference data

| Method | Path             | Notes                                                                                                                                                 |
| ------ | ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| POST   | `/auth/login`    | `{ "email", "password" }` → `{ user, store, features }`. `email` is the login: a username such as `oana.boboc` or an email address, case-insensitive. |
| POST   | `/auth/logout`   | Revokes the session.                                                                                                                                  |
| GET    | `/auth/me`       | `{ user, store, features }`                                                                                                                           |
| POST   | `/auth/password` | `{ "currentPassword", "newPassword" }` (at least 10 characters). Revokes the user's other sessions.                                                   |
| GET    | `/catalog`       | `{ visitReasons, nextActions, productCategories, journeySteps, stages }`. Each catalog item is `{ kind, code, label }`.                               |
| GET    | `/users`         | Active members of the store.                                                                                                                          |

### Workspace and dashboards

| Method | Path                               | Notes                                                                                                                                                                                                                                                                                                                            |
| ------ | ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/workspace`                       | `{ users, customers, followUps, opportunities, today }`. The caller's open follow-ups plus those completed in the last 30 days, and active opportunities plus those closed in the last 90 days (store-wide for managers). `customers` holds owned customers, customers those records refer to and, for managers, pool customers. |
| GET    | `/employees/me/dashboard`          | Counts (`myCustomers`, `overdue`, `dueToday`, `upcoming`, `waiting`, `activeOpportunities`, `offersAwaiting`), `pipeline` by stage, `recentCustomers`, `recentlyAssigned` (last 14 days), `unreadNotifications`.                                                                                                                 |
| GET    | `/manager/dashboard?from=&to=`     | Managers only. Omit both dates for all time. `summary`, `funnel`, `stepIncidence`, `employees`: see Reporting below.                                                                                                                                                                                                             |
| GET    | `/manager/employees/{id}/activity` | `from`, `to`, `offset`, `limit` (≤200). A page of the employee's visits, newest first, with `summary` (that employee's report row) and `customers` (those the visits refer to). Managers, or the employee viewing themselves.                                                                                                    |

### Customers and visits

| Method | Path                        | Notes                                                                                                                                                                                                                                                                                                                                                                    |
| ------ | --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| GET    | `/customers`                | `phone` (exact number in any format, used to warn about existing customers), `q` (name without diacritics, or a phone fragment in any common format), `owner` (`me` or a user ID), `ownership` (`owned`, `pool`), `status` (`active`, `archived`), `sort` (`recent`, `name`, `newest`, `followup`), `offset`, `limit` (≤100). Anonymized customers are never listed.     |
| POST   | `/customers`                | `{ "phone", "name"?, "tags"? }` → 201. The phone number identifies the customer; the name is optional and never generated. Phones are normalized to E.164 (`0722 345 678` becomes `+40722345678`). Shared numbers are allowed; the app warns first. New customers start in the store pool.                                                                               |
| GET    | `/customers/{id}?offset=`   | `{ customer, lastVisit, visits (20, newest first), total, followUps, opportunities, ownershipHistory, audit }`. `audit` (latest 50) is empty unless the caller is a manager.                                                                                                                                                                                             |
| PATCH  | `/customers/{id}`           | Any of `name`, `phone`, `tags`, `status` (`active`/`archived`; managers only). Editable by the owner, by anyone while the customer is unowned, or by managers.                                                                                                                                                                                                           |
| POST   | `/customers/{id}/ownership` | `{ "ownership": "owned" }` claims a pool customer for the caller; `{ "ownership": "pool" }` returns one. Nobody hands a customer directly to someone else (403). Claiming a colleague's customer returns `409 CUSTOMER_ALREADY_OWNED`. Only the owner or a manager returns a customer to the pool. `crmctl deactivate-user` returns that person's customers to the pool. |
| POST   | `/customers/{id}/anonymize` | Managers only. Irreversible: clears the name, phone, tags and all free-text notes; counts and stages remain.                                                                                                                                                                                                                                                             |
| GET    | `/customers/{id}/visits`    | Paginated visit history.                                                                                                                                                                                                                                                                                                                                                 |
| POST   | `/customers/{id}/visits`    | The visit request below. → 201 `{ visit, opportunities, followUp, reminder, customer }`, all saved in one transaction.                                                                                                                                                                                                                                                   |
| PATCH  | `/visits/{id}`              | `{ "notes" }`. The author may edit within 7 days, managers at any time. The previous text is kept as a revision and the edit is audited.                                                                                                                                                                                                                                 |

Visit request:

```json
{
  "reasonCode": "billing",
  "steps": [0, 1, 3],
  "notes": "Revine după salariu.",
  "ownership": "owned",
  "resolution": { "type": "other", "status": "pending" },
  "reminder": { "due": "2026-10-30", "notes": "S-a rezolvat cazul?" },
  "nextAction": "thinking",
  "actionDetails": "",
  "agreedDate": false,
  "due": "",
  "opportunities": [
    { "product": "Red Unlimited", "category": "mobile", "estimatedValue": 65 }
  ]
}
```

- `steps`: required, distinct journey indices 0–7, stored as given. Missing steps are never inferred. `furthestStep` is derived. Step 3 is "Atragerea intenției comerciale".
- `resolution`: required when step 1 (Rezolvarea solicitării) was performed, and rejected otherwise. `{ "type": "invoice", "holder": "holder" | "other" }` (titular / netitular) or `{ "type": "other", "status": "resolved" | "unresolved" | "pending" }` (pending means a case is open).
- `reminder`: optional internal check, allowed only for an unresolved or pending other request. It needs a date from today on and a note, and creates a follow-up of kind `reminder` for the caller. It is not a date agreed with the customer.
- `nextAction`: the step agreed with the customer, a `next_action` catalog code: `none`, `thinking`, `consulting`, `comparing`, `not_interested`, `keep_in_touch`, `other`. Empty means `none`. `other` requires `actionDetails` (≤500 characters). `keep_in_touch` records contact consent on the visit, with its author and time in the audit log; it is not a general marketing permission.
- `agreedDate`: true only when a date was agreed with the customer. `due` is then required and cannot be in the past, and a follow-up of kind `agreed` is created. Without an agreed date nothing is scheduled, but the step is kept on the visit and on any new opportunity (`nextStep`).
- `reasonCode`: optional; must be a `visit_reason` catalog code.
- `ownership`: `keep` (the default) or `owned`, which claims a pool customer under the ownership rules above.
- `opportunities`: up to 5; each starts at `identified`. The agreed follow-up is linked only when exactly one opportunity is created.
- The visit updates the customer's last interaction, unarchives an archived customer and notifies the owner when a colleague records it.
- Visits include `details: { nextAction, nextActionLabel, actionDetails, contactConsent, agreedDate, due, resolution }`. Visits recorded before this model have an empty `nextAction`.

### Follow-ups and opportunities

| Method | Path                  | Notes                                                                                                                                                                                                                                                      |
| ------ | --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/follow-ups`         | `employee` (`me` by default; `all` or a colleague's ID for managers), `status` (`active` by default, `all`, `open`, `waiting`, `unreachable`, `done`), `due` (`overdue`, `today`, `upcoming`), `customerId`, `offset`, `limit` (≤200). Sorted by due date. |
| POST   | `/follow-ups`         | `{ "customerId", "type", "due", "employeeId"?, "opportunityId"?, "notes"? }` → 201. Only managers may assign others. A linked opportunity must be open, belong to the same customer and be writable by the caller.                                         |
| PATCH  | `/follow-ups/{id}`    | `{ "status": "done" }`, or `{ "status": "open" \| "waiting" \| "unreachable", "due": "YYYY-MM-DD" }`, optionally with `"notes"`. Allowed for the assignee or a manager. Completion is final; repeating it is a no-op.                                      |
| GET    | `/opportunities`      | `employee` (as for follow-ups), `stage` (`active` by default, `all`, or a stage), `customerId`, `offset`, `limit`.                                                                                                                                         |
| POST   | `/opportunities`      | `{ "customerId", "product", "category"?, "estimatedValue"?, "notes"?, "employeeId"? }` → 201.                                                                                                                                                              |
| PATCH  | `/opportunities/{id}` | Any of `stage`, `product`, `category`, `estimatedValue`, `notes`. Allowed for the owner or a manager. Won and lost opportunities return `409 INVALID_STAGE_TRANSITION` for stage or detail changes. Repeating the current stage records no event.          |

Follow-ups include `kind`: `agreed` (date agreed with the customer), `reminder` (internal check) or `task` (scheduled directly). An opportunity counts as having a next step when a follow-up is linked to it or its `nextStep` is set.

Stages: `identified`, `qualified`, `verification`, `presentation`, `offer`, `waiting`, `won`, `lost`, `paused`. Opportunities include `stageChangedAt` (for stale detection), `closedAt` and `sourceVisitId`.

### Experience follow-ups

The day after serving a customer, each employee checks how the visit went.

| Method | Path               | Notes                                                                                                                                                                                                                                       |
| ------ | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/experience`      | `{ items: [{ id, customerId, employeeId, day, status }] }`: one task per customer and store-local visit day in the last 7 days, never today and never before the feature started in the store. `status` is `open`, `unreachable` or `done`. |
| PATCH  | `/experience/{id}` | `{ "status" }`. `id` is `customerId:day`. `unreachable` keeps the task pending; `done` completes it and can be undone. Changes are audited.                                                                                                 |

The first notifications request of a day creates one `experience` notification when checks are waiting.

### Notifications

| Method | Path                  | Notes                                                                                                                                                                                                                                                    |
| ------ | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/notifications`      | `?unread=true` optional. `{ items (latest 50), unread }`. Kinds: `customer_assigned`, `customer_reassigned`, `customer_returned`, `followup_assigned`, `opportunity_assigned`, `opportunity_updated`. Actors are never notified about their own actions. |
| POST   | `/notifications/read` | `{ "ids": [...] }`; an empty list marks all as read.                                                                                                                                                                                                     |

### Health

`GET /health` (liveness) and `GET /ready` (checks the database connection) are unauthenticated.

## Reporting

`/manager/dashboard` numbers come only from recorded events. Nothing is typed in by hand.

- **Range figures** use store-local dates, from the start of `from` up to (not including) the day after `to`:
  - `summary`: `visits`, `customersHandled` (distinct customers visited), `newCustomers`, `opportunitiesCreated`, `offers` and `contracts`/`lost` (stage transitions into `offer`, `won` and `lost`; an opportunity reaching `offer` twice counts twice).
  - `employees`: the same figures per store member. Offers and contracts are credited to the opportunity's owner.
- **Current figures** ignore the range: `poolCustomers`, `activeOpportunities`, `followUpsDueToday`, `overdueFollowUps`, and per employee `portfolioCustomers`, `activeOpportunities`, `openFollowUps`, `overdueFollowUps`.
- **Funnel** (conversation progression): all visits, then visits whose furthest step reached Atenție și permisiune ("commercial conversation"), Verificare, Prezentare, Ofertă and Contractare. `rate` is the share of the previous level.
- **Step incidence** counts the visits that included each step. It shows how often each step happens, not a conversion rate.

Commercial outcomes (offers, contracts) come from opportunity stage events, separate from how far the conversation got.
