package dispatch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
)

type Action struct {
	Tool          string            `json:"tool"`
	Target        string            `json:"target"`
	Arguments     map[string]string `json:"arguments"`
	PolicyVersion string            `json:"policy_version"`
}

func ActionDigest(a Action) string {
	keys := make([]string, 0, len(a.Arguments))
	for k := range a.Arguments {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make([][2]string, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, [2]string{k, a.Arguments[k]})
	}
	b, _ := json.Marshal(struct {
		Tool, Target, Policy string
		Args                 [][2]string
	}{a.Tool, a.Target, a.PolicyVersion, ordered})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type Approval struct {
	Digest, Actor string
	Expires       time.Time
}

func (a Approval) ValidFor(action Action, now time.Time) bool {
	return a.Actor != "" && now.Before(a.Expires) && a.Digest == ActionDigest(action)
}

type Controller struct {
	mu      sync.Mutex
	results map[string]string
	fences  map[string]uint64
}

func NewController() *Controller {
	return &Controller{results: map[string]string{}, fences: map[string]uint64{}}
}
func (c *Controller) ExecuteOnce(key string, fn func() (string, error)) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if key == "" {
		return "", errors.New("idempotency key required")
	}
	if v, ok := c.results[key]; ok {
		return v, nil
	}
	v, err := fn()
	if err != nil {
		return "", err
	}
	c.results[key] = v
	return v, nil
}
func (c *Controller) RenewLease(task string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fences[task]++
	return c.fences[task]
}
func (c *Controller) Commit(task string, token uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if token == 0 || token != c.fences[task] {
		return errors.New("stale fencing token")
	}
	return nil
}

type Outcome string

const (
	OutcomeCompleted Outcome = "completed"
	OutcomeUnknown   Outcome = "outcome_unknown"
)

func ClassifyTransportResult(sideEffecting, responseObserved bool) Outcome {
	if sideEffecting && !responseObserved {
		return OutcomeUnknown
	}
	return OutcomeCompleted
}
