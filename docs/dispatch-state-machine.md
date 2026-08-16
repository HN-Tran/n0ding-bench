# Dispatch task state machine

## States

`created`, `ready`, `assigning`, `running`, `pause_requested`, `paused`, `cancel_requested`, `succeeded`, `failed`, `cancelled`, `outcome_unknown`.

Terminal states are `succeeded`, `failed`, `cancelled`, and `outcome_unknown`.

## Transitions

| From | Trigger | To | Guard/evidence |
|---|---|---|---|
| created | dependencies satisfied | ready | dependency snapshot |
| ready | routing decision | assigning | policy revision and reason |
| assigning | adapter acknowledges | running | assignment ID, fencing token |
| assigning | definite rejection | ready/failed | retry policy |
| assigning/running | transport ambiguity after side effect | outcome_unknown | last request and idempotency key |
| running | pause requested | pause_requested | requester |
| pause_requested | adapter confirms quiescence | paused | acknowledgement |
| paused | resume with valid lease | running | new fencing token |
| ready/assigning/running/paused | cancel requested | cancel_requested | requester |
| cancel_requested | adapter confirms cancellation | cancelled | acknowledgement |
| running | result accepted | succeeded | result/artifact references |
| running | definite failure | failed | normalized error |

An operator resolves `outcome_unknown` only by recording new evidence; resolution creates a linked task attempt and never rewrites history. Stale fencing tokens are rejected. Duplicate commands with the same idempotency key return the recorded outcome.

## Approval sub-state

A guarded action moves from `proposed` to `approved`, `rejected`, or `expired`. Approval is valid only if the SHA-256 digest of canonical action type, target and parameters matches, the approver is authorized, and expiry has not passed. Execution produces a separate audit event.
