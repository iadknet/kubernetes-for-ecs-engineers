// Package claudecli is a chat.Provider backed by the Claude Code CLI running in
// headless mode (`claude --print`).
//
// The subprocess runs in the server's working directory, and that is what
// grounds its answers: started at the repository root it inherits AGENTS.md and
// the repo's teaching skills, so "explain this card" comes back in the ECS
// framing the curriculum uses. The root specifically — from a subdirectory the
// CLI resolves no skills and leaves CLAUDE.md's @AGENTS.md import unexpanded,
// which is why `make run-chat` starts the server from there. Auth is the CLI's
// own login state — a local subscription — so this process never reads, stores,
// or forwards a credential.
//
// Configuration:
//
//	CLAUDE_BIN  path to the claude binary (default "claude", resolved on PATH)
//	CHAT_MODEL  the model used when a request names none (default "sonnet")
//
// New validates both. A missing binary or an unknown model fails at startup,
// not on the first question.
package claudecli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/iadk/k8s-flashcards/internal/chat"
)

// The CLI exposes no programmatic listing, so what it accepts is named here and
// validated against — an unknown value is rejected before a subprocess starts.
var (
	models  = []string{"sonnet", "opus", "haiku"}
	efforts = []string{"low", "medium", "high", "xhigh", "max"}
)

// tools is the read-only set the subprocess gets. --tools replaces the built-in
// set outright, so this is a ceiling rather than a preference: the session has
// no Bash, Edit, or Write to reach for. File reads are not scoped to the repo;
// what limits that is the loopback guard on the handlers above.
const tools = "Read,Grep,Glob"

// tailBytes bounds how much subprocess output an error or log line quotes.
// Enough to identify a failure, small enough that a chatty process cannot grow
// the heap through it.
const tailBytes = 4 << 10

// Provider talks to the Claude Code CLI, holding the current conversation's
// session id — what --resume needs to make a follow-up question a follow-up
// rather than a fresh start.
type Provider struct {
	bin   string
	model string

	// One conversation, one turn at a time. This is a single-user app and a
	// session cannot have two turns in flight, so a turn holds the lock for its
	// whole run rather than racing another onto the same session id.
	mu        sync.Mutex
	sessionID string
}

// New builds a provider from the environment, failing if the CLI it needs is
// not installed or the configured model is not one it offers.
func New() (*Provider, error) {
	bin := os.Getenv("CLAUDE_BIN")
	if bin == "" {
		bin = "claude"
	}

	resolved, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("claude cli: locating %q (set CLAUDE_BIN): %w", bin, err)
	}

	model := os.Getenv("CHAT_MODEL")
	if model == "" {
		model = models[0]
	}

	if !slices.Contains(models, model) {
		return nil, fmt.Errorf("claude cli: CHAT_MODEL %q is not one of %s", model, strings.Join(models, ", "))
	}

	return &Provider{bin: resolved, model: model}, nil
}

// Options reports what this provider accepts. The slices are copies: the caller
// renders and validates against them, and neither should be able to edit the
// provider's own list.
func (p *Provider) Options() chat.Options {
	return chat.Options{
		Models:       slices.Clone(models),
		DefaultModel: p.model,
		Efforts:      slices.Clone(efforts),
	}
}

// Reset drops the session id, so the next turn starts a new conversation.
func (p *Provider) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.sessionID = ""
}

