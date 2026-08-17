package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hn-tran/n0ding-bench/internal/core"
)

func TestRunLifecycleAndModeIsolation(t *testing.T) {
	s := core.NewStore()
	h := New("bench", s)
	req := httptest.NewRequest("POST", "/api/v1/runs", strings.NewReader(`{"id":"one","name":"One"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if event, err := s.Append("one", "benchmark.completed", map[string]any{"password": "nope"}); err != nil || event.Data["password"] != "[REDACTED]" {
		t.Fatalf("append/redact: %+v %v", event, err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/runs/one/projection", nil))
	if !strings.Contains(w.Body.String(), `"status":"completed"`) {
		t.Fatalf("projection: %s", w.Body.String())
	}
	_ = s.CreateRun(core.Run{ID: "other", Mode: "other"})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/runs", nil))
	if strings.Contains(w.Body.String(), "other") {
		t.Fatalf("mode leak: %s", w.Body.String())
	}
}

func TestDefinitionsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bench.db")
	s, err := core.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	h := New("bench", s)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/datasets", strings.NewReader(`{"id":"persisted","name":"Persisted","version":"1","cases":[{"id":"c","input":"x","expected":"x"}]}`)))
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	s.Close()
	s, err = core.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w = httptest.NewRecorder()
	New("bench", s).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/datasets", nil))
	if !strings.Contains(w.Body.String(), `"id":"persisted"`) {
		t.Fatalf("definition lost: %s", w.Body.String())
	}
}

func TestEventsResumeJSON(t *testing.T) {
	s := core.NewStore()
	_ = s.CreateRun(core.Run{ID: "r", Mode: "bench"})
	a, _ := s.Append("r", "event.one", nil)
	b, _ := s.Append("r", "event.two", nil)
	h := New("bench", s)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/runs/r/events?after="+json.Number(string(rune('0'+a.ID))).String(), nil))
	var out struct {
		Events []core.Event `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 1 || out.Events[0].ID != b.ID {
		t.Fatalf("resume failed: %s", w.Body.String())
	}
}

func TestSSEResumesFromLastEventID(t *testing.T) {
	s := core.NewStore()
	_ = s.CreateRun(core.Run{ID: "r", Mode: "bench"})
	first, _ := s.Append("r", "event.one", nil)
	second, _ := s.Append("r", "event.two", nil)
	srv := httptest.NewServer(New("bench", s))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/v1/runs/r/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", json.Number(string(rune('0'+first.ID))).String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	rd := bufio.NewReader(resp.Body)
	var buf bytes.Buffer
	for {
		line, err := rd.ReadString('\n')
		buf.WriteString(line)
		if strings.HasPrefix(line, "data:") {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	got := buf.String()
	if strings.Contains(got, `"type":"event.one"`) || !strings.Contains(got, `"event_id":"`+strconv.FormatInt(second.ID, 10)+`"`) {
		t.Fatalf("bad resumed SSE: %s", got)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
}

func TestAuthModeIsolationAndCursorValidation(t *testing.T) {
	s := core.NewStore()
	_ = s.CreateRun(core.Run{ID: "other-only", Mode: "other"})
	h := NewAuthenticated("bench", s, "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/runs", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth: %d", w.Code)
	}
	req := httptest.NewRequest("GET", "/api/v1/runs/other-only/events", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-mode access: %d %s", w.Code, w.Body.String())
	}
	_ = s.CreateRun(core.Run{ID: "bench-only", Mode: "bench"})
	req = httptest.NewRequest("GET", "/api/v1/runs/bench-only/events?after=-1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor accepted: %d", w.Code)
	}
}

func TestSecretAbsentFromAPIAndExport(t *testing.T) {
	s := core.NewStore()
	h := New("bench", s)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/runs", strings.NewReader(`{"ID":"safe","Name":"sentinel-supersecret"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	if _, err := s.Append("safe", "run.started", map[string]any{"message": "sentinel-supersecret", "api_key": "sentinel-supersecret"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/runs/safe/events", "/api/v1/runs/safe/export"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if strings.Contains(w.Body.String(), "sentinel-supersecret") {
			t.Fatalf("secret in %s", path)
		}
	}
}

func TestScoreExpectedActualSentinelsRedactedFromStorageAPIAndExport(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bench.db")
	s, err := core.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	h := New("bench", s)
	post := func(path, body string, want int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	created := post("/api/v1/datasets", `{"id":"secret-d","name":"Secret","version":"1","cases":[{"id":"c","input":"q","expected":"sentinel-expected-secret"}]}`, 201)
	var createdDataset Dataset
	if err := json.Unmarshal(created, &createdDataset); err != nil || createdDataset.Cases[0].Expected != "[REDACTED]" {
		t.Fatalf("POST dataset not structurally redacted: %+v err=%v", createdDataset, err)
	}
	post("/api/v1/suites", `{"id":"secret-s","name":"Secret suite","version":"1","dataset_id":"secret-d","scorers":[{"kind":"exact"}]}`, 201)
	post("/api/v1/targets", `{"id":"secret-t","name":"Secret target","adapter":"fake","outputs":{"c":"sentinel-actual-secret"}}`, 201)
	post("/api/v1/bench/runs", `{"id":"secret-r","name":"Secret run","suite_id":"secret-s","target_ids":["secret-t"]}`, 201)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/datasets", nil))
	var listed struct {
		Datasets []Dataset `json:"datasets"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil || len(listed.Datasets) != 1 || listed.Datasets[0].Cases[0].Expected != "[REDACTED]" {
		t.Fatalf("GET datasets not structurally redacted: %+v err=%v", listed, err)
	}
	for _, path := range []string{"/api/v1/runs/secret-r/events", "/api/v1/runs/secret-r/export"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || strings.Contains(w.Body.String(), "sentinel-expected-secret") || strings.Contains(w.Body.String(), "sentinel-actual-secret") {
			t.Fatalf("sentinel leak in %s: %s", path, w.Body.String())
		}
	}
	for _, suffix := range []string{"", "-wal"} {
		raw, err := os.ReadFile(dbPath + suffix)
		if err != nil && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("sentinel-expected-secret")) || bytes.Contains(raw, []byte("sentinel-actual-secret")) {
			t.Fatalf("sentinel persisted in %s", dbPath+suffix)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEventBacklogIsBounded(t *testing.T) {
	s := core.NewStore()
	_ = s.CreateRun(core.Run{ID: "r", Mode: "bench"})
	for i := 0; i < maxSSEBacklog+1; i++ {
		_, _ = s.Append("r", "case.completed", map[string]any{"n": i})
	}
	w := httptest.NewRecorder()
	New("bench", s).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/runs/r/events", nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("unbounded backlog returned: %d", w.Code)
	}
}

func TestBenchDefinitionsRunAndComparison(t *testing.T) {
	h := New("bench", core.NewStore())
	post := func(path, body string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("POST %s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	post("/api/v1/datasets", `{"id":"d","name":"Dataset","version":"1","cases":[{"id":"c","input":"question","expected":"yes"}]}`, 201)
	post("/api/v1/suites", `{"id":"s","name":"Suite","version":"1","dataset_id":"d","scorers":[{"kind":"exact"}]}`, 201)
	post("/api/v1/targets", `{"id":"good","name":"Good","adapter":"fake","outputs":{"c":"yes"}}`, 201)
	post("/api/v1/targets", `{"id":"bad","name":"Bad","adapter":"fake","outputs":{"c":"no"}}`, 201)
	post("/api/v1/bench/runs", `{"id":"a","name":"A","suite_id":"s","target_ids":["bad"]}`, 201)
	post("/api/v1/bench/runs", `{"id":"b","name":"B","suite_id":"s","target_ids":["good"]}`, 201)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/comparisons?baseline=a&candidate=b", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"delta":1`) || !strings.Contains(w.Body.String(), `"configuration_delta"`) || !strings.Contains(w.Body.String(), `"missing_treatment":"zero"`) {
		t.Fatalf("comparison: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/targets", nil))
	if strings.Contains(w.Body.String(), `"outputs"`) {
		t.Fatalf("target outputs leaked from listing: %s", w.Body.String())
	}
}

func TestBenchInputLimitsAndCancelTerminal(t *testing.T) {
	h := New("bench", core.NewStore())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/datasets", strings.NewReader(`{"id":"d","name":"x","cases":[]} trailing`)))
	if w.Code != 400 {
		t.Fatalf("trailing JSON accepted: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/runs", strings.NewReader(`{"id":"r","name":"R"}`)))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/runs/r/cancel", nil))
	if w.Code != 409 {
		t.Fatalf("cancel: %d %s", w.Code, w.Body.String())
	}
}

func TestCrossSiteMutationRejected(t *testing.T) {
	h := New("bench", core.NewStore())
	req := httptest.NewRequest("POST", "/api/v1/datasets", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-site mutation=%d %s", w.Code, w.Body.String())
	}
}

func TestMutationRateLimit(t *testing.T) {
	h := New("bench", core.NewStore())
	var w *httptest.ResponseRecorder
	for i := 0; i < 121; i++ {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/datasets", strings.NewReader(`{}`)))
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit=%d %s", w.Code, w.Body.String())
	}
}

func TestDatasetCSVImportEndpoint(t *testing.T) {
	h := New("bench", core.NewStore())
	req := httptest.NewRequest("POST", "/api/v1/datasets/import?id=csv&name=CSV&version=1&format=csv", strings.NewReader("id,input,expected\na,q,x\n"))
	req.Header.Set("Content-Type", "text/csv")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"digest":"sha256:`) {
		t.Fatalf("import=%d %s", w.Code, w.Body.String())
	}
}

func TestBenchRunCancellationStopsActiveWork(t *testing.T) {
	h := New("bench", core.NewStore())
	post := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
		return w
	}
	if w := post("/api/v1/datasets", `{"id":"d","name":"Dataset","version":"1","cases":[{"id":"c","input":"q","expected":"yes"}]}`); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	if w := post("/api/v1/suites", `{"id":"s","name":"Suite","version":"1","dataset_id":"d","scorers":[{"kind":"exact"}]}`); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	if w := post("/api/v1/targets", `{"id":"slow","name":"Slow","adapter":"fake","outputs":{"c":"yes"},"delay_ms":5000}`); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- post("/api/v1/bench/runs", `{"id":"cancel-me","name":"Cancel","suite_id":"s","target_ids":["slow"]}`)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		w := post("/api/v1/runs/cancel-me/cancel", ``)
		if w.Code == 202 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run never became cancellable: %d %s", w.Code, w.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case w := <-done:
		if w.Code != 201 || !strings.Contains(w.Body.String(), `"status":"cancelled"`) {
			t.Fatalf("cancelled run: %d %s", w.Code, w.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not stop provider work")
	}
}
