package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hn-tran/n0ding-bench/internal/bundle"
	"github.com/hn-tran/n0ding-bench/internal/core"
	"github.com/hn-tran/n0ding-bench/internal/httpapi"
)

func TestInitAndStableUsageExit(t *testing.T) {
	var out, errout bytes.Buffer
	if got := run([]string{"init", "--db", filepath.Join(t.TempDir(), "bench.db")}, &out, &errout); got != exitOK {
		t.Fatalf("init=%d %s", got, errout.String())
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Fatalf("non-json output: %s", out.String())
	}
	out.Reset()
	errout.Reset()
	if got := run([]string{"unknown"}, &out, &errout); got != exitUsage {
		t.Fatalf("usage exit=%d", got)
	}
}

// TestDogfoodRegressionGate is the hermetic private vertical slice for the v0.1
// release gate. The fake adapter is deliberate: unlike a remote provider it is
// a supported, deterministic target whose exact response evidence is replayable.
func TestDogfoodRegressionGate(t *testing.T) {
	store := core.NewStore()
	providerCalls := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A replay import must never reach a target. All target execution in this
		// scenario happens synchronously on the two run-creation requests below.
		if r.URL.Path == "/api/v1/bench/runs" && r.Method == http.MethodPost {
			providerCalls++
		}
		httpapi.New("bench", store).ServeHTTP(w, r)
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	post := func(path, body string, want int) []byte {
		t.Helper()
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("POST %s: %d %s", path, resp.StatusCode, raw)
		}
		return raw
	}
	post("/api/v1/datasets", `{"id":"dogfood-data","name":"Dogfood exact answers","version":"1","cases":[{"id":"capital","input":"capital of France","expected":"Paris","metadata":{"owner":"release-gate"}},{"id":"sum","input":"2+2","expected":"4","metadata":{"owner":"release-gate"}}]}`, http.StatusCreated)
	post("/api/v1/suites", `{"id":"dogfood-suite","name":"Dogfood exact suite","version":"1","dataset_id":"dogfood-data","scorers":[{"kind":"exact"}]}`, http.StatusCreated)
	post("/api/v1/targets", `{"id":"baseline-target","name":"Baseline fixture","adapter":"fake","outputs":{"capital":"Paris","sum":"4"}}`, http.StatusCreated)
	post("/api/v1/targets", `{"id":"regressed-target","name":"Regressed fixture","adapter":"fake","outputs":{"capital":"Lyon","sum":"5"}}`, http.StatusCreated)
	post("/api/v1/bench/runs", `{"id":"dogfood-baseline","name":"Dogfood baseline","suite_id":"dogfood-suite","target_ids":["baseline-target"],"seed":17}`, http.StatusCreated)
	post("/api/v1/bench/runs", `{"id":"dogfood-regressed","name":"Dogfood regressed","suite_id":"dogfood-suite","target_ids":["regressed-target"],"seed":17}`, http.StatusCreated)
	if providerCalls != 2 {
		t.Fatalf("target execution count=%d, want 2", providerCalls)
	}

	var out, errout bytes.Buffer
	junit := filepath.Join(t.TempDir(), "dogfood-regression.xml")
	code := run([]string{"ci", "--url", srv.URL, "--baseline", "dogfood-baseline", "--candidate", "dogfood-regressed", "--min-delta", "-0.25", "--junit", junit}, &out, &errout)
	if code != exitRejected {
		t.Fatalf("CI exit=%d, want %d; out=%s err=%s", code, exitRejected, out.String(), errout.String())
	}
	if !strings.Contains(out.String(), `"delta":-1`) || !strings.Contains(out.String(), `"minimum":-0.25`) || !strings.Contains(out.String(), `"targets"`) || !strings.Contains(out.String(), `baseline-target`) || !strings.Contains(out.String(), `regressed-target`) {
		t.Fatalf("comparison lacks threshold/config evidence: %s", out.String())
	}
	xmlEvidence, err := os.ReadFile(junit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(xmlEvidence), `name="candidate-vs-baseline[baseline=dogfood-baseline,candidate=dogfood-regressed]"`) || !strings.Contains(string(xmlEvidence), `delta=-1 minimum=-0.25 samples=2`) {
		t.Fatalf("JUnit lacks named expected failure: %s", xmlEvidence)
	}

	events, status, err := do("GET", srv.URL+"/api/v1/runs/dogfood-regressed/events", "", nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("events: %d %v %s", status, err, events)
	}
	for _, evidence := range []string{`"case":"capital"`, `"expected":"Paris"`, `"actual":"Lyon"`, `"scorer":"exact"`, `"passed":false`, `"seed":17`} {
		if !strings.Contains(string(events), evidence) {
			t.Fatalf("events lack %s: %s", evidence, events)
		}
	}

	exported, status, err := do("GET", srv.URL+"/api/v1/runs/dogfood-regressed/export", "", nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("export: %d %v %s", status, err, exported)
	}
	var envelope bundle.Bundle
	if err := json.Unmarshal(exported, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Manifest.EventsDigest == "" || envelope.Manifest.ProjectionDigest == "" {
		t.Fatalf("export lacks checksums: %+v", envelope.Manifest)
	}
	beforeReplay := providerCalls
	post("/api/v1/replay/import", string(exported), http.StatusOK)
	if providerCalls != beforeReplay {
		t.Fatalf("offline replay invoked target: before=%d after=%d", beforeReplay, providerCalls)
	}
}

func TestCIThresholdGreenAndRed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"delta":-0.1,"samples":10}`))
	}))
	defer srv.Close()
	var out, errout bytes.Buffer
	junit := filepath.Join(t.TempDir(), "report.xml")
	if got := run([]string{"ci", "--url", srv.URL, "--baseline", "a", "--candidate", "b", "--min-delta", "-0.2"}, &out, &errout); got != exitOK {
		t.Fatalf("green=%d %s", got, errout.String())
	}
	out.Reset()
	errout.Reset()
	if got := run([]string{"ci", "--url", srv.URL, "--baseline", "a", "--candidate", "b", "--min-delta", "0", "--junit", junit}, &out, &errout); got != exitRejected {
		t.Fatalf("red=%d %s", got, errout.String())
	}
	raw, err := os.ReadFile(junit)
	if err != nil || !strings.Contains(string(raw), `failures="1"`) {
		t.Fatalf("bad junit %v %s", err, raw)
	}
}
