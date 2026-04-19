# concurrent-http-probe

A lightweight CLI tool that fires concurrent HTTP requests against one or more endpoints and reports latency statistics, throughput, and error rates. Think a cut-down version of [hey](https://github.com/rakyll/hey) or [wrk](https://github.com/wg/wrk).

[![CI](https://github.com/PunchedChimera/concurrent-http-probe/actions/workflows/ci.yml/badge.svg)](https://github.com/PunchedChimera/concurrent-http-probe/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/PunchedChimera/concurrent-http-probe)](https://goreportcard.com/report/github.com/PunchedChimera/concurrent-http-probe)

## Features

- Concurrent requests via a goroutine worker pool
- Latency percentiles: P50, P95, P99
- Throughput (req/s) and error rate
- Multiple target URLs in a single run
- JSON output for scripting and dashboards
- Custom HTTP method and headers
- Configurable per-request timeout

## Installation

**Pre-built binary** — download from the [Releases](https://github.com/PunchedChimera/concurrent-http-probe/releases) page (Linux, macOS, Windows; amd64 + arm64).

**From source** — requires Go 1.22+:

```bash
go install github.com/PunchedChimera/concurrent-http-probe/cmd/probe@latest
```

## Usage

```
probe -u <url> [flags]

Flags:
  -u, --url string[]          Target URL (repeatable)
  -n, --requests int          Number of requests per URL (default 100)
  -c, --concurrency int       Number of concurrent workers (default 10)
  -t, --timeout duration      Per-request timeout, e.g. 500ms, 5s (default 10s)
  -m, --method string         HTTP method: GET, POST, HEAD… (default "GET")
  -H, --header string[]       HTTP header (repeatable)
      --json                  Output results as JSON
      --keep-alive            Reuse TCP connections (default true)
  -h, --help                  Help for probe
```

### Examples

**Basic probe:**
```bash
probe -u https://example.com
```

**Higher load — 500 requests across 50 workers:**
```bash
probe -u https://example.com -n 500 -c 50
```

**Multiple endpoints in one run:**
```bash
probe -u https://api.example.com/health -u https://api.example.com/ready -n 200 -c 20
```

**With an auth header:**
```bash
probe -u https://api.example.com/data -H "Authorization: Bearer $TOKEN"
```

**JSON output (pipe to jq):**
```bash
probe -u https://example.com --json | jq '.latency_ms.p99'
```

**HEAD requests with a short timeout:**
```bash
probe -u https://example.com -m HEAD -t 2s -n 1000 -c 100
```

### Sample output

```
Probing [https://example.com] — 100 requests × 10 workers × 10s timeout...

  Target URLs:   https://example.com
  Requests:      100 total, 100 OK, 0 failed
  Error rate:    0.00%
  Throughput:    47.23 req/s
  Duration:      2.117s

  Latency
  ───────
  Min:           18.45ms
  Mean:          34.12ms
  P50:           31.80ms
  P95:           67.44ms
  P99:           94.21ms
  Max:           101.33ms
```

## Building from source

```bash
git clone https://github.com/PunchedChimera/concurrent-http-probe.git
cd concurrent-http-probe

# Run tests
go test ./...

# Build binary
go build -o bin/probe ./cmd/probe

# Run
./bin/probe -u https://example.com -n 50 -c 10
```

## How it works

The concurrency model is a classic **worker pool**:

```
             jobs channel            results channel
Run() ──────────────────► workers ──────────────────► collector
        (buffered, N total)  (N goroutines)         (drains to slice)
```

Each worker goroutine reads URLs from a shared jobs channel, fires the HTTP request, and writes a `Result` to the results channel. Closing the jobs channel signals workers to exit — no mutexes, no shared mutable state.

Statistics (percentiles, throughput, error rate) are computed in a separate pure package with no I/O, making them independently testable.

## CI / CD

GitHub Actions runs on every push and pull request:

- Tests on **Linux, macOS, and Windows** across Go 1.22 and 1.23
- Race detector (`-race`) on Linux
- Coverage gate (≥ 90%)
- `golangci-lint`

Tagged releases (`v*.*.*`) trigger [GoReleaser](https://goreleaser.com), which produces cross-platform binaries and a GitHub Release automatically.

## License

MIT
