package bundle

import (
	"encoding/json"
	"github.com/hn-tran/n0ding-bench/internal/core"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestReplayCannotInvokeRecordedTargetEndpoint(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer provider.Close()
	s := core.NewStore()
	if err := s.CreateRun(core.Run{ID: "offline", Mode: "bench"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("offline", "target.manifest", map[string]any{"adapter": "openai-compatible", "endpoint": provider.URL}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("offline", "benchmark.completed", map[string]any{"failed": 0}); err != nil {
		t.Fatal(err)
	}
	raw, err := Export(s, "offline")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAndReplay(raw); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("offline replay invoked recorded provider %d times", got)
	}
}

func TestExportVerifyReplayAndTamper(t *testing.T) {
	s := core.NewStore()
	core.LoadFixture(s, "bench")
	raw, err := Export(s, "bench-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyAndReplay(raw); err != nil {
		t.Fatal(err)
	}
	for i := range raw {
		if raw[i] == 'b' {
			raw[i] = 'x'
			break
		}
	}
	if _, err = VerifyAndReplay(raw); err == nil {
		t.Fatal("tampered bundle accepted")
	}
}

func TestSequenceGapRejectedEvenWithUpdatedChecksum(t *testing.T) {
	s := core.NewStore()
	core.LoadFixture(s, "bench")
	raw, _ := Export(s, "bench-fixture")
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	b.Events[1].Sequence = 9
	b.Manifest.EventsDigest, _ = eventsDigest(b.Events)
	raw, _ = json.Marshal(b)
	if _, err := VerifyAndReplay(raw); err == nil {
		t.Fatal("sequence gap accepted")
	}
}
