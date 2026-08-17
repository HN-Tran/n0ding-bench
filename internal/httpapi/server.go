package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hn-tran/n0ding-bench/internal/adapters"
	"github.com/hn-tran/n0ding-bench/internal/bench"
	"github.com/hn-tran/n0ding-bench/internal/bundle"
	"github.com/hn-tran/n0ding-bench/internal/core"
	webassets "github.com/hn-tran/n0ding-bench/web"
)

type Server struct {
	Mode     string
	Store    *core.Store
	Token    string
	mu       sync.RWMutex
	datasets map[string]Dataset
	suites   map[string]Suite
	targets  map[string]Target
	cancels  map[string]context.CancelFunc
	rateMu   sync.Mutex
	rates    map[string]rateWindow
}

type rateWindow struct {
	Started time.Time
	Count   int
}

type Dataset struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Version string              `json:"version"`
	Cases   []bench.DatasetCase `json:"cases"`
	Digest  string              `json:"digest,omitempty"`
}
type Suite struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Version   string             `json:"version"`
	DatasetID string             `json:"dataset_id"`
	Scorers   []bench.ScorerSpec `json:"scorers"`
	Digest    string             `json:"digest,omitempty"`
}
type Target struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Adapter   string            `json:"adapter"`
	Model     string            `json:"model,omitempty"`
	Endpoint  string            `json:"endpoint,omitempty"`
	APIKeyEnv string            `json:"api_key_env,omitempty"`
	Outputs   map[string]string `json:"outputs,omitempty"`
	DelayMS   int               `json:"delay_ms,omitempty"`
	Failures  int               `json:"failures,omitempty"`
}

const maxBodyBytes int64 = 1 << 20
const maxSSEBacklog = 1000
const maxDefinitions = 1000
const maxCases = 10000
const maxTargetsPerRun = 64

func New(mode string, store *core.Store) http.Handler {
	return NewAuthenticated(mode, store, "")
}

func NewAuthenticated(mode string, store *core.Store, token string) http.Handler {
	s := &Server{Mode: mode, Store: store, Token: token, datasets: map[string]Dataset{}, suites: map[string]Suite{}, targets: map[string]Target{}, cancels: map[string]context.CancelFunc{}, rates: map[string]rateWindow{}}
	s.loadDefinitions()
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", s.health)
	m.HandleFunc("GET /api/v1/runs", s.listRuns)
	m.HandleFunc("POST /api/v1/runs", s.createRun)
	m.HandleFunc("GET /api/v1/runs/{id}/events", s.events)
	if mode == "bench" {
		m.HandleFunc("GET /api/v1/datasets", s.listDatasets)
		m.HandleFunc("POST /api/v1/datasets", s.createDataset)
		m.HandleFunc("POST /api/v1/datasets/import", s.importDataset)
		m.HandleFunc("GET /api/v1/suites", s.listSuites)
		m.HandleFunc("POST /api/v1/suites", s.createSuite)
		m.HandleFunc("GET /api/v1/targets", s.listTargets)
		m.HandleFunc("POST /api/v1/targets", s.createTarget)
		m.HandleFunc("POST /api/v1/bench/runs", s.startBenchRun)
		m.HandleFunc("POST /api/v1/runs/{id}/cancel", s.cancelBenchRun)
		m.HandleFunc("GET /api/v1/comparisons", s.compareRuns)
	}
	m.HandleFunc("GET /api/v1/runs/{id}/projection", s.projection)
	m.HandleFunc("POST /api/v1/fixtures", s.fixture)
	m.HandleFunc("GET /api/v1/runs/{id}/export", s.exportRun)
	m.HandleFunc("POST /api/v1/replay/import", s.importReplay)
	m.Handle("GET /", http.FileServer(http.FS(webassets.FS)))
	return s.security(m)
}

func (s *Server) loadDefinitions() {
	for _, item := range []struct {
		kind string
		put  func(json.RawMessage)
	}{{"dataset", func(raw json.RawMessage) {
		var v Dataset
		if json.Unmarshal(raw, &v) == nil {
			s.datasets[v.ID] = v
		}
	}}, {"suite", func(raw json.RawMessage) {
		var v Suite
		if json.Unmarshal(raw, &v) == nil {
			s.suites[v.ID] = v
		}
	}}, {"target", func(raw json.RawMessage) {
		var v Target
		if json.Unmarshal(raw, &v) == nil {
			s.targets[v.ID] = v
		}
	}}} {
		rows, _ := s.Store.Definitions(item.kind)
		for _, raw := range rows {
			item.put(raw)
		}
	}
}

