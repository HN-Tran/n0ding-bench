package bench

import (
	"testing"
	"time"
)

func TestEvaluateAndCompare(t *testing.T) {
	pass := Evaluate(Case{ID: "a", Expected: "yes", Mode: "exact"}, "yes", time.Millisecond, 1, 0)
	timeout := Evaluate(Case{ID: "b", Expected: "x", Mode: "contains", Timeout: time.Millisecond}, "x", 2*time.Millisecond, 1, 0)
	if pass.Score != 1 || timeout.Status != "timed_out" {
		t.Fatalf("unexpected results: %#v %#v", pass, timeout)
	}
	c := Compare("a", "b", []Result{{Score: .5}}, []Result{{Score: 1}})
	if c.Delta != .5 {
		t.Fatalf("comparison %#v", c)
	}
}
