// Package main is the entry point for the probe CLI.
//
// In Go there can only be one 'main' package per binary, and it must contain
// exactly one 'main()' function. Unlike Java, there's no class wrapping it.
// The package declaration 'package main' tells the compiler this is an executable,
// not a library.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/punchedchimera/concurrent-http-probe/internal/probe"
	"github.com/punchedchimera/concurrent-http-probe/internal/report"
	"github.com/punchedchimera/concurrent-http-probe/internal/stats"
)

// Version is set at build time via ldflags:
//
//	go build -ldflags "-X main.Version=1.0.0"
//
// This is Go's equivalent of Maven's ${project.version} filtering.
// When not set (local dev builds), it falls back to "dev".
var Version = "dev"

func main() {
	// cobra.Command is the central type in the Cobra CLI framework.
	// Think of it as a command descriptor — it holds the flags, help text,
	// and the function to run. Cobra handles --help, completion, and subcommands.
	root := buildRootCommand()

	// os.Exit(1) on error. Cobra prints the error for us.
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// buildRootCommand constructs the cobra command tree.
// Extracting this from main() makes the CLI testable — you can call
// buildRootCommand() in a test and invoke it with args.
func buildRootCommand() *cobra.Command {
	// Flags are declared as local variables here and captured by the Run closure.
	// This avoids global state — a common Go anti-pattern to avoid.
	var (
		urls        []string
		requests    int
		concurrency int
		timeout     time.Duration
		method      string
		headers     []string
		jsonOutput  bool
		keepAlive   bool
	)

	cmd := &cobra.Command{
		// The Use string defines the command name and argument syntax shown in --help.
		Use:   "probe",
		Short: "Concurrent HTTP probe — measure latency and throughput of HTTP endpoints",
		Long: `probe fires concurrent HTTP requests against one or more endpoints and reports:

  • Latency percentiles: P50, P95, P99
  • Min / mean / max latency
  • Throughput (req/s)
  • Error rate

Example:
  probe -u https://example.com -u https://api.example.com -n 200 -c 20`,

		// SilenceUsage prevents cobra from printing the full usage block on every
		// error — only show it for flag-parsing errors, not runtime errors.
		SilenceUsage: true,

		// RunE is like Run but returns an error. Cobra handles printing and exit code.
		// The 'E' suffix is a Go convention for "error-returning variant".
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := probe.Config{
				URLs:        urls,
				Requests:    requests,
				Concurrency: concurrency,
				Timeout:     timeout,
				Method:      method,
				Headers:     headers,
				KeepAlive:   keepAlive,
			}

			// Signal handling: listen for Ctrl+C (SIGINT) and SIGTERM.
			// signal.NotifyContext returns a context that is cancelled when the
			// process receives one of the listed signals. The stop function
			// should be deferred to release resources.
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			fmt.Fprintf(cmd.OutOrStdout(),
				"Probing %v — %d requests × %d workers × %v timeout...\n",
				urls, requests, concurrency, timeout)

			results, duration, err := probe.Run(ctx, cfg)
			if err != nil {
				return fmt.Errorf("probe failed: %w", err)
			}

			summary := stats.Calculate(results, duration)
			report.Print(summary, urls, report.Options{
				JSON:   jsonOutput,
				Writer: cmd.OutOrStdout(),
			})

			return nil
		},
	}

	// StringArrayVar binds a []string flag that can be repeated:  -u a -u b
	// StringSliceVar would parse  -u a,b  as two values — ArrayVar is clearer for URLs.
	cmd.Flags().StringArrayVarP(&urls, "url", "u", nil,
		"target URL (repeatable: -u https://a.com -u https://b.com)")

	cmd.Flags().IntVarP(&requests, "requests", "n", 100,
		"number of requests per URL")

	cmd.Flags().IntVarP(&concurrency, "concurrency", "c", 10,
		"number of concurrent workers")

	// DurationVar parses Go duration strings: "500ms", "2s", "1m30s"
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 10*time.Second,
		"per-request timeout (e.g. 500ms, 5s)")

	cmd.Flags().StringVarP(&method, "method", "m", "GET",
		"HTTP method: GET, POST, HEAD, PUT, DELETE")

	cmd.Flags().StringArrayVarP(&headers, "header", "H", nil,
		"HTTP header (repeatable: -H 'Authorization: Bearer token')")

	cmd.Flags().BoolVar(&jsonOutput, "json", false,
		"output results as JSON")

	cmd.Flags().BoolVar(&keepAlive, "keep-alive", true,
		"reuse TCP connections between requests")

	// MarkRequired causes cobra to error if -u is not provided.
	if err := cmd.MarkFlagRequired("url"); err != nil {
		// This can only fail if "url" isn't registered above — a programming error,
		// not a runtime error. Panic is appropriate for programmer mistakes.
		panic(err)
	}

	// Version subcommand — good practice for any CLI tool.
	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the probe version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "probe %s\n", Version)
		},
	})

	return cmd
}
