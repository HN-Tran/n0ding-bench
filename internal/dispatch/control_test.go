package dispatch

import (
	"testing"
	"time"
)

func TestApprovalMutationInvalidates(t *testing.T) {
	a := Action{Tool: "write", Target: "x", Arguments: map[string]string{"v": "1"}, PolicyVersion: "v1"}
	p := Approval{Digest: ActionDigest(a), Actor: "owner", Expires: time.Now().Add(time.Hour)}
	if !p.ValidFor(a, time.Now()) {
		t.Fatal("approval invalid")
	}
	a.Arguments["v"] = "2"
	if p.ValidFor(a, time.Now()) {
		t.Fatal("mutated action accepted")
	}
}
func TestIdempotencyFencingAndUnknown(t *testing.T) {
	c := NewController()
	calls := 0
	fn := func() (string, error) { calls++; return "ok", nil }
	c.ExecuteOnce("k", fn)
	c.ExecuteOnce("k", fn)
	if calls != 1 {
		t.Fatalf("executed %d", calls)
	}
	old := c.RenewLease("t")
	fresh := c.RenewLease("t")
	if c.Commit("t", old) == nil || c.Commit("t", fresh) != nil {
		t.Fatal("fencing failed")
	}
	if ClassifyTransportResult(true, false) != OutcomeUnknown {
		t.Fatal("ambiguous effect retried")
	}
}
