.PHONY: test build run-bench run-dispatch

test:
	go test ./...

build:
	go build -o bin/n0ding-lab ./cmd/n0ding-lab

run-bench:
	go run ./cmd/n0ding-lab -mode bench

run-dispatch:
	go run ./cmd/n0ding-lab -mode dispatch