func validDefinition(id, name string) bool { return id != "" && len(id) <= 128 && len(name) <= 256 }
func decodeLimited(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return errors.New("exactly one JSON value required")
	}
	return nil
}

func (s *Server) listDatasets(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := make([]Dataset, 0, len(s.datasets))
	for _, x := range s.datasets {
		v = append(v, x)
	}
	write(w, 200, map[string]any{"datasets": v})
}
func (s *Server) listSuites(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := make([]Suite, 0, len(s.suites))
	for _, x := range s.suites {
		v = append(v, x)
	}
	write(w, 200, map[string]any{"suites": v})
}
func (s *Server) listTargets(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := make([]Target, 0, len(s.targets))
	for _, x := range s.targets {
		x.Outputs = nil
		v = append(v, x)
	}
	write(w, 200, map[string]any{"targets": v})
}
func (s *Server) createDataset(w http.ResponseWriter, r *http.Request) {
	var x Dataset
	if decodeLimited(w, r, &x) != nil || !validDefinition(x.ID, x.Name) || x.Version == "" || len(x.Cases) == 0 || len(x.Cases) > maxCases {
		write(w, 400, map[string]string{"error": "valid id, name, and 1..10000 cases required"})
		return
	}
	for _, c := range x.Cases {
		if c.ID == "" || len(c.Input) > 65536 || len(c.Expected) > 65536 {
			write(w, 400, map[string]string{"error": "invalid case"})
			return
		}
	}
	if err := redactDataset(&x); err != nil {
		write(w, 500, map[string]string{"error": "redact dataset"})
		return
	}
	versioned, err := bench.NewDataset(x.Name, x.Version, x.Cases)
	if err != nil {
		write(w, 400, map[string]string{"error": err.Error()})
		return
	}
	x.Digest = versioned.Digest
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.datasets) >= maxDefinitions {
		write(w, 409, map[string]string{"error": "dataset limit reached"})
		return
	}
	if _, ok := s.datasets[x.ID]; ok {
		write(w, 409, map[string]string{"error": "dataset already exists"})
		return
	}
	if err := s.Store.SaveDefinition("dataset", x.ID, x); err != nil {
		write(w, 500, map[string]string{"error": "persist dataset"})
		return
	}
	s.datasets[x.ID] = x
	write(w, 201, x)
}

func (s *Server) importDataset(w http.ResponseWriter, r *http.Request) {
	id, name, version := r.URL.Query().Get("id"), r.URL.Query().Get("name"), r.URL.Query().Get("version")
	if !validDefinition(id, name) || version == "" {
		write(w, 400, map[string]string{"error": "id, name, and version are required"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxCases)*(1<<20))
	defer r.Body.Close()
	var cases []bench.DatasetCase
	var err error
	switch strings.ToLower(r.URL.Query().Get("format")) {
	case "jsonl":
		cases, err = bench.ImportJSONL(r.Body)
	case "csv":
		cases, err = bench.ImportCSV(r.Body)
	default:
		err = errors.New("format must be jsonl or csv")
	}
	if err != nil {
		write(w, 400, map[string]string{"error": err.Error()})
		return
	}
	versioned, err := bench.NewDataset(name, version, cases)
	if err != nil {
		write(w, 400, map[string]string{"error": err.Error()})
		return
	}
	x := Dataset{ID: id, Name: name, Version: version, Cases: cases, Digest: versioned.Digest}
	if err := redactDataset(&x); err != nil {
		write(w, 500, map[string]string{"error": "redact dataset"})
		return
	}
	versioned, err = bench.NewDataset(x.Name, x.Version, x.Cases)
	if err != nil {
		write(w, 400, map[string]string{"error": err.Error()})
		return
	}
	x.Digest = versioned.Digest
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.datasets) >= maxDefinitions {
		write(w, 409, map[string]string{"error": "dataset limit reached"})
		return
	}
	if _, ok := s.datasets[id]; ok {
		write(w, 409, map[string]string{"error": "dataset already exists"})
		return
	}
	if err := s.Store.SaveDefinition("dataset", id, x); err != nil {
		write(w, 500, map[string]string{"error": "persist dataset"})
		return
	}
	s.datasets[id] = x
	write(w, 201, x)
}

