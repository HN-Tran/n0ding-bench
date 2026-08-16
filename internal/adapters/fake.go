package adapters

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/hn-tran/n0ding-bench/internal/bench"
)

type Fake struct {
	Name      string
	Responses map[string]string
	Delay     time.Duration
	Failures  int
	mu        sync.Mutex
	calls     map[string]int
}

func (f *Fake) Identity() bench.TargetIdentity {
	return bench.TargetIdentity{Kind: "fake", Name: f.Name, Model: "deterministic"}
}
func (f *Fake) Invoke(ctx context.Context, r bench.TargetRequest) (bench.TargetResponse, error) {
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[r.CaseID]++
	call := f.calls[r.CaseID]
	f.mu.Unlock()
	select {
	case <-time.After(f.Delay):
	case <-ctx.Done():
		return bench.TargetResponse{}, ctx.Err()
	}
	if call <= f.Failures {
		return bench.TargetResponse{}, TemporaryError{errors.New("configured temporary failure")}
	}
	out, ok := f.Responses[r.CaseID]
	if !ok {
		out = r.Input
	}
	return bench.TargetResponse{Output: out}, nil
}

type TemporaryError struct{ Err error }

func (e TemporaryError) Error() string   { return e.Err.Error() }
func (e TemporaryError) Unwrap() error   { return e.Err }
func (e TemporaryError) Retryable() bool { return true }
