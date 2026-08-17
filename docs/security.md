# Security model

Datasets, prompts, provider responses, event payloads, replay bundles and browser content are untrusted input.

- The server binds to loopback by default.
- Non-loopback binding fails unless bearer authentication is configured; use TLS at a trusted reverse proxy.
- Values matching the documented built-in credential patterns (including
  bearer tokens, common API-key fields, `sk-...`, and sentinel test values) are
  redacted before definition responses, SQLite/WAL, logs, SSE and export.
- Ordinary dataset and prompt content is evidence, not confidential storage;
  arbitrary sensitive prose that does not match those patterns is not magically
  recognized as a secret and must not be submitted.
- `api_key_env` stores only a validated environment-variable name used to look
  up a credential server-side. That identifier is intentionally preserved;
  the environment variable's value is never stored in a target definition.
- Replay imports are size-limited, checksum-verified and read-only.
- Provider URLs and redirects must pass the target policy before any request.
- Browser output is rendered as text under a restrictive CSP.
- n0ding Bench does not execute imported bundles, arbitrary plugins or downloaded code.

The local single-user preview is not a hostile multi-tenant sandbox. It does not claim tamper-proof storage, exactly-once distributed execution or production-grade isolation.
