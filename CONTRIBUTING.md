# Contributing

n0ding Bench accepts focused fixes, tests, documentation, and reproducible
evaluation evidence for the public preview. Open an issue before investing in a
large interface or scope change.

Run the complete local gate before opening a pull request:

```sh
go test ./...
go test -race ./...
go vet ./...
make build
sh scripts/package-smoke.sh
```

Do not commit provider credentials, evaluation data containing private prompts,
generated databases, or binaries. Report vulnerabilities through
[SECURITY.md](SECURITY.md), not a public issue. Contributions are submitted
under the repository's [Apache-2.0 license](LICENSE).
