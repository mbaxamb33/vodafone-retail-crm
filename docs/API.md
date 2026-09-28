# HTTP API v1

Base path: `/api/v1`. JSON requests and responses. Successful GET/create requests return resource JSON directly. Mutation acknowledgements return `{ "ok": true }`.

Authentication uses `vf_session`, an HttpOnly, SameSite=Strict cookie (Secure when APP_ORIGIN uses HTTPS). Sessions last eight hours. Mutations require an `Origin` matching `APP_ORIGIN`. Protected requests return 401 when unauthenticated; manager access is checked by the server. All customer operations are scoped to the session's store; actor IDs are derived from the session, never from mutation input.

| Method | Path                       | Request / response                                                                                                                                                                                       |
| ------ | -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| POST   | `/auth/login`              | Demo only: `{ "role": "employee" }` or `manager`; returns current user.                                                                                                                                  |
| GET    | `/auth/me`                 | Current user: id, name, role, storeId.                                                                                                                                                                   |
| POST   | `/auth/logout`             | Invalidates session.                                                                                                                                                                                     |
| GET    | `/workspace`               | Store users and customers; caller-owned followUps and opportunities, or store-wide for managers.                                                                                                         |
| POST   | `/customers`               | `{ "name": "Client Demo", "phone": "0722 000 999" }`; 201 customer. Shared phone numbers allowed.                                                                                                        |
| GET    | `/customers/{id}?offset=0` | customer, lastVisit (independent of pagination), visits (20 maximum, newest first), total, customer followUps and opportunities, latest 50 audit events (managers only).                                 |
| POST   | `/customers/{id}/visits`   | Visit request below; 201 acknowledgement after atomic persistence.                                                                                                                                       |
| PATCH  | `/follow-ups/{id}`         | `{ "status": "done" }` completes; `{ "status": "open"                                                                                                                                                    | "waiting" | "unreachable", "due": "2026-10-01" }` reschedules or records an outcome. Responsible employee or store manager only. Completed tasks cannot reopen; repeat completion is idempotent. |
| PATCH  | `/opportunities/{id}`      | `{ "stage": "offer" }`; responsible employee or manager. Won/lost opportunities cannot change stage.                                                                                                     |
| GET    | `/manager/dashboard`       | Optional `from=YYYY-MM-DD&to=YYYY-MM-DD` (both required together). Returns visits, newCustomers and opportunity stage events within inclusive store-local dates. No bounds returns all recorded history. |
| GET    | `/health`                  | Unauthenticated liveness.                                                                                                                                                                                |
| GET    | `/ready`                   | Unauthenticated readiness after startup repository load. Does not probe ongoing disk health.                                                                                                             |

Visit request:

```json
{
  "reason": "Reînnoire abonament",
  "steps": [0, 3, 5],
  "notes": "Clientul dorește să discutăm oferta la următoarea vizită.",
  "ownership": "owned",
  "nextAction": "Sună clientul",
  "due": "2026-10-01",
  "product": "Red Unlimited"
}
```

`steps` is a nonempty array of distinct zero-based journey indices 0–7. Missing intermediate steps are never inferred. `ownership` is `keep` (default when omitted), `owned` (current actor), `pool`, or `unassigned`. `keep` lets a colleague record a visit without changing ownership. An employee cannot reassign another employee's customer. `nextAction`, `product` and `notes` may be empty strings. A nonempty next action requires a valid calendar date. A product creates a distinct opportunity at `identified`, independent of visit steps. When the visit creates both an opportunity and follow-up, the follow-up is explicitly linked to that opportunity. Dates are store calendar dates; timestamps are server-generated UTC RFC3339.

Stages: `identified`, `qualified`, `verification`, `presentation`, `offer`, `waiting`, `won`, `lost`, `paused`.

All responses carry `X-Request-ID`. Error structure:

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Verifică datele introduse."
  }
}
```

Status codes: 401 UNAUTHORIZED, 403 FORBIDDEN, 404 NOT_FOUND, 409 CONFLICT, 422 VALIDATION_FAILED, 429 RATE_LIMITED, 500 INTERNAL_ERROR. Internal persistence errors are not returned to clients. Field-level error details and an OpenAPI document are future additions.

## Standalone relationship actions

`POST /customers/{id}/ownership` accepts `{ "ownership": "owned", "ownerId": "employee-id" }`, or `{ "ownership": "pool" }` / `{ "ownership": "unassigned" }`. An omitted ownerId for owned means the actor. Employees may assign only themselves and cannot change another employee’s ownership. Managers may choose any member of the same store. Ownership changes are atomic and audited; existing tasks retain their owners.

`POST /follow-ups` accepts `customerId`, optional `opportunityId`, optional `employeeId` (defaults to actor), `type` (nonempty, at most 100 characters) and `due` (YYYY-MM-DD). Only managers may assign a different employee. An opportunity link must reference an open opportunity for the same customer and be writable by the actor. Returns 201 with the follow-up.

Follow-ups include optional `opportunityId` and `completedAt`. Opportunities include `updatedAt`, representing the last stage change (legacy records may have a zero timestamp; clients fall back to createdAt). A repeated same-stage request produces no event and does not reset stage age.

Manager report `events` contains id, customerId, employeeId (the actor who recorded the transition), stage and at. Offer entries count transitions, not distinct opportunities. Initial seeded opportunity states are deliberately not converted into historical events. All-time and ranged activity stay separate from current workspace pipeline totals.
