package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Dataset and Suite are immutable content-addressed inputs. Their versions are
// labels for humans; Digest is derived from canonical JSON and is authoritative.
type Dataset struct {
	Name    string        `json:"name"`
	Version string        `json:"version"`
	Cases   []DatasetCase `json:"cases"`
	Digest  string        `json:"digest"`
}

type DatasetCase struct {
	ID       string         `json:"id"`
	Input    string         `json:"input"`
	Expected string         `json:"expected,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Suite struct {
	Name          string       `json:"name"`
	Version       string       `json:"version"`
	DatasetDigest string       `json:"dataset_digest"`
	Scorers       []ScorerSpec `json:"scorers"`
	Digest        string       `json:"digest"`
}

type ScorerSpec struct {
	Kind      string  `json:"kind"`
	Pattern   string  `json:"pattern,omitempty"`
	Tolerance float64 `json:"tolerance,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
}

func NewDataset(name, version string, cases []DatasetCase) (Dataset, error) {
	if name == "" || version == "" || len(cases) == 0 {
		return Dataset{}, errors.New("dataset name, version and cases are required")
	}
	copyCases := make([]DatasetCase, len(cases))
	for i, c := range cases {
		copyCases[i] = c
		if c.Metadata != nil {
			// JSON round-tripping prevents callers from mutating nested metadata
			// after the content digest has been assigned.
			raw, err := json.Marshal(c.Metadata)
			if err != nil {
				return Dataset{}, fmt.Errorf("case %q metadata: %w", c.ID, err)
			}
			if err := json.Unmarshal(raw, &copyCases[i].Metadata); err != nil {
				return Dataset{}, err
			}
		}
	}
	seen := map[string]bool{}
	for _, c := range copyCases {
		if c.ID == "" || seen[c.ID] {
			return Dataset{}, fmt.Errorf("case IDs must be non-empty and unique: %q", c.ID)
		}
		seen[c.ID] = true
	}
	d := Dataset{Name: name, Version: version, Cases: copyCases}
	d.Digest = digest(struct {
		Name, Version string
		Cases         []DatasetCase
	}{name, version, copyCases})
	return d, nil
}

func NewSuite(name, version, datasetDigest string, scorers []ScorerSpec) (Suite, error) {
	if name == "" || version == "" || datasetDigest == "" || len(scorers) == 0 {
		return Suite{}, errors.New("suite name, version, dataset digest and scorers are required")
	}
	s := Suite{Name: name, Version: version, DatasetDigest: datasetDigest, Scorers: append([]ScorerSpec(nil), scorers...)}
	s.Digest = digest(struct {
		Name, Version, DatasetDigest string
		Scorers                      []ScorerSpec
	}{name, version, datasetDigest, s.Scorers})
	return s, nil
}

func (d Dataset) Verify() error {
	x, err := NewDataset(d.Name, d.Version, d.Cases)
	if err != nil {
		return err
	}
	if x.Digest != d.Digest {
		return errors.New("dataset digest mismatch")
	}
	return nil
}
func (s Suite) Verify() error {
	x, err := NewSuite(s.Name, s.Version, s.DatasetDigest, s.Scorers)
	if err != nil {
		return err
	}
	if x.Digest != s.Digest {
		return errors.New("suite digest mismatch")
	}
	return nil
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type ReproducibilityManifest struct {
	SchemaVersion    string            `json:"schema_version"`
	RunID            string            `json:"run_id"`
	DatasetDigest    string            `json:"dataset_digest"`
	SuiteDigest      string            `json:"suite_digest"`
	Target           TargetIdentity    `json:"target"`
	Seed             int64             `json:"seed"`
	StartedAt        time.Time         `json:"started_at"`
	FinishedAt       time.Time         `json:"finished_at"`
	Config           map[string]string `json:"config"`
	CaseOrder        []string          `json:"case_order"`
	Digest           string            `json:"digest"`
	BuildVersion     string            `json:"build_version"`
	BuildCommit      string            `json:"build_commit"`
	AdapterVersion   string            `json:"adapter_version"`
	ScorerVersions   []string          `json:"scorer_versions"`
	Runtime          string            `json:"runtime"`
	LogicalCPUs      int               `json:"logical_cpus"`
	EnvironmentNames []string          `json:"environment_names,omitempty"`
}

func NewManifestVersions(target TargetIdentity, scorers []ScorerSpec) (string, []string) {
	adapter := target.Kind + "/v1"
	versions := make([]string, 0, len(scorers))
	for _, s := range scorers {
		versions = append(versions, s.Kind+"/v1")
	}
	sort.Strings(versions)
	return adapter, versions
}

func (m *ReproducibilityManifest) Seal() {
	m.SchemaVersion = "1.0"
	sort.Strings(m.CaseOrder)
	clone := *m
	clone.Digest = ""
	m.Digest = digest(clone)
}
func (m ReproducibilityManifest) Verify() bool {
	got := m.Digest
	m.Digest = ""
	return got != "" && got == digest(m)
}
