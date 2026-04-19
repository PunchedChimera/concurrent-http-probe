// Package probe implements the concurrent HTTP request engine.
//
// The core design uses Go's worker-pool pattern:
//
//	┌─────────────┐     jobs chan     ┌──────────┐     results chan     ┌───────────┐
//	│ Run() feeds │ ───────────────► │  Worker  │ ──────────────────► │ collector │
//	│  jobs in   │                  │ goroutine│                      │ goroutine │
//	└─────────────┘                  └──────────┘                      └───────────┘
//
// Java mental model: think ThreadPoolExecutor with a LinkedBlockingQueue<URL>
// feeding workers that write to another BlockingQueue<Result>. But goroutines
// are ~2 KB vs ~1 MB for OS threads, so spinning up 1000 is routine in Go.
package probe

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/punchedchimera/concurrent-http-probe/internal/stats"
)

// Config holds all parameters for a probe run.
// Exported fields (uppercase) are set by the CLI layer.
type Config struct {
	URLs        []string
	Requests    int           // total requests per URL
	Concurrency int           // number of parallel workers
	Timeout     time.Duration // per-request HTTP timeout
	Method      string        // HTTP method: GET, POST, HEAD, etc.
	Headers     []string      // raw "Key: Value" header strings
	KeepAlive   bool          // reuse TCP connections (default true)
}

// job is an unexported type — it's the unit of work passed through the jobs channel.
// Embedding the URL and request number lets us track which attempt this is.
type job struct {
	url       string
	requestID int
}

// Run fires all HTTP requests concurrently and collects results.
//
// It returns a slice of Result (one per request) and the wall-clock duration.
// The context allows the caller to cancel the entire run (e.g. Ctrl+C).
//
// Key Go concept: functions can return multiple values. The convention is
// (result, error) — callers must check the error before using the result.
func Run(ctx context.Context, cfg Config) ([]stats.Result, time.Duration, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, 0, err
	}

	client := buildClient(cfg)
	headers, err := parseHeaders(cfg.Headers)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid header: %w", err)
	}

	totalJobs := len(cfg.URLs) * cfg.Requests

	// Buffered channels decouple producer and consumer.
	// A buffered channel of size N can hold N items without blocking the sender.
	// An unbuffered channel (make(chan T)) blocks sender until receiver is ready.
	// Java analogy: ArrayBlockingQueue vs SynchronousQueue.
	jobs := make(chan job, totalJobs)
	results := make(chan stats.Result, totalJobs)

	// sync.WaitGroup is the Go equivalent of Java's CountDownLatch.
	// Add(n) sets the counter, Done() decrements, Wait() blocks until zero.
	var wg sync.WaitGroup

	// Fan-out: spawn exactly cfg.Concurrency worker goroutines.
	// 'go func()' launches a goroutine — extremely cheap (2KB stack, grows as needed).
	// The goroutine runs concurrently with this function; it's not a thread per se.
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			// 'defer' schedules this call to run when the enclosing function returns.
			// It's like Java's finally block, but stacked LIFO and much more ergonomic.
			// Here it ensures wg.Done() is called even if the worker panics.
			defer wg.Done()
			worker(ctx, client, cfg.Method, headers, jobs, results)
		}()
	}

	start := time.Now()

	// Feed all jobs into the channel.
	// Since jobs is buffered to totalJobs, this loop never blocks.
	for _, u := range cfg.URLs {
		for i := 0; i < cfg.Requests; i++ {
			jobs <- job{url: u, requestID: i}
		}
	}
	// Closing a channel signals "no more data" to all receivers.
	// Workers' 'for j := range jobs' loops will exit when the channel is drained and closed.
	// This is the idiomatic Go shutdown signal — never close from the receiver side.
	close(jobs)

	// Wait for all workers to finish, then close results so the collector below can drain it.
	// We do this in a goroutine so we don't block here — we need to drain results concurrently.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all results from the channel.
	// 'for r := range results' reads until the channel is closed and empty.
	collected := make([]stats.Result, 0, totalJobs)
	for r := range results {
		collected = append(collected, r)
	}

	return collected, time.Since(start), nil
}

// worker is the function each goroutine runs. It reads jobs from the channel
// and writes results back — it knows nothing about how many workers exist.
func worker(
	ctx context.Context,
	client *http.Client,
	method string,
	headers http.Header,
	jobs <-chan job, // '<-chan' means receive-only — compile-time enforced
	results chan<- stats.Result, // 'chan<-' means send-only
) {
	// 'for j := range jobs' blocks waiting for the next job, and exits cleanly
	// when the jobs channel is closed. This is the canonical worker loop in Go.
	for j := range jobs {
		// Check if the context has been cancelled (e.g. user pressed Ctrl+C).
		// select with a default case is non-blocking — it falls through if no
		// channel operation is immediately ready.
		select {
		case <-ctx.Done():
			return
		default:
		}

		results <- doRequest(ctx, client, method, headers, j.url)
	}
}

// doRequest executes a single HTTP request and returns a Result.
// All timing and error handling is contained here.
func doRequest(
	ctx context.Context,
	client *http.Client,
	method string,
	headers http.Header,
	url string,
) stats.Result {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return stats.Result{URL: url, Error: fmt.Errorf("build request: %w", err)}
	}

	// Copy headers onto the request.
	for key, vals := range headers {
		for _, v := range vals {
			req.Header.Add(key, v)
		}
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		return stats.Result{URL: url, Latency: latency, Error: err}
	}
	// Always drain and close the body — Go's HTTP client reuses connections only
	// if the previous response body was fully consumed. Forgetting this is a
	// common Go gotcha that causes connection pool exhaustion under load.
	defer resp.Body.Close()

	return stats.Result{
		URL:        url,
		StatusCode: resp.StatusCode,
		Latency:    latency,
	}
}

// buildClient constructs an http.Client tuned for load-testing.
func buildClient(cfg Config) *http.Client {
	transport := &http.Transport{
		// DisableKeepAlives forces a new TCP connection per request.
		// For latency testing you usually want keep-alives ON (default),
		// but the flag lets users simulate cold connections.
		DisableKeepAlives: !cfg.KeepAlive,

		// These match Go's defaults but are listed explicitly for clarity.
		MaxIdleConns:        cfg.Concurrency * 2,
		MaxIdleConnsPerHost: cfg.Concurrency,
	}
	return &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
		// Don't follow redirects automatically during probing —
		// a redirect is itself a signal worth capturing.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// parseHeaders converts ["Content-Type: application/json"] into http.Header.
func parseHeaders(raw []string) (http.Header, error) {
	h := http.Header{}
	for _, s := range raw {
		// fmt.Sscanf would work but is fragile with values containing ':'.
		// We split on the first ':' only.
		for i, c := range s {
			if c == ':' {
				key := s[:i]
				val := s[i+1:]
				// strings.TrimSpace equivalent — trim the leading space after ':'
				if len(val) > 0 && val[0] == ' ' {
					val = val[1:]
				}
				h.Add(key, val)
				goto next
			}
		}
		return nil, fmt.Errorf("header %q missing ': ' separator", s)
	next:
	}
	return h, nil
}

// validateConfig returns an error for any obviously invalid configuration.
func validateConfig(cfg Config) error {
	if len(cfg.URLs) == 0 {
		return fmt.Errorf("at least one URL is required")
	}
	if cfg.Requests <= 0 {
		return fmt.Errorf("--requests must be > 0")
	}
	if cfg.Concurrency <= 0 {
		return fmt.Errorf("--concurrency must be > 0")
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("--timeout must be > 0")
	}
	return nil
}
