# Hermetic regression dogfood gate

`TestDogfoodRegressionGate` is the v0.1 public-preview end-to-end evidence scenario.
It creates a content-addressed two-case dataset and exact-match suite, then runs
the supported deterministic fake adapter once with correct answers and once
with deliberately wrong answers. The declared minimum delta is `-0.25`; the
candidate delta is `-1`, so `n0ding-bench ci` must exit `4` and write a JUnit
failure naming both run IDs.

The same scenario checks case IDs, expected and actual values, scorer kind and
result, configuration delta, export event/projection checksums, and offline
import/replay. Replay verification accepts only bundle bytes and constructs a
fresh projection store; it has no target or adapter dependency and therefore no
execution boundary to invoke. This is a hermetic CLI/API release-gate check, not
UI coverage or evidence about remote-model quality. Run it with:

```bash
go test ./cmd/n0ding-bench -run '^TestDogfoodRegressionGate$' -v
```
