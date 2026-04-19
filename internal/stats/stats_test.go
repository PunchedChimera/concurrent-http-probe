// Package stats_test uses the '_test' suffix — a black-box test package.
// It can only access exported symbols from 'stats', which forces you to test
// the public API rather than implementation details. This is the Go equivalent
// of a separate test class in Java that only uses public methods.
//
// For white-box tests (testing unexported functions), you'd use 'package stats'
// (no suffix) in the test file — both styles can coexist in the same directory.
package stats_test

import (
	"testing"
	"time"

	"github.com/punchedchimera/concurrent-http-probe/internal/stats"
)

// TestCalculate uses the table-driven test pattern — idiomatic Go.
// Define all scenarios as a slice of anonymous structs, then loop.
// This is the Go equivalent of JUnit 5's @ParameterizedTest but with zero
// framework overhead. The go test runner reports each subtest individually.
func TestCalculate(t *testing.T) {
	ms := time.Millisecond // local alias for readability

	tests := []struct {
		name     string
		results  []stats.Result
		duration time.Duration
		want     stats.Summary
	}{
		{
			name:     "empty results returns zero summary",
			results:  []stats.Result{},
			duration: time.Second,
			want:     stats.Summary{},
		},
		{
			name: "all successful requests",
			results: []stats.Result{
				{URL: "http://a.test", StatusCode: 200, Latency: 100 * ms},
				{URL: "http://a.test", StatusCode: 200, Latency: 200 * ms},
				{URL: "http://a.test", StatusCode: 200, Latency: 300 * ms},
			},
			duration: time.Second,
			want: stats.Summary{
				TotalRequests: 3,
				Successful:    3,
				Failed:        0,
				ErrorRate:     0.0,
				MinLatency:    100 * ms,
				MaxLatency:    300 * ms,
				MeanLatency:   200 * ms,
				// Nearest-rank: P50 of 3 → ceil(0.5*3)=2 → index 1 → 200ms
				P50:        200 * ms,
				P95:        300 * ms,
				P99:        300 * ms,
				Duration:   time.Second,
				Throughput: 3.0,
			},
		},
		{
			name: "mixed success and network error",
			results: []stats.Result{
				{URL: "http://a.test", StatusCode: 200, Latency: 50 * ms},
				{URL: "http://a.test", Error: errTimeout},
				{URL: "http://a.test", StatusCode: 200, Latency: 150 * ms},
				{URL: "http://a.test", Error: errTimeout},
			},
			duration: 2 * time.Second,
			want: stats.Summary{
				TotalRequests: 4,
				Successful:    2,
				Failed:        2,
				ErrorRate:     0.5,
				MinLatency:    50 * ms,
				MaxLatency:    150 * ms,
				MeanLatency:   100 * ms,
				// Nearest-rank P50 of [50ms, 150ms]: ceil(0.5*2)=1 → index 0 → 50ms
				P50:           50 * ms,
				P95:           150 * ms,
				P99:           150 * ms,
				Duration:      2 * time.Second,
				Throughput:    1.0,
			},
		},
		{
			name: "HTTP 4xx counts as failure",
			results: []stats.Result{
				{URL: "http://a.test", StatusCode: 404, Latency: 80 * ms},
				{URL: "http://a.test", StatusCode: 500, Latency: 120 * ms},
			},
			duration: time.Second,
			want: stats.Summary{
				TotalRequests: 2,
				Successful:    0,
				Failed:        2,
				ErrorRate:     1.0,
				Duration:      time.Second,
				Throughput:    0.0,
			},
		},
		{
			name: "single result",
			results: []stats.Result{
				{URL: "http://a.test", StatusCode: 201, Latency: 75 * ms},
			},
			duration: 500 * time.Millisecond,
			want: stats.Summary{
				TotalRequests: 1,
				Successful:    1,
				Failed:        0,
				ErrorRate:     0.0,
				MinLatency:    75 * ms,
				MaxLatency:    75 * ms,
				MeanLatency:   75 * ms,
				P50:           75 * ms,
				P95:           75 * ms,
				P99:           75 * ms,
				Duration:      500 * time.Millisecond,
				Throughput:    2.0,
			},
		},
		{
			name: "all errors — no latency stats",
			results: []stats.Result{
				{URL: "http://a.test", Error: errTimeout},
				{URL: "http://a.test", Error: errTimeout},
			},
			duration: time.Second,
			want: stats.Summary{
				TotalRequests: 2,
				Successful:    0,
				Failed:        2,
				ErrorRate:     1.0,
				Duration:      time.Second,
				Throughput:    0.0,
			},
		},
		{
			name: "zero duration does not divide by zero",
			results: []stats.Result{
				{URL: "http://a.test", StatusCode: 200, Latency: 100 * ms},
			},
			duration: 0,
			want: stats.Summary{
				TotalRequests: 1,
				Successful:    1,
				Failed:        0,
				ErrorRate:     0.0,
				MinLatency:    100 * ms,
				MaxLatency:    100 * ms,
				MeanLatency:   100 * ms,
				P50:           100 * ms,
				P95:           100 * ms,
				P99:           100 * ms,
				Duration:      0,
				Throughput:    0.0,
			},
		},
		{
			name: "percentile accuracy with 100 evenly spaced results",
			results: func() []stats.Result {
				// Anonymous function invoked immediately — Go's way of computing
				// a complex slice literal inline. Similar to a Java initializer block.
				r := make([]stats.Result, 100)
				for i := range r {
					r[i] = stats.Result{
						StatusCode: 200,
						Latency:    time.Duration(i+1) * ms,
					}
				}
				return r
			}(),
			duration: 10 * time.Second,
			want: stats.Summary{
				TotalRequests: 100,
				Successful:    100,
				Failed:        0,
				ErrorRate:     0.0,
				MinLatency:    1 * ms,
				MaxLatency:    100 * ms,
				MeanLatency:   50*ms + 500*time.Microsecond, // (1+100)/2 * ms
				// Nearest-rank: P50 → ceil(0.5*100)=50 → index 49 → 50ms
				P50:        50 * ms,
				P95:        95 * ms,
				P99:        99 * ms,
				Duration:   10 * time.Second,
				Throughput: 10.0,
			},
		},
	}

	for _, tt := range tests {
		// t.Run creates a named subtest. On failure you'll see:
		//   --- FAIL: TestCalculate/all_successful_requests
		// You can run a single subtest with: go test -run TestCalculate/all_successful
		t.Run(tt.name, func(t *testing.T) {
			got := stats.Calculate(tt.results, tt.duration)
			if got != tt.want {
				// %+v prints struct fields with their names — essential for debugging.
				t.Errorf("Calculate() =\n  got:  %+v\n  want: %+v", got, tt.want)
			}
		})
	}
}

// errTimeout is a package-level test error sentinel.
// In Go, errors are often compared by identity using errors.Is(), so named
// sentinels are idiomatic (like Java's specific exception classes).
var errTimeout = &testError{msg: "connection timed out"}

type testError struct{ msg string }

// Error() satisfies the built-in 'error' interface.
// Go interfaces are implicit — any type with an Error() string method IS an error.
// No 'implements' keyword required.
func (e *testError) Error() string { return e.msg }
