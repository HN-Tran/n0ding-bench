package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/hn-tran/n0ding-bench/internal/core"
	"github.com/hn-tran/n0ding-bench/internal/httpapi"
)

const (
	exitOK          = 0
	exitUsage       = 2
	exitUnavailable = 3
	exitRejected    = 4
	exitInternal    = 5
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "init":
		return initCommand(args[1:], stdout, stderr)
	case "serve":
		return serveCommand(args[1:], stdout, stderr)
	case "run":
		return requestCommand("POST", "/api/v1/bench/runs", args[1:], stdout, stderr)
	case "runs":
		return requestCommand("GET", "/api/v1/runs", args[1:], stdout, stderr)
	case "export":
		return exportCommand(args[1:], stdout, stderr)
	case "doctor":
		return doctorCommand(args[1:], stdout, stderr)
	case "ci":
		return ciCommand(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return exitUsage
	}
}
func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: n0ding-bench <init|serve|run|runs|export|doctor|ci> [options]")
}

type junitSuite struct {
	XMLName  xml.Name  `xml:"testsuite"`
	Name     string    `xml:"name,attr"`
	Tests    int       `xml:"tests,attr"`
	Failures int       `xml:"failures,attr"`
	Case     junitCase `xml:"testcase"`
}
type junitCase struct {
	Name    string        `xml:"name,attr"`
	Failure *junitFailure `xml:"failure,omitempty"`
}
type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func ciCommand(args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet("ci", flag.ContinueOnError)
	fs.SetOutput(errout)
	url := fs.String("url", "http://127.0.0.1:8080", "server URL")
	token := fs.String("auth-token", "", "bearer token")
	baseline := fs.String("baseline", "", "baseline run")
	candidate := fs.String("candidate", "", "candidate run")
	minDelta := fs.Float64("min-delta", 0, "minimum acceptable candidate-baseline delta")
	junit := fs.String("junit", "", "JUnit output path")
	if fs.Parse(args) != nil || *baseline == "" || *candidate == "" {
		jsonOut(errout, map[string]string{"error": "--baseline and --candidate required"})
		return exitUsage
	}
	path := fmt.Sprintf("/api/v1/comparisons?baseline=%s&candidate=%s", neturl.QueryEscape(*baseline), neturl.QueryEscape(*candidate))
	raw, status, e := do("GET", *url+path, *token, nil)
	if e != nil {
		return exitUnavailable
	}
	if status != 200 {
		errout.Write(raw)
		return exitRejected
	}
	var comparison struct {
		Delta   float64 `json:"delta"`
		Samples int     `json:"samples"`
	}
	if json.Unmarshal(raw, &comparison) != nil {
		return exitInternal
	}
	passed := comparison.Delta >= *minDelta
	suite := junitSuite{Name: "n0ding-bench-regression", Tests: 1, Case: junitCase{Name: "candidate-vs-baseline"}}
	if !passed {
		suite.Failures = 1
		suite.Case.Failure = &junitFailure{Message: "regression threshold crossed", Text: fmt.Sprintf("delta=%g minimum=%g samples=%d", comparison.Delta, *minDelta, comparison.Samples)}
	}
	if *junit != "" {
		b, _ := xml.MarshalIndent(suite, "", "  ")
		b = append([]byte(xml.Header), b...)
		if os.WriteFile(*junit, b, 0600) != nil {
			return exitInternal
		}
	}
	jsonOut(out, map[string]any{"passed": passed, "delta": comparison.Delta, "minimum": *minDelta, "samples": comparison.Samples})
	if !passed {
		return exitRejected
	}
	return exitOK
}
func jsonOut(w io.Writer, v any) { _ = json.NewEncoder(w).Encode(v) }

func initCommand(args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(errout)
	db := fs.String("db", "bench.db", "SQLite database")
	if fs.Parse(args) != nil {
		return exitUsage
	}
	if e := os.MkdirAll(filepath.Dir(*db), 0700); e != nil && filepath.Dir(*db) != "." {
		jsonOut(errout, map[string]any{"error": e.Error()})
		return exitInternal
	}
	s, e := core.OpenStore(*db)
	if e != nil {
		jsonOut(errout, map[string]any{"error": e.Error()})
		return exitInternal
	}
	s.Close()
	jsonOut(out, map[string]any{"ok": true, "database": *db})
	return exitOK
}

