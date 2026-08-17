FROM golang:1.25-alpine AS build
ARG VERSION=0.1.0-dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/hn-tran/n0ding-bench/internal/bench.BuildVersion=${VERSION} -X github.com/hn-tran/n0ding-bench/internal/bench.BuildCommit=${COMMIT}" -o /out/n0ding-bench ./cmd/n0ding-bench \
    && mkdir -p /out/data \
    && chown 65532:65532 /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build --chown=65532:65532 /out/data /data
WORKDIR /data
COPY --from=build /out/n0ding-bench /usr/local/bin/n0ding-bench
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/n0ding-bench"]
CMD ["serve", "--addr", "0.0.0.0:8080", "--db", "/data/bench.db"]
