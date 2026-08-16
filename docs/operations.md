# Operations

Bench stores state in a SQLite database using WAL mode. Back up the database only with a SQLite-aware backup/checkpoint procedure or while the service is stopped. Keep exported replay bundles and artifacts under the same retention policy as the source prompts and responses.

On restart, committed events are integrity-checked and projections are reconstructed. Corrupt, gapped or conflicting event history fails closed instead of producing a plausible run. Interrupted trials must be reconciled explicitly and are never silently described as completed.

Default resource policy is deliberately bounded: request bodies, SSE backlog, concurrency, timeouts, retries and replay bundle size have explicit ceilings. Tune them only for a measured workload.

## Install and upgrade

Build with `make build`, copy `bin/n0ding-bench` into a directory on `PATH`, then run `n0ding-bench init --db /path/bench.db`. For an upgrade, stop the service, take a backup, replace the binary and start it again. The service validates stored history before accepting traffic. Roll back by stopping the service, restoring both the prior binary and its matching database backup, then restarting.

## Backup and restore

Stop Bench before copying `bench.db`, or use SQLite's online backup API. Do not copy only the main file while `bench.db-wal` is active. Restore while the service is stopped, retain the replaced files until validation succeeds, and run `n0ding-bench doctor` after startup.

Uninstall by stopping the service and removing the binary. Delete the database and replay exports only when their retention period has expired; they may contain prompts and model outputs even though recognized credentials are redacted.
