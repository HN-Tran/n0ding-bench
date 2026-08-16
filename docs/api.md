# HTTP API

The API is versioned under `/api/v1`. Remote binds require a bearer token; local loopback is the default.

- `GET /healthz`
- `GET|POST /api/v1/datasets`
- `POST /api/v1/datasets/import?id=...&name=...&version=...&format=csv|jsonl`
- `GET|POST /api/v1/suites`
- `GET|POST /api/v1/targets`
- `POST /api/v1/bench/runs`
- `GET /api/v1/runs`
- `GET /api/v1/runs/{id}/projection`
- `GET /api/v1/runs/{id}/events` (JSON or SSE with `Accept: text/event-stream` and `Last-Event-ID`)
- `POST /api/v1/runs/{id}/cancel`
- `GET /api/v1/comparisons?baseline=...&candidate=...`
- `GET /api/v1/runs/{id}/export`
- `POST /api/v1/replay/import`

Mutating JSON endpoints require JSON bodies. Dataset imports accept CSV or JSONL. Requests, definitions, cases, targets, attempts, timeouts, concurrency, SSE backlog and replay bundles are bounded. Comparisons require two single-target runs using the same immutable suite digest; missing samples are scored as zero and reported explicitly.

Errors are JSON objects with an `error` field. `400` means invalid input, `401/403` authentication or origin rejection, `404` missing resource, `409` invalid state/conflict, and `413` an oversized body.