// Send runs one turn, emitting the answer's text as the CLI streams it.
func (p *Provider) Send(ctx context.Context, turn chat.Turn, emit func(delta string) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// The working directory is inherited on purpose — running inside the repo is
	// what gives the subprocess AGENTS.md and the repo's skills.
	//
	//nolint:gosec // G204: bin comes from CLAUDE_BIN (operator config, validated by
	// LookPath at startup) and every argument is app-composed; the user's message is
	// one argv element passed to exec directly, never through a shell.
	cmd := exec.CommandContext(ctx, p.bin, p.args(turn)...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("claude cli: opening stdout: %w", err)
	}

	stderr := &tail{max: tailBytes}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("claude cli: starting %s: %w", p.bin, err)
	}

	session, streamErr := stream(stdout, emit)
	if streamErr != nil {
		// Wait must not be called while the pipe still owes bytes to a reader that
		// has stopped reading, or the child blocks writing into it forever.
		_ = cmd.Process.Kill()
	}

	waitErr := cmd.Wait()
	code := cmd.ProcessState.ExitCode()

	switch {
	case ctx.Err() != nil:
		slog.Warn("chat turn cancelled", "session_id", session, "exit_code", code)

		return fmt.Errorf("claude cli: %w", ctx.Err())
	case streamErr != nil:
		slog.Error("chat stream failed",
			"session_id", session, "exit_code", code, "stderr", stderr.String(), "error", streamErr)

		return streamErr
	case waitErr != nil:
		slog.Error("chat subprocess failed",
			"session_id", session, "exit_code", code, "stderr", stderr.String(), "error", waitErr)

		return fmt.Errorf("claude cli: %w (exit %d): %s", waitErr, code, stderr.String())
	}

	slog.Info("chat turn", "session_id", session, "exit_code", code)

	// Only a completed turn advances the conversation. Resuming a session whose
	// turn was killed halfway would replay a half-finished exchange.
	if session != "" {
		p.sessionID = session
	}

	return nil
}

// args builds one invocation. Callers hold p.mu: this reads the session id.
func (p *Provider) args(turn chat.Turn) []string {
	model := turn.Model
	if model == "" {
		model = p.model
	}

	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--tools", tools,
		"--model", model,
	}

	if turn.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", turn.SystemPrompt)
	}

	if turn.Effort != "" {
		args = append(args, "--effort", turn.Effort)
	}

	// Every turn after the first resumes the same conversation. --model is still
	// passed and overrides the resumed session's model, so switching models
	// mid-conversation costs no context.
	if p.sessionID != "" {
		args = append(args, "--resume", p.sessionID)
	}

	return append(args, prompt(turn))
}

// prompt puts the card block ahead of the question, so the model reads what is
// on screen before it reads what was asked about it.
func prompt(turn chat.Turn) string {
	if turn.CardContext == "" {
		return turn.Message
	}

	return turn.CardContext + "\n\n" + turn.Message
}

// event is the slice of the CLI's stream-json shape this app depends on.
// Everything else in an event is deliberately not decoded, so a release that
// adds fields changes nothing here.
type event struct {
	SessionID string `json:"session_id"`
	Event     struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
}

// stream decodes the NDJSON event stream, emitting assistant text as it arrives
// and returning the session id the events carry.
//
// A release that changes the shape rather than extending it fails the turn with
// the tail of what was actually read — the only way the log can show what moved.
func stream(r io.Reader, emit func(delta string) error) (string, error) {
	var session string

	raw := &tail{max: tailBytes}
	dec := json.NewDecoder(io.TeeReader(r, raw))

	for {
		var e event

		if err := dec.Decode(&e); err != nil {
			if errors.Is(err, io.EOF) {
				return session, nil
			}

			return session, fmt.Errorf("claude cli: unreadable event stream (output tail: %q): %w", raw.String(), err)
		}

		if e.SessionID != "" {
			session = e.SessionID
		}

		// Thinking deltas ride the same channel and are not the answer.
		if e.Event.Type != "content_block_delta" || e.Event.Delta.Type != "text_delta" {
			continue
		}

		if err := emit(e.Event.Delta.Text); err != nil {
			return session, fmt.Errorf("claude cli: relaying the answer: %w", err)
		}
	}
}

// tail keeps the last max bytes written to it. Subprocess output is unbounded;
// an error message that quotes it must not be.
type tail struct {
	max int
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}

	return len(p), nil
}

func (t *tail) String() string { return strings.TrimSpace(string(t.buf)) }
