package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBuildRootCommand_Version checks the version subcommand.
// Testing main() directly is impractical, so we test buildRootCommand()
// which contains all the logic. This is a common pattern in Go CLI testing.
func TestBuildRootCommand_Version(t *testing.T) {
	cmd := buildRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	if !strings.Contains(buf.String(), "probe") {
		t.Errorf("version output should contain 'probe', got: %q", buf.String())
	}
}

func TestBuildRootCommand_MissingURL(t *testing.T) {
	cmd := buildRootCommand()
	cmd.SetArgs([]string{"-n", "1"})

	// We expect an error because --url is required.
	if err := cmd.Execute(); err == nil {
		t.Error("expected error when --url is missing")
	}
}

func TestBuildRootCommand_InvalidFlag(t *testing.T) {
	cmd := buildRootCommand()
	cmd.SetArgs([]string{"--unknown-flag"})

	if err := cmd.Execute(); err == nil {
		t.Error("expected error for unknown flag")
	}
}

func TestBuildRootCommand_RunProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cmd := buildRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	cmd.SetArgs([]string{
		"-u", srv.URL,
		"-n", "5",
		"-c", "2",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("probe command failed: %v", err)
	}

	output := buf.String()
	// Verify the output contains key stats fields.
	for _, want := range []string{"Probing", "total", "req/s"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, output)
		}
	}
}

func TestBuildRootCommand_JSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cmd := buildRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	cmd.SetArgs([]string{
		"-u", srv.URL,
		"-n", "3",
		"-c", "1",
		"--json",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("probe command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"total_requests"`) {
		t.Errorf("JSON output missing total_requests\ngot: %s", output)
	}
}

func TestBuildRootCommand_CustomMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cmd := buildRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	cmd.SetArgs([]string{
		"-u", srv.URL,
		"-n", "1",
		"-c", "1",
		"-m", "HEAD",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("probe command failed: %v", err)
	}

	if gotMethod != http.MethodHead {
		t.Errorf("expected method HEAD, got %q", gotMethod)
	}
}

func TestBuildRootCommand_CustomTimeout(t *testing.T) {
	// Verify that a short timeout causes timeouts against a slow server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler is slow — but our timeout should fire first.
		// We use a blocking read on Done() instead of time.Sleep so the
		// test goroutine itself is not blocked when the server exits.
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cmd := buildRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	cmd.SetArgs([]string{
		"-u", srv.URL,
		"-n", "2",
		"-c", "1",
		"-t", "10ms",
	})

	// Run should not return an error even if all requests time out —
	// timeouts are captured as Result.Error, not as a top-level failure.
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}

	// The output should show 100% error rate.
	if !strings.Contains(buf.String(), "100.00%") {
		t.Errorf("expected 100%% error rate for timed-out requests\ngot:\n%s", buf.String())
	}
}
