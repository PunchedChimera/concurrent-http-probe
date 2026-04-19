package probe_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/punchedchimera/concurrent-http-probe/internal/probe"
)

// newTestServer is a helper that spins up an in-process HTTP server.
// httptest.NewServer is part of the standard library — no mocking framework needed.
// The server is real: it listens on a random port on 127.0.0.1.
// t.Cleanup registers a teardown function, equivalent to @AfterEach in JUnit.
func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper() // marks this as a helper so stack traces point to the caller
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_BasicSuccess(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cfg := probe.Config{
		URLs:        []string{srv.URL},
		Requests:    10,
		Concurrency: 3,
		Timeout:     5 * time.Second,
		Method:      http.MethodGet,
		KeepAlive:   true,
	}

	results, duration, err := probe.Run(context.Background(), cfg)

	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}
	if duration <= 0 {
		t.Errorf("expected positive duration, got %v", duration)
	}
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("result[%d]: unexpected error: %v", i, r.Error)
		}
		if r.StatusCode != http.StatusOK {
			t.Errorf("result[%d]: expected status 200, got %d", i, r.StatusCode)
		}
	}
}

func TestRun_MultipleURLs(t *testing.T) {
	srv1 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv2 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	cfg := probe.Config{
		URLs:        []string{srv1.URL, srv2.URL},
		Requests:    5,
		Concurrency: 2,
		Timeout:     5 * time.Second,
		Method:      http.MethodGet,
		KeepAlive:   true,
	}

	results, _, err := probe.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// 2 URLs × 5 requests = 10 total
	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}
}

func TestRun_ServerErrors(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	cfg := probe.Config{
		URLs:        []string{srv.URL},
		Requests:    5,
		Concurrency: 2,
		Timeout:     5 * time.Second,
		Method:      http.MethodGet,
		KeepAlive:   true,
	}

	results, _, err := probe.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	for _, r := range results {
		if r.StatusCode != 500 {
			t.Errorf("expected status 500, got %d", r.StatusCode)
		}
	}
}

