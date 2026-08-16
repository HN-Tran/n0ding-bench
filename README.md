# n0ding Lab

Private vertical prototype testing whether Bench and Dispatch can share compatible event/replay infrastructure without sharing domain vocabulary. It is intentionally not a public release or a production-readiness claim.

```bash
go test ./...
go run ./cmd/n0ding-lab -mode bench -db bench.db -addr 127.0.0.1:8080
go run ./cmd/n0ding-lab -mode dispatch -db dispatch.db -addr 127.0.0.1:8081
```

Open the corresponding URL and run the deterministic fixture. Each mode owns its SQLite-WAL database and recovers projections after restart.

Non-loopback binding fails closed unless `-auth-token` is supplied. The token is required for API requests in that mode. TLS is expected at a trusted reverse proxy for remote use.

Replay bundles are read-only JSON evidence envelopes:

- `GET /api/v1/runs/{id}/export`
- `POST /api/v1/replay/import`

The import verifies size, manifest, event checksum, run scope, and normalized projection digest. It reconstructs a projection without invoking providers, agents, or tools.

## Evidence and limitations

Automated tests cover SQLite restart recovery, pre-persistence redaction, separate databases, SSE resume, mode isolation, authenticated APIs, deterministic Bench scoring/comparison, Dispatch approval binding, idempotency, fencing, `outcome_unknown`, and replay tamper detection.

This remains a vertical prototype. It does not claim tamper-proof storage, exactly-once distributed execution, sandboxing, intelligent routing, full remote-model reproducibility, multi-user isolation, or production readiness.
