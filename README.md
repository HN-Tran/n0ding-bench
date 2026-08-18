# n0ding Bench

[![CI](https://github.com/HN-Tran/n0ding-bench/actions/workflows/ci.yml/badge.svg)](https://github.com/HN-Tran/n0ding-bench/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
![Status](https://img.shields.io/badge/status-public%20preview-orange)

Local-first benchmark and evaluation harness for running, comparing, inspecting and replaying AI evaluations without hiding how scores were produced.

> **Public v0.1 preview.** The interfaces may change and the project is not
> production-ready. Use it for local evaluation and reproducible experiments.

## Quick start

```bash
go build -o n0ding-bench ./cmd/n0ding-bench
./n0ding-bench init --db bench.db
./n0ding-bench serve --db bench.db
```

Open <http://127.0.0.1:8080>. The deterministic fixture and fake target require no account or provider credential.

For a published container port, set `N0DING_BENCH_AUTH_TOKEN`; remote binding fails closed without it. The web UI asks for that token once and keeps it only in page memory.

CLI commands:

```text
n0ding-bench init
n0ding-bench serve
n0ding-bench run --file run.json
n0ding-bench runs
n0ding-bench export --run RUN_ID
n0ding-bench doctor
n0ding-bench ci --baseline RUN_ID --candidate RUN_ID --min-delta 0 --junit report.xml
```

The CLI emits JSON and uses stable exit codes: `0` success, `2` usage, `3` unavailable, `4` rejected and `5` internal failure.

## What v0.1 covers

- immutable dataset and suite versions with content digests;
- deterministic fake and OpenAI-compatible targets;
- exact, contains, regex, numeric-tolerance, latency and error-rate scoring;
- bounded concurrency, timeouts, retry classification and cancellation;
- case-level evidence, scorer provenance and explicit baseline comparison;
- SQLite-WAL recovery, resumable SSE and offline projection replay;
- checksummed export/import that never invokes a provider;
- embedded LIVE/REPLAY web UI;
- loopback-only default and fail-closed authenticated remote bind.

Bench records reproducible configuration and evidence, not guaranteed bit-identical remote-model output. Aggregates never hide raw failures or missing samples.

## Documentation

- [Reproducibility and comparison](docs/reproducibility.md)
- [Security model](docs/security.md)
- [Operations](docs/operations.md)
- [HTTP API](docs/api.md)
- [Threat model](docs/threat-model.md)
- [Event schema](schemas/event-envelope.schema.json)

Bench is independent. It does not require n0ding Cache or any agent-orchestration product.

## Contributing, security, and license

Focused contributions are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md).
Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).
n0ding Bench is licensed under [Apache-2.0](LICENSE).
