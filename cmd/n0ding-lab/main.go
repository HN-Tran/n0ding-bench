package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/hn-tran/n0ding-lab/internal/core"
	"github.com/hn-tran/n0ding-lab/internal/httpapi"
)

func main() {
	mode := flag.String("mode", "bench", "bench or dispatch")
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	fixture := flag.Bool("fixture", false, "load deterministic fixture")
	dbPath := flag.String("db", "", "SQLite database path (default: <mode>.db)")
	authToken := flag.String("auth-token", "", "required bearer token for non-loopback bind")
	flag.Parse()
	if *mode != "bench" && *mode != "dispatch" {
		fmt.Fprintln(os.Stderr, "mode must be bench or dispatch")
		os.Exit(2)
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatalf("invalid listen address: %v", err)
	}
	ip := net.ParseIP(host)
	loopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !loopback && *authToken == "" {
		log.Fatal("non-loopback bind requires -auth-token")
	}
	if *dbPath == "" {
		*dbPath = *mode + ".db"
	}
	store, err := core.OpenStore(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if *fixture {
		if _, err := core.LoadFixture(store, *mode); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("n0ding %s listening on http://%s", *mode, *addr)
	server := &http.Server{Addr: *addr, Handler: httpapi.NewAuthenticated(*mode, store, *authToken), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Fatal(server.ListenAndServe())
}