func TestRun_CustomHeaders(t *testing.T) {
	var receivedAuth string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	cfg := probe.Config{
		URLs:        []string{srv.URL},
		Requests:    1,
		Concurrency: 1,
		Timeout:     5 * time.Second,
		Method:      http.MethodGet,
		Headers:     []string{"Authorization: Bearer test-token"},
		KeepAlive:   true,
	}

	_, _, err := probe.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if receivedAuth != "Bearer test-token" {
		t.Errorf("expected Authorization header 'Bearer test-token', got %q", receivedAuth)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	// This test verifies that cancelling the context stops work promptly.
	// The server sleeps so requests pile up — cancellation should cut them short.
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	cfg := probe.Config{
		URLs:        []string{srv.URL},
		Requests:    100,
		Concurrency: 5,
		Timeout:     5 * time.Second,
		Method:      http.MethodGet,
		KeepAlive:   true,
	}

	results, _, err := probe.Run(ctx, cfg)
	// We don't fail on error here — context cancellation may surface as an error
	// or as partial results depending on timing.
	_ = err

	// We should have processed far fewer than 100 requests.
	if len(results) >= 100 {
		t.Errorf("expected cancellation to stop early, but got %d results", len(results))
	}
}

func TestRun_Validation(t *testing.T) {
	// Table-driven validation tests — each row exercises one invalid config.
	tests := []struct {
		name    string
		cfg     probe.Config
		wantErr string
	}{
		{
			name:    "no URLs",
			cfg:     probe.Config{Requests: 1, Concurrency: 1, Timeout: time.Second, Method: "GET"},
			wantErr: "URL",
		},
		{
			name:    "zero requests",
			cfg:     probe.Config{URLs: []string{"http://x.test"}, Requests: 0, Concurrency: 1, Timeout: time.Second, Method: "GET"},
			wantErr: "--requests",
		},
		{
			name:    "zero concurrency",
			cfg:     probe.Config{URLs: []string{"http://x.test"}, Requests: 1, Concurrency: 0, Timeout: time.Second, Method: "GET"},
			wantErr: "--concurrency",
		},
		{
			name:    "zero timeout",
			cfg:     probe.Config{URLs: []string{"http://x.test"}, Requests: 1, Concurrency: 1, Timeout: 0, Method: "GET"},
			wantErr: "--timeout",
		},
		{
			name:    "malformed header",
			cfg:     probe.Config{URLs: []string{"http://x.test"}, Requests: 1, Concurrency: 1, Timeout: time.Second, Method: "GET", Headers: []string{"NoSeparator"}},
			wantErr: "separator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := probe.Run(context.Background(), tt.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRun_Concurrency_RaceDetector(t *testing.T) {
	// This test is designed to be run with 'go test -race'.
	// The race detector instruments memory accesses and reports data races.
	// If there's a shared-memory bug in the worker pool, it will show up here.
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cfg := probe.Config{
		URLs:        []string{srv.URL},
		Requests:    50,
		Concurrency: 20,
		Timeout:     5 * time.Second,
		Method:      http.MethodGet,
		KeepAlive:   true,
	}

	results, _, err := probe.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 50 {
		t.Errorf("expected 50 results, got %d", len(results))
	}
}

func TestRun_InvalidURL(t *testing.T) {
	cfg := probe.Config{
		URLs:        []string{"://bad-url"},
		Requests:    1,
		Concurrency: 1,
		Timeout:     time.Second,
		Method:      http.MethodGet,
		KeepAlive:   true,
	}

	results, _, err := probe.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() should not return a top-level error for bad URLs: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected result to contain an error for an invalid URL")
	}
}

func TestRun_HEAD_Method(t *testing.T) {
	var receivedMethod string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	})

	cfg := probe.Config{
		URLs:        []string{srv.URL},
		Requests:    1,
		Concurrency: 1,
		Timeout:     5 * time.Second,
		Method:      http.MethodHead,
		KeepAlive:   true,
	}

	_, _, err := probe.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if receivedMethod != http.MethodHead {
		t.Errorf("expected method HEAD, got %q", receivedMethod)
	}
}

func TestRun_LatencyIsRecorded(t *testing.T) {
	delay := 20 * time.Millisecond
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	})

	cfg := probe.Config{
		URLs:        []string{srv.URL},
		Requests:    3,
		Concurrency: 1,
		Timeout:     5 * time.Second,
		Method:      http.MethodGet,
		KeepAlive:   true,
	}

	results, _, err := probe.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	for i, r := range results {
		if r.Latency < delay {
			t.Errorf("result[%d]: latency %v should be >= server delay %v", i, r.Latency, delay)
		}
	}
}

func TestRun_RedirectNotFollowed(t *testing.T) {
	// Verifies that the probe captures the redirect response (301/302) rather
	// than silently following it, since redirect latency is meaningful signal.
	redirectTarget := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusMovedPermanently)
	})

	cfg := probe.Config{
		URLs:        []string{srv.URL},
		Requests:    1,
		Concurrency: 1,
		Timeout:     5 * time.Second,
		Method:      http.MethodGet,
		KeepAlive:   true,
	}

	results, _, err := probe.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if results[0].StatusCode != http.StatusMovedPermanently {
		t.Errorf("expected 301, got %d", results[0].StatusCode)
	}
}

func TestRun_ParseHeaders_Valid(t *testing.T) {
	// Verify that a header with a colon in the value is parsed correctly.
	// e.g. "Authorization: Bearer abc:def" — the split should only occur on first colon.
	var got string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	cfg := probe.Config{
		URLs:        []string{srv.URL},
		Requests:    1,
		Concurrency: 1,
		Timeout:     5 * time.Second,
		Method:      http.MethodGet,
		Headers:     []string{fmt.Sprintf("Authorization: Bearer %s", "abc:def")},
		KeepAlive:   true,
	}
	_, _, err := probe.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Bearer abc:def" {
		t.Errorf("got %q, want %q", got, "Bearer abc:def")
	}
}
