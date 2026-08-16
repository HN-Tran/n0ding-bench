package bench

import (
	"strings"
	"testing"
)

func TestImportJSONLAndCSV(t *testing.T) {
	jsonl := "{\"id\":\"a\",\"input\":\"q\",\"expected\":\"x\"}\n{\"id\":\"b\",\"input\":\"r\"}\n"
	cases, err := ImportJSONL(strings.NewReader(jsonl))
	if err != nil || len(cases) != 2 || cases[0].Expected != "x" {
		t.Fatalf("jsonl=%+v err=%v", cases, err)
	}
	cases, err = ImportCSV(strings.NewReader("id,input,expected\na,q,x\nb,r,\n"))
	if err != nil || len(cases) != 2 || cases[1].ID != "b" {
		t.Fatalf("csv=%+v err=%v", cases, err)
	}
}

func TestImportsRejectDuplicateIDs(t *testing.T) {
	_, err := ImportCSV(strings.NewReader("id,input\na,q\na,r\n"))
	if err == nil {
		t.Fatal("duplicate IDs accepted")
	}
}
