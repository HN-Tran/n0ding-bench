package bench

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Case struct {
	ID, Input, Expected, Mode string
	Timeout                   time.Duration
}
type Result struct {
	CaseID, Status, Evidence, ScorerVersion string
	Score                                   float64
	Duration                                time.Duration
	Attempt                                 int
	RetryOf                                 int
}

func Evaluate(c Case, actual string, duration time.Duration, attempt, retryOf int) Result {
	r := Result{CaseID: c.ID, Status: "completed", Evidence: actual, ScorerVersion: "deterministic/v1", Duration: duration, Attempt: attempt, RetryOf: retryOf}
	if c.Timeout > 0 && duration > c.Timeout {
		r.Status = "timed_out"
		return r
	}
	switch c.Mode {
	case "exact":
		if actual == c.Expected {
			r.Score = 1
		}
	case "contains":
		if strings.Contains(actual, c.Expected) {
			r.Score = 1
		}
	case "regex":
		matched, err := regexp.MatchString(c.Expected, actual)
		if err != nil {
			r.Status = "failed"
			r.Evidence = fmt.Sprintf("invalid scorer regex: %v", err)
			return r
		}
		if matched {
			r.Score = 1
		}
	default:
		r.Status = "failed"
		r.Evidence = "unknown deterministic scorer"
	}
	return r
}

type Comparison struct {
	Baseline, Candidate                  string
	BaselineScore, CandidateScore, Delta float64
	Samples                              int
}

func Compare(baseline, candidate string, a, b []Result) Comparison {
	c := Comparison{Baseline: baseline, Candidate: candidate, Samples: min(len(a), len(b))}
	for i := 0; i < c.Samples; i++ {
		c.BaselineScore += a[i].Score
		c.CandidateScore += b[i].Score
	}
	if c.Samples > 0 {
		c.BaselineScore /= float64(c.Samples)
		c.CandidateScore /= float64(c.Samples)
	}
	c.Delta = c.CandidateScore - c.BaselineScore
	return c
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