func serveCommand(args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(errout)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	db := fs.String("db", "bench.db", "SQLite database")
	token := fs.String("auth-token", "", "bearer token required for remote binds")
	if fs.Parse(args) != nil {
		return exitUsage
	}
	host, _, e := net.SplitHostPort(*addr)
	if e != nil {
		jsonOut(errout, map[string]any{"error": "invalid listen address"})
		return exitUsage
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) && *token == "" {
		jsonOut(errout, map[string]any{"error": "non-loopback bind requires --auth-token"})
		return exitUsage
	}
	s, e := core.OpenStore(*db)
	if e != nil {
		jsonOut(errout, map[string]any{"error": e.Error()})
		return exitInternal
	}
	defer s.Close()
	jsonOut(out, map[string]any{"ok": true, "product": "n0ding-bench", "address": *addr})
	srv := http.Server{Addr: *addr, Handler: httpapi.NewAuthenticated("bench", s, *token), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	e = srv.ListenAndServe()
	if errors.Is(e, http.ErrServerClosed) {
		return exitOK
	}
	jsonOut(errout, map[string]any{"error": e.Error()})
	return exitUnavailable
}

type clientFlags struct{ URL, Token, File string }

func parseClient(name string, args []string, needFile bool, errout io.Writer) (clientFlags, bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errout)
	var c clientFlags
	fs.StringVar(&c.URL, "url", "http://127.0.0.1:8080", "server URL")
	fs.StringVar(&c.Token, "auth-token", "", "bearer token")
	if needFile {
		fs.StringVar(&c.File, "file", "", "JSON input file")
	}
	if fs.Parse(args) != nil {
		return c, false
	}
	if needFile && c.File == "" {
		jsonOut(errout, map[string]string{"error": "--file required"})
		return c, false
	}
	return c, true
}
func do(method, url, token string, body io.Reader) ([]byte, int, error) {
	req, e := http.NewRequest(method, url, body)
	if e != nil {
		return nil, 0, e
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	cl := http.Client{Timeout: 30 * time.Second}
	resp, e := cl.Do(req)
	if e != nil {
		return nil, 0, e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 9<<20))
	return raw, resp.StatusCode, e
}
func requestCommand(method, path string, args []string, out, errout io.Writer) int {
	need := method == "POST"
	c, ok := parseClient("request", args, need, errout)
	if !ok {
		return exitUsage
	}
	var body io.Reader
	if need {
		raw, e := os.ReadFile(c.File)
		if e != nil {
			jsonOut(errout, map[string]string{"error": e.Error()})
			return exitUsage
		}
		body = bytes.NewReader(raw)
	}
	raw, status, e := do(method, c.URL+path, c.Token, body)
	if e != nil {
		jsonOut(errout, map[string]string{"error": e.Error()})
		return exitUnavailable
	}
	if status < 200 || status > 299 {
		errout.Write(raw)
		return exitRejected
	}
	out.Write(raw)
	return exitOK
}
func exportCommand(args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(errout)
	url := fs.String("url", "http://127.0.0.1:8080", "server URL")
	token := fs.String("auth-token", "", "bearer token")
	id := fs.String("run", "", "run id")
	file := fs.String("out", "", "output file (stdout by default)")
	if fs.Parse(args) != nil || *id == "" {
		jsonOut(errout, map[string]string{"error": "--run required"})
		return exitUsage
	}
	raw, status, e := do("GET", *url+"/api/v1/runs/"+*id+"/export", *token, nil)
	if e != nil {
		return exitUnavailable
	}
	if status != 200 {
		errout.Write(raw)
		return exitRejected
	}
	if *file != "" {
		if e = os.WriteFile(*file, raw, 0600); e != nil {
			jsonOut(errout, map[string]string{"error": e.Error()})
			return exitInternal
		}
		jsonOut(out, map[string]any{"ok": true, "file": *file})
		return exitOK
	}
	out.Write(raw)
	return exitOK
}
func doctorCommand(args []string, out, errout io.Writer) int {
	c, ok := parseClient("doctor", args, false, errout)
	if !ok {
		return exitUsage
	}
	raw, status, e := do("GET", c.URL+"/healthz", c.Token, nil)
	if e != nil {
		jsonOut(errout, map[string]any{"ok": false, "error": e.Error()})
		return exitUnavailable
	}
	if status != 200 {
		errout.Write(raw)
		return exitRejected
	}
	out.Write(raw)
	return exitOK
}