func redactDataset(dataset *Dataset) error {
	raw, err := json.Marshal(dataset)
	if err != nil {
		return err
	}
	var object map[string]any
	if err = json.Unmarshal(raw, &object); err != nil {
		return err
	}
	clean, err := json.Marshal(core.Redact(object))
	if err != nil {
		return err
	}
	return json.Unmarshal(clean, dataset)
}
func (s *Server) createSuite(w http.ResponseWriter, r *http.Request) {
	var x Suite
	if decodeLimited(w, r, &x) != nil || !validDefinition(x.ID, x.Name) || x.Version == "" || x.DatasetID == "" || len(x.Scorers) == 0 {
		write(w, 400, map[string]string{"error": "valid id, name, and dataset_id required"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.datasets[x.DatasetID]; !ok {
		write(w, 400, map[string]string{"error": "dataset not found"})
		return
	}
	versioned, err := bench.NewSuite(x.Name, x.Version, s.datasets[x.DatasetID].Digest, x.Scorers)
	if err != nil {
		write(w, 400, map[string]string{"error": err.Error()})
		return
	}
	x.Digest = versioned.Digest
	if len(s.suites) >= maxDefinitions {
		write(w, 409, map[string]string{"error": "suite limit reached"})
		return
	}
	if _, ok := s.suites[x.ID]; ok {
		write(w, 409, map[string]string{"error": "suite already exists"})
		return
	}
	if err := s.Store.SaveDefinition("suite", x.ID, x); err != nil {
		write(w, 500, map[string]string{"error": "persist suite"})
		return
	}
	s.suites[x.ID] = x
	write(w, 201, x)
}
func (s *Server) createTarget(w http.ResponseWriter, r *http.Request) {
	var x Target
	if decodeLimited(w, r, &x) != nil || !validDefinition(x.ID, x.Name) || (x.Adapter != "fake" && x.Adapter != "openai-compatible") || len(x.Outputs) > maxCases || x.DelayMS < 0 || x.Failures < 0 {
		write(w, 400, map[string]string{"error": "valid target required"})
		return
	}
	if x.Adapter == "openai-compatible" && (x.Endpoint == "" || x.Model == "") {
		write(w, 400, map[string]string{"error": "OpenAI-compatible target requires endpoint and model"})
		return
	}
	for k, v := range x.Outputs {
		if len(k) > 128 || len(v) > 65536 {
			write(w, 400, map[string]string{"error": "target output exceeds limit"})
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.targets) >= maxDefinitions {
		write(w, 409, map[string]string{"error": "target limit reached"})
		return
	}
	if _, ok := s.targets[x.ID]; ok {
		write(w, 409, map[string]string{"error": "target already exists"})
		return
	}
	if err := s.Store.SaveDefinition("target", x.ID, x); err != nil {
		write(w, 500, map[string]string{"error": "persist target"})
		return
	}
	s.targets[x.ID] = x
	safe := x
	safe.Outputs = nil
	write(w, 201, safe)
}

func (s *Server) startBenchRun(w http.ResponseWriter, r *http.Request) {
	var x struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		SuiteID     string   `json:"suite_id"`
		TargetIDs   []string `json:"target_ids"`
		Concurrency int      `json:"concurrency,omitempty"`
		MaxAttempts int      `json:"max_attempts,omitempty"`
		TimeoutMS   int      `json:"timeout_ms,omitempty"`
		Seed        int64    `json:"seed,omitempty"`
	}
	if decodeLimited(w, r, &x) != nil || !validDefinition(x.ID, x.Name) || x.SuiteID == "" || len(x.TargetIDs) == 0 || len(x.TargetIDs) > maxTargetsPerRun {
		write(w, 400, map[string]string{"error": "valid id, suite_id, and 1..64 target_ids required"})
		return
	}
	s.mu.RLock()
	suite, ok := s.suites[x.SuiteID]
	dataset := s.datasets[suite.DatasetID]
	targets := make([]Target, 0, len(x.TargetIDs))
	for _, id := range x.TargetIDs {
		t, exists := s.targets[id]
		if !exists {
			ok = false
			break
		}
		targets = append(targets, t)
	}
	s.mu.RUnlock()
	if !ok {
		write(w, 400, map[string]string{"error": "suite or target not found"})
		return
	}
	if err := s.Store.CreateRun(core.Run{ID: x.ID, Name: x.Name, Mode: "bench"}); err != nil {
		write(w, 409, map[string]string{"error": err.Error()})
		return
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[x.ID] = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.cancels, x.ID)
		s.mu.Unlock()
	}()
	versionedDataset, err := bench.NewDataset(dataset.Name, dataset.Version, dataset.Cases)
	if err != nil {
		write(w, 500, map[string]string{"error": err.Error()})
		return
	}
	versionedSuite, err := bench.NewSuite(suite.Name, suite.Version, versionedDataset.Digest, suite.Scorers)
	if err != nil {
		write(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if x.Concurrency < 1 {
		x.Concurrency = 4
	}
	if x.MaxAttempts < 1 {
		x.MaxAttempts = 2
	}
	if x.TimeoutMS < 1 {
		x.TimeoutMS = 30000
	}
	if x.Concurrency > 64 || x.MaxAttempts > 10 || x.TimeoutMS > 600000 {
		write(w, 400, map[string]string{"error": "limits: concurrency<=64, max_attempts<=10, timeout_ms<=600000"})
		return
	}
	_, _ = s.Store.Append(x.ID, "benchmark.started", map[string]any{"suite": suite.ID, "suite_digest": versionedSuite.Digest, "dataset": dataset.ID, "dataset_digest": versionedDataset.Digest, "targets": x.TargetIDs, "cases": len(dataset.Cases), "concurrency": x.Concurrency, "max_attempts": x.MaxAttempts, "timeout_ms": x.TimeoutMS, "seed": x.Seed})
	expectedByCase := make(map[string]string, len(versionedDataset.Cases))
	for _, datasetCase := range versionedDataset.Cases {
		expectedByCase[datasetCase.ID] = datasetCase.Expected
	}
	failed := 0
	for _, target := range targets {
		var adapter bench.Target
		switch target.Adapter {
		case "fake":
			adapter = &adapters.Fake{Name: target.Name, Responses: target.Outputs, Delay: time.Duration(target.DelayMS) * time.Millisecond, Failures: target.Failures}
		case "openai-compatible":
			adapter = &adapters.OpenAICompatible{Endpoint: target.Endpoint, Model: target.Model, APIKey: os.Getenv(target.APIKeyEnv)}
		default:
			write(w, 400, map[string]string{"error": "unsupported target adapter"})
			return
		}
		result, runErr := bench.Run(runCtx, versionedDataset, versionedSuite, adapter, bench.RunOptions{RunID: x.ID + "/" + target.ID, Concurrency: x.Concurrency, MaxAttempts: x.MaxAttempts, Timeout: time.Duration(x.TimeoutMS) * time.Millisecond, Seed: x.Seed})
		if runErr != nil {
			if errors.Is(runErr, context.Canceled) {
				break
			}
			failed++
			_, _ = s.Store.Append(x.ID, "target.failed", map[string]any{"target": target.ID, "error": runErr.Error()})
			continue
		}
		if target.APIKeyEnv != "" {
			result.Manifest.EnvironmentNames = []string{target.APIKeyEnv}
			result.Manifest.Seal()
		}
		_, _ = s.Store.Append(x.ID, "target.manifest", map[string]any{"target": target.ID, "manifest": result.Manifest})
		for _, caseResult := range result.Cases {
			for _, attempt := range caseResult.History {
				payload := map[string]any{"case": caseResult.CaseID, "target": target.ID, "attempt": attempt.Attempt, "status": attempt.Status, "duration_ns": attempt.Duration.Nanoseconds(), "error": attempt.Error}
				_, _ = s.Store.Append(x.ID, "case.attempt", payload)
			}
			for _, score := range caseResult.Scores {
				_, _ = s.Store.Append(x.ID, "score.recorded", map[string]any{"case": caseResult.CaseID, "target": target.ID, "scorer": score.Kind, "scorer_version": score.Kind + "/v1", "score": score.Value, "passed": score.Passed, "evidence": score.Evidence, "expected": expectedByCase[caseResult.CaseID], "actual": caseResult.Output})
			}
			_, _ = s.Store.Append(x.ID, "case."+caseResult.Status, map[string]any{"case": caseResult.CaseID, "target": target.ID, "attempts": caseResult.Attempts, "duration_ns": caseResult.Duration.Nanoseconds(), "output": caseResult.Output, "error": caseResult.Error})
			if caseResult.Status != "completed" {
				failed++
			}
		}
	}
	terminal := "benchmark.completed"
	if errors.Is(runCtx.Err(), context.Canceled) {
		terminal = "benchmark.cancelled"
	} else if failed > 0 {
		terminal = "benchmark.completed_with_errors"
	}
	_, _ = s.Store.Append(x.ID, terminal, map[string]any{"suite": suite.ID, "failed": failed})
	p, _ := s.Store.Replay(x.ID, 0)
	write(w, 201, p)
}
func (s *Server) cancelBenchRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.owns(id) {
		write(w, 404, map[string]string{"error": "run not found"})
		return
	}
	p, _ := s.Store.Replay(id, 0)
	if p.Status == "completed" || p.Status == "failed" || p.Status == "cancelled" || p.Status == "interrupted" {
		write(w, 409, map[string]string{"error": "run is terminal"})
		return
	}
	s.mu.RLock()
	cancel := s.cancels[id]
	s.mu.RUnlock()
	if cancel == nil {
		write(w, 409, map[string]string{"error": "run is not active"})
		return
	}
	e, err := s.Store.Append(id, "benchmark.cancel_requested", map[string]any{"acknowledged": false})
	if err != nil {
		write(w, 500, map[string]string{"error": err.Error()})
		return
	}
	cancel()
	write(w, 202, e)
}
func scores(s *core.Store, id string) []bench.Result {
	var out []bench.Result
	for _, e := range s.Events(id, 0) {
		if e.Type != "score.recorded" {
			continue
		}
		score, _ := e.Data["score"].(float64)
		caseID, _ := e.Data["case"].(string)
		scorer, _ := e.Data["scorer_version"].(string)
		out = append(out, bench.Result{CaseID: caseID + "/" + scorer, Score: score})
	}
	return out
}
func (s *Server) compareRuns(w http.ResponseWriter, r *http.Request) {
	a, b := r.URL.Query().Get("baseline"), r.URL.Query().Get("candidate")
	if !s.owns(a) || !s.owns(b) {
		write(w, 404, map[string]string{"error": "run not found"})
		return
	}
	comparisonMeta := func(id string) (string, int) {
		for _, e := range s.Store.Events(id, 0) {
			if e.Type != "benchmark.started" {
				continue
			}
			digest, _ := e.Data["suite_digest"].(string)
			targets, _ := e.Data["targets"].([]any)
			if targets == nil {
				if typed, ok := e.Data["targets"].([]string); ok {
					return digest, len(typed)
				}
			}
			return digest, len(targets)
		}
		return "", 0
	}
	ad, at := comparisonMeta(a)
	bd, bt := comparisonMeta(b)
	if ad == "" || ad != bd || at != 1 || bt != 1 {
		write(w, 409, map[string]string{"error": "comparison requires two single-target runs with the same suite digest"})
		return
	}
	startConfig := func(id string) map[string]string {
		out := map[string]string{}
		for _, e := range s.Store.Events(id, 0) {
			if e.Type != "benchmark.started" {
				continue
			}
			for _, key := range []string{"dataset_digest", "suite_digest", "targets", "concurrency", "max_attempts", "timeout_ms", "seed"} {
				raw, _ := json.Marshal(e.Data[key])
				out[key] = string(raw)
			}
			break
		}
		return out
	}
	ac, bc := startConfig(a), startConfig(b)
	configDelta := map[string]map[string]string{}
	for key, av := range ac {
		if bv := bc[key]; av != bv {
			configDelta[key] = map[string]string{"baseline": av, "candidate": bv}
		}
	}
	c := bench.Compare(a, b, scores(s.Store, a), scores(s.Store, b))
	write(w, 200, map[string]any{"baseline": c.Baseline, "candidate": c.Candidate, "baseline_score": c.BaselineScore, "candidate_score": c.CandidateScore, "delta": c.Delta, "samples": c.Samples, "missing_baseline": c.MissingBaseline, "missing_candidate": c.MissingCandidate, "missing_treatment": "zero", "compatible": true, "configuration_delta": configDelta})
}

func (s *Server) exportRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.owns(id) {
		write(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	raw, err := bundle.Export(s.Store, id)
	if err != nil {
		write(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="n0ding-replay.json"`)
	w.Write(raw)
}
func (s *Server) importReplay(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, bundle.MaxBundleBytes)
	defer r.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		write(w, http.StatusBadRequest, map[string]string{"error": "invalid or oversized replay bundle"})
		return
	}
	p, err := bundle.VerifyAndReplay(raw)
	if err != nil {
		write(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	write(w, http.StatusOK, map[string]any{"mode": "replay", "projection": p})
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if !s.allowMutation(r.RemoteAddr) {
				w.Header().Set("Retry-After", "60")
				write(w, http.StatusTooManyRequests, map[string]string{"error": "mutation rate limit exceeded"})
				return
			}
			if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
				write(w, http.StatusForbidden, map[string]string{"error": "cross-site request rejected"})
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					write(w, http.StatusForbidden, map[string]string{"error": "origin rejected"})
					return
				}
			}
			isDatasetImport := r.URL.Path == "/api/v1/datasets/import"
			if ct := r.Header.Get("Content-Type"); r.ContentLength != 0 && ct != "" && !isDatasetImport && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
				write(w, http.StatusUnsupportedMediaType, map[string]string{"error": "application/json required"})
				return
			}
		}
		if s.Token != "" && strings.HasPrefix(r.URL.Path, "/api/") {
			authorization := r.Header.Get("Authorization")
			if !strings.HasPrefix(authorization, "Bearer ") || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(authorization, "Bearer ")), []byte(s.Token)) != 1 {
				write(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowMutation(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	now := time.Now()
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	w := s.rates[host]
	if w.Started.IsZero() || now.Sub(w.Started) >= time.Minute {
		w = rateWindow{Started: now}
	}
	w.Count++
	s.rates[host] = w
	return w.Count <= 120
}

func (s *Server) owns(id string) bool { run, ok := s.Store.GetRun(id); return ok && run.Mode == s.Mode }

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, map[string]any{"ok": true, "mode": s.Mode})
}

func (s *Server) listRuns(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, map[string]any{"runs": s.Store.ListRuns(s.Mode)})
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var x struct{ ID, Name string }
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.ID == "" {
		write(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	run := core.Run{ID: x.ID, Name: x.Name, Mode: s.Mode}
	if err := s.Store.CreateRun(run); err != nil {
		write(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	run, _ = s.Store.GetRun(x.ID)
	write(w, http.StatusCreated, run)
}

func (s *Server) appendEvent(w http.ResponseWriter, r *http.Request) {
	if !s.owns(r.PathValue("id")) {
		write(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var x struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Type == "" {
		write(w, http.StatusBadRequest, map[string]string{"error": "type required"})
		return
	}
	e, err := s.Store.Append(r.PathValue("id"), x.Type, x.Data)
	if err != nil {
		write(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	write(w, http.StatusCreated, e)
}

func afterID(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("after")
	if raw == "" {
		raw = r.Header.Get("Last-Event-ID")
	}
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid event cursor")
	}
	return n, nil
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.owns(id) {
		write(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	cursor, err := afterID(r)
	if err != nil {
		write(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		events, more := s.Store.EventsLimit(id, cursor, maxSSEBacklog)
		if more {
			write(w, http.StatusConflict, map[string]string{"error": "event backlog exceeds bounded response; advance cursor"})
			return
		}
		write(w, http.StatusOK, map[string]any{"events": events})
		return
	}
	if _, more := s.Store.EventsLimit(id, cursor, maxSSEBacklog); more {
		write(w, http.StatusConflict, map[string]string{"error": "event backlog exceeds stream limit; resync from snapshot"})
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		write(w, http.StatusInternalServerError, map[string]string{"error": "stream unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func() bool {
		events, more := s.Store.EventsLimit(id, cursor, maxSSEBacklog)
		if more {
			return false
		}
		for _, e := range events {
			b, _ := json.Marshal(e)
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Type, b); err != nil {
				return false
			}
			cursor = e.ID
		}
		f.Flush()
		return true
	}
	for {
		ch, _ := s.Store.Subscribe()
		if !send() {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			continue
		case <-time.After(15 * time.Second):
			fmt.Fprint(w, ": keepalive\n\n")
			f.Flush()
		}
	}
}

func (s *Server) projection(w http.ResponseWriter, r *http.Request) {
	if !s.owns(r.PathValue("id")) {
		write(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	upto, _ := strconv.ParseInt(r.URL.Query().Get("upto"), 10, 64)
	p, err := s.Store.Replay(r.PathValue("id"), upto)
	if err != nil {
		write(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	write(w, http.StatusOK, p)
}

func (s *Server) fixture(w http.ResponseWriter, _ *http.Request) {
	run, err := core.LoadFixture(s.Store, s.Mode)
	if err != nil {
		write(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	write(w, http.StatusCreated, run)
}
