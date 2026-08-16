package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAndStableUsageExit(t *testing.T) {
	var out, errout bytes.Buffer
	if got := run([]string{"init", "--db", filepath.Join(t.TempDir(), "bench.db")}, &out, &errout); got != exitOK {
		t.Fatalf("init=%d %s", got, errout.String())
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Fatalf("non-json output: %s", out.String())
	}
	out.Reset()
	errout.Reset()
	if got := run([]string{"unknown"}, &out, &errout); got != exitUsage {
		t.Fatalf("usage exit=%d", got)
	}
}

func TestCIThresholdGreenAndRed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"delta":-0.1,"samples":10}`))
	}))
	defer srv.Close()
	var out, errout bytes.Buffer
	junit := filepath.Join(t.TempDir(), "report.xml")
	if got := run([]string{"ci", "--url", srv.URL, "--baseline", "a", "--candidate", "b", "--min-delta", "-0.2"}, &out, &errout); got != exitOK {
		t.Fatalf("green=%d %s", got, errout.String())
	}
	out.Reset()
	errout.Reset()
	if got := run([]string{"ci", "--url", srv.URL, "--baseline", "a", "--candidate", "b", "--min-delta", "0", "--junit", junit}, &out, &errout); got != exitRejected {
		t.Fatalf("red=%d %s", got, errout.String())
	}
	raw, err := os.ReadFile(junit)
	if err != nil || !strings.Contains(string(raw), `failures="1"`) {
		t.Fatalf("bad junit %v %s", err, raw)
	}
}
