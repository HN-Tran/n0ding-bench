# Security model

Datasets, prompts, provider responses, event payloads, replay bundles and browser content are untrusted input.

- The server binds to loopback by default.
- Non-loopback binding fails unless bearer authentication is configured; use TLS at a trusted reverse proxy.
- Secrets are redacted before SQLite/WAL, logs, SSE and export.
- Replay imports are size-limited, checksum-verified and read-only.
- Provider URLs and redirects must pass the target policy before any request.
- Browser output is rendered as text under a restrictive CSP.
- n0ding Bench does not execute imported bundles, arbitrary plugins or downloaded code.

The local single-user preview is not a hostile multi-tenant sandbox. It does not claim tamper-proof storage, exactly-once distributed execution or production-grade isolation.
