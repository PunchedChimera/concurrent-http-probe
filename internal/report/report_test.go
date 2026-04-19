package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/punchedchimera/concurrent-http-probe/internal/report"
	"github.com/punchedchimera/concurrent-http-probe/internal/stats"
)

// makeSummary builds a fully-populated Summary for tests.
func makeSummary() stats.Summary {
	ms := time.Millisecond
	return stats.Summary{
		TotalRequests: 100,
		Successful:    95,
		Failed:        5,
		ErrorRate:     0.05,
		MinLatency:    10 * ms,
		MeanLatency:   50 * ms,
		P50:           45 * ms,
		P95:           90 * ms,
		P99:           150 * ms,
		MaxLatency:    200 * ms,
		Duration:      10 * time.Second,
		Throughput:    9.5,
	}
}

func TestPrint_TableContainsKeyFields(t *testing.T) {
	var buf bytes.Buffer // bytes.Buffer implements io.Writer — no file needed
	report.Print(makeSummary(), []string{"http://example.com"}, report.Options{Writer: &buf})

	output := buf.String()
	checks := []string{
		"http://example.com",
		"100 total",
		"95 OK",
		"5 failed",
		"5.00%",     // error rate
		"9.50 req/s", // throughput
		"P50",
		"P95",
		"P99",
	}
	for _, want := range checks {
		if !strings.Contains(output, want) {
			t.Errorf("table output missing %q\nfull output:\n%s", want, output)
		}
	}
}

func TestPrint_JSONIsValidAndComplete(t *testing.T) {
	var buf bytes.Buffer
	report.Print(makeSummary(), []string{"http://example.com"}, report.Options{
		JSON:   true,
		Writer: &buf,
	})

	// Decode into a generic map so we can check fields without duplicating the struct.
	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("JSON output is invalid: %v\noutput was:\n%s", err, buf.String())
	}

	requiredFields := []string{
		"total_requests",
		"successful",
		"failed",
		"error_rate_pct",
		"throughput_rps",
		"duration_ms",
		"latency_ms",
	}
	for _, field := range requiredFields {
		if _, ok := got[field]; !ok {
			t.Errorf("JSON output missing field %q", field)
		}
	}

	latency, ok := got["latency_ms"].(map[string]interface{})
	if !ok {
		t.Fatal("latency_ms should be an object")
	}
	for _, key := range []string{"min", "mean", "p50", "p95", "p99", "max"} {
		if _, ok := latency[key]; !ok {
			t.Errorf("latency_ms missing key %q", key)
		}
	}
}

func TestPrint_JSONValues(t *testing.T) {
	var buf bytes.Buffer
	report.Print(makeSummary(), []string{"http://a.test"}, report.Options{
		JSON:   true,
		Writer: &buf,
	})

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if v := got["total_requests"].(float64); v != 100 {
		t.Errorf("total_requests = %v, want 100", v)
	}
	if v := got["error_rate_pct"].(float64); v != 5.0 {
		t.Errorf("error_rate_pct = %v, want 5.0", v)
	}
}

func TestPrint_ZeroSummary(t *testing.T) {
	// Ensure printing a zero-value Summary doesn't panic.
	var buf bytes.Buffer
	report.Print(stats.Summary{}, []string{}, report.Options{Writer: &buf})
	// If we reach here without panicking, the test passes.
}

func TestPrint_MultipleURLs(t *testing.T) {
	var buf bytes.Buffer
	urls := []string{"http://a.test", "http://b.test"}
	report.Print(makeSummary(), urls, report.Options{Writer: &buf})

	if !strings.Contains(buf.String(), "http://a.test") {
		t.Error("output should contain first URL")
	}
	if !strings.Contains(buf.String(), "http://b.test") {
		t.Error("output should contain second URL")
	}
}

func TestFmtDuration_Ranges(t *testing.T) {
	// We test fmtDuration indirectly through Print output.
	// Sub-ms, ms-range, and s-range durations should all format correctly.
	tests := []struct {
		latency time.Duration
		want    string
	}{
		{500 * time.Microsecond, "µs"},
		{25 * time.Millisecond, "ms"},
		{2 * time.Second, "s"},
		{0, "N/A"},
	}

	for _, tt := range tests {
		s := stats.Summary{
			TotalRequests: 1,
			Successful:    1,
			MinLatency:    tt.latency,
			MeanLatency:   tt.latency,
			P50:           tt.latency,
			P95:           tt.latency,
			P99:           tt.latency,
			MaxLatency:    tt.latency,
			Duration:      time.Second,
		}
		var buf bytes.Buffer
		report.Print(s, []string{"http://test"}, report.Options{Writer: &buf})
		if !strings.Contains(buf.String(), tt.want) {
			t.Errorf("for latency %v, expected output to contain %q\ngot:\n%s",
				tt.latency, tt.want, buf.String())
		}
	}
}
