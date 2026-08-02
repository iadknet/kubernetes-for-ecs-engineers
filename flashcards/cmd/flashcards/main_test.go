package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// discard is a logger for tests that exercise run(): the wiring under test is
// the error it returns, not what it says on the way there.
func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestEnvBool(t *testing.T) {
	for _, tt := range []struct {
		name     string
		value    string
		fallback bool
		expected bool
	}{
		{"unset takes the fallback", "", false, false},
		{"unset keeps a true fallback", "", true, true},
		{"true", "true", false, true},
		{"1", "1", false, true},
		{"mixed case", "True", false, true},
		{"false overrides a true fallback", "false", true, false},
		{"0 overrides a true fallback", "0", true, false},
		{"unparseable takes the fallback", "yes please", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_ENV_BOOL", tt.value)

			if got := envBool("TEST_ENV_BOOL", tt.fallback); got != tt.expected {
				t.Errorf("envBool(%q, %v) = %v, want %v", tt.value, tt.fallback, got, tt.expected)
			}
		})
	}
}

// Every response passes through statusRecorder. A wrapper that does not unwrap
// is invisible to http.ResponseController, which is how the chat stream both
// flushes each delta and clears the server's 30s WriteTimeout — so without this
// the panel dies on its first delta and no unit test on the handler notices.
func TestStatusRecorderStaysTransparentToResponseController(t *testing.T) {
	t.Parallel()

	// Over a real connection, not a recorder: a write deadline only exists on
	// one, and the assertion is that the wrapper does not hide it.
	srv := httptest.NewServer(logging(discard(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rc := http.NewResponseController(w)

		if err := rc.SetWriteDeadline(time.Time{}); err != nil {
			t.Errorf("SetWriteDeadline through the wrapper: %v", err)
		}

		if _, err := io.WriteString(w, "streaming"); err != nil {
			t.Errorf("writing: %v", err)
		}

		if err := rc.Flush(); err != nil {
			t.Errorf("Flush through the wrapper: %v", err)
		}
	})))
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: the handler under test did not run", res.StatusCode)
	}
}

// A misconfigured chat provider must be a boot failure, not a panel that 500s
// on first use: the operator finds out at deploy time either way, and only one
// of those is cheap.
func TestRunRejectsUnknownChatProvider(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("CHAT_ENABLED", "true")
	t.Setenv("CHAT_PROVIDER", "bogus")

	err := run(discard())
	if err == nil {
		t.Fatal("run() accepted an unknown CHAT_PROVIDER")
	}

	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should name the unknown provider, got: %v", err)
	}
}

// Provider-specific validation belongs to the provider, but it has to reach the
// process exit code: chat enabled on a machine with no claude installed is a
// misconfiguration, and the operator should learn that at boot.
func TestRunRejectsAMissingClaudeBinary(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-installed")

	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("CHAT_ENABLED", "true")
	t.Setenv("CHAT_PROVIDER", "claude-cli")
	t.Setenv("CLAUDE_BIN", missing)

	err := run(discard())
	if err == nil {
		t.Fatal("run() started with chat enabled and no claude binary")
	}

	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error should name the binary it looked for, got: %v", err)
	}
}
