# Vertical prototype acceptance scenarios

The prototype passes only when both domains start and operate independently. Evidence must be automated tests or captured API/database assertions, not screenshots alone.

## Shared event path

1. Start either product with a fresh database, create a run and observe live events over SSE.
2. Disconnect during the run, reconnect with the last event ID and receive every missing event exactly once in sequence.
3. Restart the process and reconstruct the same terminal projection from SQLite.
4. Export a run, import/read it in replay mode and obtain the same normalized projection without any provider, adapter or tool invocation.
5. Inject a sentinel secret in every input/output channel and prove it is absent from SQLite/WAL, logs, SSE and export.

## Bench slice

1. Run a deterministic fixture suite containing pass, fail, timeout and malformed-provider cases.
2. Show attempts, measurements and evaluator-versioned scores in the API and minimal UI.
3. Retry one failed case; preserve the original attempt and link the new attempt.
4. Cancel a running case; retain prior evidence and reach `cancelled` only after acknowledgement.
5. Compare two fixture subjects without claiming that stochastic output is identical.

## Dispatch slice

1. Observe three fixture agents and route dependent tasks using a named, versioned policy.
2. Display assignments, messages, artifacts and reasons from genuine emitted events.
3. Pause and resume a task with acknowledgement and a renewed fencing token.
4. Repeat a side-effecting command with one idempotency key and prove it executes once.
5. Simulate a lost response after dispatch; show `outcome_unknown` and prohibit blind retry.
6. Propose an approval-bound action, mutate one parameter and prove the prior approval is invalid.

## Operational gates

- Default startup listens only on loopback.
- Remote bind without configured authentication fails startup.
- Corrupt/conflicting event sequences produce an explicit integrity error, not a plausible projection.
- Slow SSE consumers cannot grow memory without bound.
- Bench can run with Dispatch absent, and Dispatch can run with Bench absent.

## Exit decision

Passing the scenarios authorizes implementation of Bench v0.1. It does not authorize production-readiness, intelligent-routing, fully-reproducible, high-availability or tamper-proof claims. Dispatch proceeds first as an observability/control layer; new routing intelligence requires a separately evidenced need.
