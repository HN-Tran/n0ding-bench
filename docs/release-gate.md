# Private v0.1 release gate

The private v0.1 candidate is ready only when:

1. A clean machine reaches its first deterministic run in under ten minutes.
2. Fake and local OpenAI-compatible targets exercise pass, fail, malformed, timeout, retry and cancellation paths.
3. CLI, API and UI expose raw cases, scorer provenance, configuration deltas, missing samples and failure treatment beside aggregates.
4. Kill/restart reconciles committed work without losing or duplicating evidence.
5. Offline export/import reproduces the normalized projection without target invocation.
6. Sentinel secrets are absent from SQLite/WAL, logs, API, SSE and export.
7. Remote binding without authentication and hostile/tampered/oversized imports fail closed.
8. CI thresholds demonstrate an intentional green and red run with stable exit codes and JUnit evidence.
9. Tests, race check, vet, builds and package smoke tests pass on the exact candidate commit.

Passing this gate does not establish production readiness, universal model quality, statistical significance, tamper-proof storage or bit-identical remote reproducibility.
