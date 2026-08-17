package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
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
	srv := httptest.NewServer(httpapi.New("bench", store))
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
	var out, errout bytes.Buffer
	var green struct {
		Passed  bool    `json:"passed"`
		Delta   float64 `json:"delta"`
		Minimum float64 `json:"minimum"`
	}
	if code := run([]string{"ci", "--url", srv.URL, "--baseline", "dogfood-baseline", "--candidate", "dogfood-baseline", "--min-delta", "0"}, &out, &errout); code != exitOK {
		t.Fatalf("green CI exit=%d; out=%s err=%s", code, out.String(), errout.String())
	}
	if err := json.Unmarshal(out.Bytes(), &green); err != nil || !green.Passed || green.Delta != 0 || green.Minimum != 0 {
		t.Fatalf("typed green result: %+v err=%v", green, err)
	}
	out.Reset()
	errout.Reset()
	junit := filepath.Join(t.TempDir(), "dogfood-regression.xml")
	code := run([]string{"ci", "--url", srv.URL, "--baseline", "dogfood-baseline", "--candidate", "dogfood-regressed", "--min-delta", "-0.25", "--junit", junit}, &out, &errout)
	if code != exitRejected {
		t.Fatalf("CI exit=%d, want %d; out=%s err=%s", code, exitRejected, out.String(), errout.String())
	}
	var red struct {
		Passed             bool                         `json:"passed"`
		Delta              float64                      `json:"delta"`
		Minimum            float64                      `json:"minimum"`
		Samples            int                          `json:"samples"`
		ConfigurationDelta map[string]map[string]string `json:"configuration_delta"`
	}
	if err := json.Unmarshal(out.Bytes(), &red); err != nil || red.Passed || red.Delta != -1 || red.Minimum != -0.25 || red.Samples != 2 || red.ConfigurationDelta["targets"]["baseline"] == red.ConfigurationDelta["targets"]["candidate"] {
		t.Fatalf("typed red result: %+v err=%v", red, err)
	}
	xmlEvidence, err := os.ReadFile(junit)
	if err != nil {
		t.Fatal(err)
	}
	var report junitSuite
	if err := xml.Unmarshal(xmlEvidence, &report); err != nil || report.Failures != 1 || report.Tests != 1 || report.Case.Name != "candidate-vs-baseline[baseline=dogfood-baseline,candidate=dogfood-regressed]" || report.Case.Failure == nil || report.Case.Failure.Text != "delta=-1 minimum=-0.25 samples=2" {
		t.Fatalf("typed JUnit: %+v err=%v", report, err)
	}

	events, status, err := do("GET", srv.URL+"/api/v1/runs/dogfood-regressed/events", "", nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("events: %d %v %s", status, err, events)
	}
	var eventList struct {
		Events []core.Event `json:"events"`
	}
	if err := json.Unmarshal(events, &eventList); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, event := range eventList.Events {
		if event.Type == "score.recorded" && event.Data["case"] == "capital" && event.Data["expected"] == "Paris" && event.Data["actual"] == "Lyon" && event.Data["scorer"] == "exact" && event.Data["passed"] == false {
			found = true
		}
	}
	if !found {
		t.Fatalf("typed score evidence absent: %+v", eventList.Events)
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
	post("/api/v1/replay/import", string(exported), http.StatusOK)
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
