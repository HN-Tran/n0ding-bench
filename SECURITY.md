# Security policy

n0ding Bench v0.1 is a public preview with best-effort security fixes and no
response-time SLA. Its safe default is loopback-only; remote binding requires
authentication and TLS should terminate at a trusted reverse proxy.

Do not open a public issue containing exploit details, credentials, private
prompts, datasets, model output, or user data. Use this repository's private
**Report a vulnerability** flow. Include the affected revision, a redacted
deployment description, reproduction steps, impact, and known mitigations.

Bench records evidence but is not a sandbox, secret store, multi-tenant service,
or supply-chain security control. Review [docs/security.md](docs/security.md)
and [docs/threat-model.md](docs/threat-model.md) before remote use.
