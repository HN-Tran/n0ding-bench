# Bench run state machine

## States

`created`, `queued`, `running`, `cancelling`, `succeeded`, `failed`, `cancelled`.

Terminal states are `succeeded`, `failed`, and `cancelled`.

## Transitions

| From | Trigger | To | Required record |
|---|---|---|---|
| created | validate and enqueue | queued | effective redacted config |
| created | validation fails | failed | structured validation error |
| queued | worker claims run | running | worker/lease identity |
| queued | cancel requested | cancelled | requester and reason |
| running | all required cases complete | succeeded | result summary |
| running | unrecoverable error | failed | error class and retained evidence |
| running | cancel requested | cancelling | cancellation request |
| cancelling | worker acknowledges stop | cancelled | acknowledgement |
| cancelling | work completes first | succeeded/failed | actual outcome |

Retries occur inside a run as new case-attempt records. Retrying an entire terminal run creates a new run linked by `retry_of`; terminal states never reopen.

## Recovery rules

- A lost worker lease returns unfinished work to `queued`; completed attempts remain append-only.
- Duplicate events are ignored by `event_id`; conflicting events at one sequence are integrity failures.
- Missing sequence numbers make a live projection `degraded` until backfilled. `degraded` is projection health, not a run state.
- On restart, state is rebuilt from committed records; partially persisted events are never exposed.
