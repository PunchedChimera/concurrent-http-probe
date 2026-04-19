# CLAUDE.md — AI Coding Instructions for concurrent-http-probe

This file is read automatically by Claude Code and GitHub Copilot when working
in this repository. It defines coding conventions, architecture decisions, and
constraints. Keep it updated as the project evolves.

---

## What this project is

A production-grade CLI tool written in Go that fires concurrent HTTP requests
against a list of endpoints and reports latency statistics (p50, p95, p99),
error rates, and throughput. Portfolio project demonstrating Go concurrency idioms.

**Target audience:** Platform/SRE teams, Grafana k6 users, anyone who needs a
lightweight alternative to `hey` or `wrk`.

---

## Architecture

```
cmd/probe/main.go          CLI entry point (cobra). No business logic here.
internal/probe/worker.go   Worker-pool concurrency engine (goroutines + channels).
internal/stats/stats.go    Pure statistics calculation. No I/O, fully testable.
internal/report/report.go  Output formatting (table and JSON). Accepts io.Writer.
```

The `internal/` directory enforces that these packages cannot be imported by
external modules — they are implementation details of this binary.

Data flow:
```
Config → probe.Run() → []Result → stats.Calculate() → Summary → report.Print()
```

---

## Go Conventions enforced here

### Error handling
- Always return `(value, error)` from functions that can fail.
- Wrap errors with context: `fmt.Errorf("doing X: %w", err)` — the `%w` verb
  allows callers to use `errors.Is()` / `errors.As()` to unwrap.
- Never `panic()` for runtime errors. Panic only for programmer mistakes
  (e.g., calling `MarkFlagRequired` with a name that doesn't exist).

### Concurrency
- Use channels for communication between goroutines, not shared memory.
- Always close channels from the **sender** side, never the receiver.
- Use `sync.WaitGroup` for fan-out/fan-in synchronization.
- Pass `context.Context` as the first argument to any function that does I/O
  or needs cancellation support. Never store a context in a struct.
- Run `go test -race ./...` — the race detector is mandatory in CI.

### Package design
- One package per directory. Package name = last path segment.
- Uppercase = exported (public API). Lowercase = unexported (package-private).
- Keep `internal/stats` free of I/O so it remains purely testable.
- `report` package accepts `io.Writer` — never hardcode `os.Stdout`.

### Testing
- All tests are table-driven: define a `[]struct{name, input, want}`, range over it.
- Use `httptest.NewServer` for HTTP tests — no mocking frameworks.
- `t.Helper()` in any test helper function so stack traces point at the caller.
- `t.Cleanup()` for teardown instead of defer (works in subtests too).
- 100% coverage target — check with `go test -coverprofile=coverage.out ./...`
  then `go tool cover -func=coverage.out`.
- **Windows dev note:** `-race` requires CGO (a C compiler). On Windows without
  TDM-GCC/MinGW installed, run `go test ./...` instead. CI runs on Linux where
  CGO is available, so the race detector still gates every merge.

### Formatting and linting
- `gofmt -s` is non-negotiable. The formatter is authoritative.
- Run `golangci-lint run` before committing.
- Imports: stdlib first, blank line, then external packages, blank line, then
  internal packages. `goimports` enforces this automatically.

---

## Build commands

```bash
# Run all tests with race detector
go test -race ./...

# Run tests with coverage report
go test -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out   # open in browser

# Build the binary (injects git SHA as version)
go build -ldflags "-X main.Version=$(git rev-parse --short HEAD)" -o bin/probe ./cmd/probe

# Run the binary
./bin/probe -u https://httpbin.org/get -n 50 -c 10

# Lint
golangci-lint run

# Snapshot release (local, no tag required)
goreleaser release --snapshot --clean
```

---

## Release process

1. Ensure all tests pass on `main`.
2. Tag the commit: `git tag v1.0.0 && git push origin v1.0.0`
3. GitHub Actions (release.yml) triggers GoReleaser automatically.
4. Binaries for Linux/macOS/Windows (amd64 + arm64) appear on the GitHub Release page.

---

## What NOT to do

- Do not add dependencies unless absolutely necessary. The std library is rich.
- Do not add a logging framework. `fmt.Fprintf(os.Stderr, ...)` is sufficient.
- Do not use `init()` functions — they make code hard to test and reason about.
- Do not store `context.Context` in structs — always pass it as a function argument.
- Do not use `interface{}` (or `any`) when the concrete type is known.
- Do not skip the race detector in tests (`-race` flag is mandatory in CI).
- Do not follow HTTP redirects silently — the current behavior (capture 3xx as-is)
  is intentional for accurate latency measurement.

---

## Go vs Java mental models (for the owner's reference)

| Java | Go |
|------|----|
| `ThreadPoolExecutor` + `BlockingQueue` | goroutines + buffered channel |
| `CountDownLatch` | `sync.WaitGroup` |
| `try/catch/finally` | `if err != nil` + `defer` |
| `implements Runnable` | any type with the right method signature |
| `public/private/protected` | uppercase = exported, lowercase = unexported |
| `ArrayList<T>` | `[]T` (slice) |
| `HashMap<K,V>` | `map[K]V` |
| `Optional<T>` | `(T, error)` or `(T, bool)` |
| Maven `${project.version}` | `-ldflags "-X main.Version=..."` |
| `@ParameterizedTest` | table-driven tests |
