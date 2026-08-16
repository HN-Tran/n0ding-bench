package bench

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const maxImportCases = 10000
const maxImportLine = 1 << 20

// ImportJSONL reads one DatasetCase JSON object per line.
func ImportJSONL(r io.Reader) ([]DatasetCase, error) {
	s := bufio.NewScanner(io.LimitReader(r, int64(maxImportCases*maxImportLine)))
	s.Buffer(make([]byte, 64*1024), maxImportLine)
	var cases []DatasetCase
	for s.Scan() {
		if len(cases) == maxImportCases {
			return nil, errors.New("dataset exceeds 10000 cases")
		}
		var c DatasetCase
		if err := json.Unmarshal(s.Bytes(), &c); err != nil {
			return nil, fmt.Errorf("JSONL line %d: %w", len(cases)+1, err)
		}
		cases = append(cases, c)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, errors.New("dataset is empty")
	}
	if _, err := NewDataset("import", "1", cases); err != nil {
		return nil, err
	}
	return cases, nil
}

// ImportCSV expects a header containing id,input and optional expected.
func ImportCSV(r io.Reader) ([]DatasetCase, error) {
	cr := csv.NewReader(io.LimitReader(r, int64(maxImportCases*maxImportLine)))
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV header: %w", err)
	}
	index := map[string]int{}
	for i, name := range header {
		index[name] = i
	}
	if _, ok := index["id"]; !ok {
		return nil, errors.New("CSV requires id header")
	}
	if _, ok := index["input"]; !ok {
		return nil, errors.New("CSV requires input header")
	}
	value := func(row []string, name string) string {
		i, ok := index[name]
		if !ok || i >= len(row) {
			return ""
		}
		return row[i]
	}
	var cases []DatasetCase
	for {
		row, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV row %d: %w", len(cases)+2, err)
		}
		if len(cases) == maxImportCases {
			return nil, errors.New("dataset exceeds 10000 cases")
		}
		cases = append(cases, DatasetCase{ID: value(row, "id"), Input: value(row, "input"), Expected: value(row, "expected")})
	}
	if len(cases) == 0 {
		return nil, errors.New("dataset is empty")
	}
	if _, err := NewDataset("import", "1", cases); err != nil {
		return nil, err
	}
	return cases, nil
}
