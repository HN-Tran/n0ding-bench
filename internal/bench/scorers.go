package bench

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Observation struct {
	Expected, Actual string
	Duration         time.Duration
	Failed           bool
}
type Score struct {
	Kind     string  `json:"kind"`
	Value    float64 `json:"value"`
	Passed   bool    `json:"passed"`
	Evidence string  `json:"evidence,omitempty"`
}

func ScoreObservation(spec ScorerSpec, o Observation) Score {
	s := Score{Kind: spec.Kind}
	switch spec.Kind {
	case "exact":
		s.Passed = o.Actual == o.Expected
	case "contains":
		s.Passed = strings.Contains(o.Actual, o.Expected)
	case "regex":
		pattern := spec.Pattern
		if pattern == "" {
			pattern = o.Expected
		}
		r, err := regexp.Compile(pattern)
		if err != nil {
			s.Evidence = "invalid regex: " + err.Error()
			return s
		}
		s.Passed = r.MatchString(o.Actual)
	case "numeric_tolerance":
		a, ae := strconv.ParseFloat(strings.TrimSpace(o.Actual), 64)
		e, ee := strconv.ParseFloat(strings.TrimSpace(o.Expected), 64)
		if ae != nil || ee != nil {
			s.Evidence = "expected and actual must be numeric"
			return s
		}
		delta := a - e
		if delta < 0 {
			delta = -delta
		}
		s.Passed = delta <= spec.Tolerance
		s.Evidence = fmt.Sprintf("delta=%g tolerance=%g", delta, spec.Tolerance)
	case "latency":
		s.Passed = o.Duration <= time.Duration(spec.Threshold*float64(time.Millisecond))
		s.Evidence = o.Duration.String()
	case "error_rate":
		if o.Failed {
			s.Value = 1
		}
		s.Passed = s.Value <= spec.Threshold
		s.Evidence = fmt.Sprintf("error=%t threshold=%g", o.Failed, spec.Threshold)
		return s
	default:
		s.Evidence = "unknown scorer"
		return s
	}
	if s.Passed {
		s.Value = 1
	}
	return s
}
