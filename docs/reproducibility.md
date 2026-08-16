# Reproducibility and comparison

n0ding Bench records a reproducible configuration and the evidence observed during a run. It does not promise bit-identical output from remote or stochastic models.

Every run manifest records the Bench version, adapter and scorer versions, target/model identifiers, parameters, dataset and suite digests, seed when supported, concurrency, retry and timeout policies, start/end timestamps, and runtime information. Environment-variable names may be recorded; values and resolved secrets are never stored.

Comparisons always retain raw case outcomes. Aggregates identify their scorer, included and missing samples, failure treatment and sample count. A delta is descriptive evidence for the selected cases, not universal statistical proof.

Projection replay reads stored events only. An execution rerun creates a new run and re-applies current policy; importing or replaying evidence never calls a provider.
