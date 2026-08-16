package bench

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
)

type TargetIdentity struct{ Kind, Name, Model, Endpoint string }
type TargetRequest struct {
	CaseID, Input string
	Seed          int64
}
type TargetResponse struct {
	Output   string
	Metadata map[string]string
}
type Target interface {
	Identity() TargetIdentity
	Invoke(context.Context, TargetRequest) (TargetResponse, error)
}
type Retryable interface{ Retryable() bool }

type RunOptions struct {
	RunID                    string
	Concurrency, MaxAttempts int
	Timeout                  time.Duration
	Seed                     int64
}
type CaseResult struct {
	CaseID, Output, Status string
	Attempts               int
	History                []AttemptRecord
	Duration               time.Duration
	Error                  string
	Scores                 []Score
}
type AttemptRecord struct {
	Attempt  int           `json:"attempt"`
	Status   string        `json:"status"`
	Output   string        `json:"output,omitempty"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
}
type RunResult struct {
	Manifest              ReproducibilityManifest
	Cases                 []CaseResult
	StartedAt, FinishedAt time.Time
}

func Run(ctx context.Context, ds Dataset, suite Suite, target Target, opt RunOptions) (RunResult, error) {
	if err := ds.Verify(); err != nil {
		return RunResult{}, err
	}
	if err := suite.Verify(); err != nil {
		return RunResult{}, err
	}
	if suite.DatasetDigest != ds.Digest {
		return RunResult{}, errors.New("suite references a different dataset")
	}
	if opt.Concurrency < 1 {
		opt.Concurrency = 1
	}
	if opt.MaxAttempts < 1 {
		opt.MaxAttempts = 1
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 30 * time.Second
	}
	result := RunResult{StartedAt: time.Now().UTC(), Cases: make([]CaseResult, len(ds.Cases))}
	identity := target.Identity()
	adapterVersion, scorerVersions := NewManifestVersions(identity, suite.Scorers)
	manifest := ReproducibilityManifest{RunID: opt.RunID, DatasetDigest: ds.Digest, SuiteDigest: suite.Digest, Target: identity, Seed: opt.Seed, StartedAt: result.StartedAt, Config: map[string]string{"concurrency": fmt.Sprint(opt.Concurrency), "max_attempts": fmt.Sprint(opt.MaxAttempts), "timeout": opt.Timeout.String(), "cache_state": "unspecified", "retry_policy": "classified-only", "warmup_policy": "none", "missing_sample_treatment": "zero-in-comparison", "timing_source": "monotonic-process-clock"}, BuildVersion: BuildVersion, BuildCommit: BuildCommit, AdapterVersion: adapterVersion, ScorerVersions: scorerVersions, Runtime: runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH, LogicalCPUs: runtime.NumCPU()}
	for _, c := range ds.Cases {
		manifest.CaseOrder = append(manifest.CaseOrder, c.ID)
	}
	manifest.Seal()
	result.Manifest = manifest
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < opt.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				result.Cases[i] = runCase(ctx, ds.Cases[i], suite.Scorers, target, opt)
			}
		}()
	}
	for i := range ds.Cases {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return result, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	result.FinishedAt = time.Now().UTC()
	result.Manifest.FinishedAt = result.FinishedAt
	result.Manifest.Seal()
	return result, nil
}

var BuildVersion = "0.1.0-dev"
var BuildCommit = "unknown"

func runCase(parent context.Context, c DatasetCase, specs []ScorerSpec, target Target, opt RunOptions) CaseResult {
	r := CaseResult{CaseID: c.ID}
	start := time.Now()
	for attempt := 1; attempt <= opt.MaxAttempts; attempt++ {
		r.Attempts = attempt
		attemptStarted := time.Now()
		ctx, cancel := context.WithTimeout(parent, opt.Timeout)
		response, err := target.Invoke(ctx, TargetRequest{CaseID: c.ID, Input: c.Input, Seed: opt.Seed})
		cause := context.Cause(ctx)
		cancel()
		if err == nil {
			r.Output = response.Output
			r.Status = "completed"
			r.History = append(r.History, AttemptRecord{Attempt: attempt, Status: r.Status, Output: r.Output, Duration: time.Since(attemptStarted)})
			break
		}
		r.Error = err.Error()
		r.Status = "failed"
		if errors.Is(cause, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			r.Status = "timed_out"
		}
		if errors.Is(err, context.Canceled) || errors.Is(parent.Err(), context.Canceled) {
			r.Status = "cancelled"
			r.History = append(r.History, AttemptRecord{Attempt: attempt, Status: r.Status, Error: r.Error, Duration: time.Since(attemptStarted)})
			break
		}
		r.History = append(r.History, AttemptRecord{Attempt: attempt, Status: r.Status, Error: r.Error, Duration: time.Since(attemptStarted)})
		var retry Retryable
		if attempt == opt.MaxAttempts || !errors.As(err, &retry) || !retry.Retryable() {
			break
		}
	}
	r.Duration = time.Since(start)
	failed := r.Status != "completed"
	for _, spec := range specs {
		r.Scores = append(r.Scores, ScoreObservation(spec, Observation{Expected: c.Expected, Actual: r.Output, Duration: r.Duration, Failed: failed}))
	}
	return r
}

type RunComparison struct {
	Baseline, Candidate                string
	Cases                              int
	BaselineMean, CandidateMean, Delta float64
	Regressions                        []string
}

func CompareRuns(a, b RunResult) RunComparison {
	c := RunComparison{Baseline: a.Manifest.RunID, Candidate: b.Manifest.RunID}
	bm := map[string]CaseResult{}
	for _, r := range a.Cases {
		bm[r.CaseID] = r
	}
	for _, x := range b.Cases {
		y, ok := bm[x.CaseID]
		if !ok {
			continue
		}
		av, bv := meanScores(y.Scores), meanScores(x.Scores)
		c.Cases++
		c.BaselineMean += av
		c.CandidateMean += bv
		if bv < av {
			c.Regressions = append(c.Regressions, x.CaseID)
		}
	}
	if c.Cases > 0 {
		c.BaselineMean /= float64(c.Cases)
		c.CandidateMean /= float64(c.Cases)
	}
	c.Delta = c.CandidateMean - c.BaselineMean
	return c
}
func meanScores(s []Score) float64 {
	if len(s) == 0 {
		return 0
	}
	var n float64
	for _, x := range s {
		n += x.Value
	}
	return n / float64(len(s))
}
