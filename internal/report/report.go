// Package report formats and prints probe summaries to stdout.
// Keeping formatting separate from logic is idiomatic in Go —
// it makes the stats package independently testable without any I/O.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/punchedchimera/concurrent-http-probe/internal/stats"
)

// Options controls output behaviour.
type Options struct {
	JSON   bool // emit machine-readable JSON instead of the table
	Writer io.Writer
}

// Print writes the summary to w in the format specified by opts.
// Accepting an io.Writer (interface) instead of *os.File makes this
// trivially testable — pass a bytes.Buffer in tests, os.Stdout in prod.
// This is the Go equivalent of dependency injection without a framework.
func Print(s stats.Summary, urls []string, opts Options) {
	if opts.JSON {
		printJSON(s, opts.Writer)
		return
	}
	printTable(s, urls, opts.Writer)
}

// jsonSummary mirrors stats.Summary with JSON tags.
// Go struct tags (the backtick strings) annotate fields for encoders.
// encoding/json uses them to control key names and omitempty behaviour.
type jsonSummary struct {
	URLs          []string `json:"urls"`
	TotalRequests int      `json:"total_requests"`
	Successful    int      `json:"successful"`
	Failed        int      `json:"failed"`
	ErrorRatePct  float64  `json:"error_rate_pct"`
	ThroughputRPS float64  `json:"throughput_rps"`
	DurationMs    float64  `json:"duration_ms"`
	LatencyMs     struct {
		Min  float64 `json:"min"`
		Mean float64 `json:"mean"`
		P50  float64 `json:"p50"`
		P95  float64 `json:"p95"`
		P99  float64 `json:"p99"`
		Max  float64 `json:"max"`
	} `json:"latency_ms"`
}

func printJSON(s stats.Summary, w io.Writer) {
	j := jsonSummary{
		TotalRequests: s.TotalRequests,
		Successful:    s.Successful,
		Failed:        s.Failed,
		ErrorRatePct:  s.ErrorRate * 100,
		ThroughputRPS: s.Throughput,
		DurationMs:    float64(s.Duration) / float64(time.Millisecond),
	}
	j.LatencyMs.Min = msf(s.MinLatency)
	j.LatencyMs.Mean = msf(s.MeanLatency)
	j.LatencyMs.P50 = msf(s.P50)
	j.LatencyMs.P95 = msf(s.P95)
	j.LatencyMs.P99 = msf(s.P99)
	j.LatencyMs.Max = msf(s.MaxLatency)

	// json.MarshalIndent pretty-prints with 2-space indentation.
	// The error is intentionally ignored here: marshalling a known struct
	// with only primitive fields cannot fail.
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(j)
}

func printTable(s stats.Summary, urls []string, w io.Writer) {
	// text/tabwriter aligns columns by padding with spaces.
	// NewWriter(output, minwidth, tabwidth, padding, padchar, flags)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	// fmt.Fprintln writes to any io.Writer — the 'F' prefix is the Go
	// convention for "formatted write to writer" (like fprintf in C).
	fmt.Fprintln(tw, "")
	fmt.Fprintf(tw, "  Target URLs:\t%s\n", strings.Join(urls, ", "))
	fmt.Fprintf(tw, "  Requests:\t%d total, %d OK, %d failed\n",
		s.TotalRequests, s.Successful, s.Failed)
	fmt.Fprintf(tw, "  Error rate:\t%.2f%%\n", s.ErrorRate*100)
	fmt.Fprintf(tw, "  Throughput:\t%.2f req/s\n", s.Throughput)
	fmt.Fprintf(tw, "  Duration:\t%s\n", s.Duration.Round(time.Millisecond))
	fmt.Fprintln(tw, "")
	fmt.Fprintln(tw, "  Latency\t")
	fmt.Fprintln(tw, "  ───────\t")
	fmt.Fprintf(tw, "  Min:\t%s\n", fmtDuration(s.MinLatency))
	fmt.Fprintf(tw, "  Mean:\t%s\n", fmtDuration(s.MeanLatency))
	fmt.Fprintf(tw, "  P50:\t%s\n", fmtDuration(s.P50))
	fmt.Fprintf(tw, "  P95:\t%s\n", fmtDuration(s.P95))
	fmt.Fprintf(tw, "  P99:\t%s\n", fmtDuration(s.P99))
	fmt.Fprintf(tw, "  Max:\t%s\n", fmtDuration(s.MaxLatency))
	fmt.Fprintln(tw, "")
	// Flush must be called to write buffered output. tabwriter buffers everything
	// until Flush so it can compute column widths across all rows.
	tw.Flush()
}

// fmtDuration formats a duration for human consumption, choosing the most
// readable unit (µs for sub-millisecond, ms for sub-second, s for the rest).
func fmtDuration(d time.Duration) string {
	switch {
	case d == 0:
		return "N/A"
	case d < time.Millisecond:
		return fmt.Sprintf("%.0fµs", float64(d)/float64(time.Microsecond))
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.3fs", d.Seconds())
	}
}

// msf converts a Duration to milliseconds as a float64, for JSON output.
func msf(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
