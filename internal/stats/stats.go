// Package stats computes latency percentiles, throughput, and error rates
// from a slice of probe results.
//
// Go packages map 1:1 to directories. Unlike Java, there's no class hierarchy —
// just functions and types grouped by directory. The package name is the last
// segment of the import path by convention (e.g. "stats" not "probe_stats").
package stats

import (
	"math"
	"sort"
	"time"
)

// Result holds the outcome of a single HTTP request.
//
// Go structs are value types by default (copied on assignment), unlike Java
// objects which are always references. We pass *Result (pointer) when we want
// reference semantics, and Result (value) when copying is fine.
type Result struct {
	URL        string
	StatusCode int
	Latency    time.Duration // time.Duration is just an int64 of nanoseconds
	Error      error         // nil means no error — errors are values, not exceptions
}

// Summary holds aggregated statistics for a complete probe run.
// All duration fields are zero if there were no successful requests.
type Summary struct {
	TotalRequests int
	Successful    int
	Failed        int

	MinLatency  time.Duration
	MaxLatency  time.Duration
	MeanLatency time.Duration
	P50         time.Duration
	P95         time.Duration
	P99         time.Duration

	Duration   time.Duration // wall-clock time for the entire run
	Throughput float64       // successful requests per second
	ErrorRate  float64       // fraction failed: 0.0–1.0
}

// Calculate computes a Summary from a slice of Results.
//
// Key Go idiom: multiple return values. Go functions commonly return
// (value, error) — the caller is forced to handle errors explicitly.
// Here we return just Summary because an empty slice is a valid non-error state.
func Calculate(results []Result, duration time.Duration) Summary {
	// len() is a built-in, not a method. There's no .size() in Go.
	if len(results) == 0 {
		return Summary{} // zero-value struct — all fields are zero/nil/""
	}

	s := Summary{
		TotalRequests: len(results),
		Duration:      duration,
	}

	// In Go, 'var' declares with a zero value. For slices, the zero value is nil,
	// which is safe to append to — no NullPointerException equivalent.
	var latencies []time.Duration
	var totalLatency time.Duration

	// 'range' over a slice gives (index, value). '_' discards the index —
	// Go enforces that every declared variable is used, so '_' is the idiom
	// for "I know this exists but don't need it here."
	for _, r := range results {
		if r.Error != nil || r.StatusCode >= 400 {
			s.Failed++
			continue
		}
		s.Successful++
		// 'append' returns a new slice header — always reassign.
		// This is unlike Java's List.add() which mutates in place.
		latencies = append(latencies, r.Latency)
		totalLatency += r.Latency
	}

	if s.TotalRequests > 0 {
		s.ErrorRate = float64(s.Failed) / float64(s.TotalRequests)
	}

	// Throughput only counts wall-clock time, not just successful requests,
	// so it reflects real-world capacity including failures.
	if duration > 0 {
		s.Throughput = float64(s.Successful) / duration.Seconds()
	}

	if len(latencies) == 0 {
		return s
	}

	// sort.Slice takes the slice and a "less than" function — Go passes functions
	// as first-class values, like Java's Comparator but without the boilerplate.
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	s.MinLatency = latencies[0]
	s.MaxLatency = latencies[len(latencies)-1]
	s.MeanLatency = totalLatency / time.Duration(len(latencies))
	s.P50 = percentile(latencies, 50)
	s.P95 = percentile(latencies, 95)
	s.P99 = percentile(latencies, 99)

	return s
}

// percentile uses the nearest-rank method on a pre-sorted slice.
//
// Lowercase first letter = unexported (package-private in Java terms).
// Go's entire visibility system is just: uppercase = exported, lowercase = not.
// No public/protected/private keywords.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	// math.Ceil returns float64; we cast to int for slice indexing.
	// Go requires explicit numeric type conversions — no implicit widening.
	idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
