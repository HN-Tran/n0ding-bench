package bench_test

import (
	"context"
	"errors"
	"github.com/hn-tran/n0ding-bench/internal/adapters"
	"github.com/hn-tran/n0ding-bench/internal/bench"
	"testing"
	"time"
)

func fixture(t *testing.T) (bench.Dataset, bench.Suite) {
	t.Helper()
	d, e := bench.NewDataset("demo", "1", []bench.DatasetCase{{ID: "a", Input: "x", Expected: "yes"}, {ID: "b", Input: "y", Expected: "42"}})
	if e != nil {
		t.Fatal(e)
	}
	s, e := bench.NewSuite("suite", "1", d.Digest, []bench.ScorerSpec{{Kind: "exact"}})
	if e != nil {
		t.Fatal(e)
	}
	return d, s
}
func TestRunRetryManifestAndComparison(t *testing.T) {
	d, s := fixture(t)
	target := &adapters.Fake{Name: "fake", Responses: map[string]string{"a": "yes", "b": "42"}, Failures: 1}
	r, e := bench.Run(context.Background(), d, s, target, bench.RunOptions{RunID: "one", Concurrency: 2, MaxAttempts: 2, Timeout: time.Second, Seed: 7})
	if e != nil {
		t.Fatal(e)
	}
	if !r.Manifest.Verify() {
		t.Fatal("manifest did not verify")
	}
	for _, c := range r.Cases {
		if c.Status != "completed" || c.Scores[0].Value != 1 {
			t.Fatalf("bad case: %#v", c)
		}
	}
	c := bench.CompareRuns(r, r)
	if c.Delta != 0 || c.Cases != 2 {
		t.Fatalf("bad comparison: %#v", c)
	}
}
func TestTamperAndTimeout(t *testing.T) {
	d, s := fixture(t)
	d.Cases[0].Input = "tampered"
	if d.Verify() == nil {
		t.Fatal("tamper accepted")
	}
	d, s = fixture(t)
	r, e := bench.Run(context.Background(), d, s, &adapters.Fake{Delay: 50 * time.Millisecond}, bench.RunOptions{RunID: "timeout", Timeout: time.Millisecond})
	if e != nil {
		t.Fatal(e)
	}
	if r.Cases[0].Status != "timed_out" {
		t.Fatalf("expected timeout: %#v", r.Cases[0])
	}
}

func TestCancellation(t *testing.T) {
	d, s := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := bench.Run(ctx, d, s, &adapters.Fake{Delay: time.Second}, bench.RunOptions{RunID: "cancel"})
	if err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestNonRetryableFailureStops(t *testing.T) {
	d, s := fixture(t)
	target := nonRetryTarget{}
	r, err := bench.Run(context.Background(), d, s, target, bench.RunOptions{RunID: "failed", MaxAttempts: 5})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range r.Cases {
		if c.Status != "failed" || c.Attempts != 1 {
			t.Fatalf("unexpected retry: %#v", c)
		}
	}
}

type nonRetryTarget struct{}

func (nonRetryTarget) Identity() bench.TargetIdentity {
	return bench.TargetIdentity{Kind: "test", Name: "failure"}
}
func (nonRetryTarget) Invoke(context.Context, bench.TargetRequest) (bench.TargetResponse, error) {
	return bench.TargetResponse{}, errors.New("permanent failure")
}
func TestScorers(t *testing.T) {
	tests := []struct {
		s    bench.ScorerSpec
		o    bench.Observation
		pass bool
	}{{bench.ScorerSpec{Kind: "contains"}, bench.Observation{Expected: "ell", Actual: "hello"}, true}, {bench.ScorerSpec{Kind: "regex", Pattern: "^h"}, bench.Observation{Actual: "hi"}, true}, {bench.ScorerSpec{Kind: "numeric_tolerance", Tolerance: .1}, bench.Observation{Expected: "1", Actual: "1.05"}, true}, {bench.ScorerSpec{Kind: "latency", Threshold: 10}, bench.Observation{Duration: 5 * time.Millisecond}, true}, {bench.ScorerSpec{Kind: "error_rate", Threshold: 0}, bench.Observation{Failed: true}, false}}
	for _, x := range tests {
		if got := bench.ScoreObservation(x.s, x.o); got.Passed != x.pass {
			t.Errorf("%s: %#v", x.s.Kind, got)
		}
	}
}
