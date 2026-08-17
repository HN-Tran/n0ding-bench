# Hermetic regression dogfood gate

`TestDogfoodRegressionGate` is the private v0.1 end-to-end evidence scenario.
It creates a content-addressed two-case dataset and exact-match suite, then runs
the supported deterministic fake adapter once with correct answers and once
with deliberately wrong answers. The declared minimum delta is `-0.25`; the
candidate delta is `-1`, so `n0ding-bench ci` must exit `4` and write a JUnit
failure naming both run IDs.

The same scenario checks case IDs, expected and actual values, scorer kind and
result, seed/configuration delta, export event/projection checksums, and offline
import/replay. A call counter proves replay does not invoke the target. This is
a hermetic release gate, not evidence about the quality or reproducibility of a
remote model. Run it with:

```bash
go test ./cmd/n0ding-bench -run '^TestDogfoodRegressionGate$' -v
```
