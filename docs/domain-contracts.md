# Domain contracts

Phase 0 defines two independent products that may share protocol concepts but never require each other.

## Shared language

- **Run**: immutable identity plus mutable execution status for one requested activity.
- **Event**: append-only, ordered observation about a run. Events describe what happened; they are not commands.
- **Artifact**: content-addressed or externally referenced output with media type, size and redacted metadata.
- **Replay**: read-only reconstruction from stored events and artifacts. Replay never invokes tools or external systems.
- **Adapter**: boundary translating a runtime/provider protocol into normalized events.
- **Action**: requested side effect. Commands and action payloads are outside the event envelope.

Shared identifiers are opaque strings. Within a run, event sequence numbers are strictly increasing integers. Consumers must tolerate unknown event types and payload fields.

## n0ding Bench

Bench evaluates a declared subject against a versioned suite.

### Inputs

- immutable suite revision and case revisions
- subject/provider configuration by non-secret reference
- explicit run parameters, seed when supported, and environment fingerprint
- scoring policy and evaluator revisions

### Outputs

- case attempts, normalized observations and scores
- latency, token and error measurements with units and provenance
- artifacts and a redacted export manifest

### Invariants

1. A run records the exact effective configuration after defaults, excluding secrets.
2. Scores include evaluator identity/version; absent evidence is not silently scored as zero.
3. Retry creates a new attempt and does not overwrite the previous attempt.
4. Cancellation prevents new work but preserves received evidence.
5. Replay is observational and cannot call providers, tools or evaluators.
6. “Reproducible” means inputs and evidence are preserved; identical model output is not promised.

Bench owns suites, cases, subjects, attempts and scores. It does not own agent routing or approvals.

## n0ding Dispatch

Dispatch observes and controls work delegated to external agent runtimes.

### Inputs

- task and dependency graph
- agent/capability declarations supplied by an adapter
- routing policy revision
- explicit approval policy

### Outputs

- assignments, attempts, messages, artifacts and terminal outcomes
- routing decisions with policy revision and reasons
- approval and control audit records

### Invariants

1. Dispatch does not claim ownership of an external agent until its adapter acknowledges assignment.
2. Every side-effecting action has an idempotency key and fencing token.
3. Approval binds actor, expiry and immutable action digest; changed input invalidates approval.
4. Ambiguous completion is `outcome_unknown`, never guessed success or failure.
5. Pause/cancel acknowledgement is distinct from the request to pause/cancel.
6. Replay cannot resend messages, assignments or tools.
7. Routing is policy-based until measured evidence supports an “intelligent” claim.

Dispatch owns its control-plane records. Agent runtimes remain authoritative for execution facts exposed through their adapters.

## Compatibility boundary

Both products may implement the shared event envelope in `schemas/event-envelope.schema.json`. Payload schemas are versioned by event type. No shared database, process, binary or installation is required. Cross-product integrations consume documented APIs/exports only.
