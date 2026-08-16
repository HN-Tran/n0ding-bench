.PHONY: test vet build run clean

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -trimpath -ldflags="-X github.com/hn-tran/n0ding-bench/internal/bench.BuildVersion=$${VERSION:-0.1.0-dev} -X github.com/hn-tran/n0ding-bench/internal/bench.BuildCommit=$$(git rev-parse --verify HEAD 2>/dev/null || echo unknown)" -o bin/n0ding-bench ./cmd/n0ding-bench

run:
	go run ./cmd/n0ding-bench serve --db bench.db

clean:
	go clean
