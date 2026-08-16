package core

import "errors"

func LoadFixture(s *Store, mode string) (Run, error) {
	if mode != "bench" {
		return Run{}, errors.New("n0ding Bench supports bench mode only")
	}
	r := Run{ID: FixtureID(mode), Mode: mode, Name: "Deterministic " + mode + " scenario"}
	if old, ok := s.GetRun(r.ID); ok {
		return old, nil
	}
	if err := s.CreateRun(r); err != nil {
		return Run{}, err
	}
	var events []struct {
		typ  string
		data map[string]any
	}
	events = []struct {
		typ  string
		data map[string]any
	}{
		{"benchmark.started", map[string]any{"suite": "smoke/v1", "models": 2, "api_key": "sentinel-never-persist"}},
		{"case.completed", map[string]any{"case": "reasoning-1", "model": "alpha", "score": 0.8, "attempt": 1, "scorer_version": "exact/v1"}},
		{"case.completed", map[string]any{"case": "reasoning-1", "model": "beta", "score": 0.9, "attempt": 1, "scorer_version": "exact/v1"}},
		{"case.failed", map[string]any{"case": "malformed-provider", "attempt": 1, "error": "invalid JSON response"}},
		{"case.timed_out", map[string]any{"case": "slow-provider", "attempt": 1, "timeout_ms": 250}},
		{"case.retried", map[string]any{"case": "slow-provider", "attempt": 2, "retry_of": 1}},
		{"case.completed", map[string]any{"case": "slow-provider", "model": "beta", "score": 1.0, "attempt": 2, "scorer_version": "contains/v1"}},
		{"case.cancel_requested", map[string]any{"case": "cancelled-provider", "attempt": 1}},
		{"case.cancelled", map[string]any{"case": "cancelled-provider", "attempt": 1, "acknowledged": true}},
		{"benchmark.completed", map[string]any{"winner": "beta"}},
	}
	for _, x := range events {
		if _, err := s.Append(r.ID, x.typ, x.data); err != nil {
			return Run{}, err
		}
	}
	return s.GetRunOrZero(r.ID), nil
}

func (s *Store) GetRunOrZero(id string) Run { r, _ := s.GetRun(id); return r }
