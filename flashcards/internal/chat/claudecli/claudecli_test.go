// Nothing here runs in parallel: every test drives a constructor that reads its
// configuration from the environment, and t.Setenv is incompatible with
// t.Parallel.
//
//nolint:paralleltest // t.Setenv, which every test needs, cannot run in parallel.
package claudecli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/iadk/k8s-flashcards/internal/chat"
	"github.com/iadk/k8s-flashcards/internal/chat/claudecli"
)

// A canned stream in the shape the real CLI emits, recorded from
// `claude -p --output-format stream-json --verbose --include-partial-messages`.
// It is the contract this package parses against: a thinking delta that must
// not reach the user, two text deltas that must, and the session id that makes
// the next turn a follow-up.
const successStream = `{"type":"system","subtype":"init","session_id":"sess-1","tools":["Glob","Grep","Read"]}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"not the answer"}},"session_id":"sess-1"}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"A Pod "}},"session_id":"sess-1"}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"is an ECS Task."}},"session_id":"sess-1"}
{"type":"stream_event","event":{"type":"message_stop"},"session_id":"sess-1"}
{"is_error":false,"type":"result","subtype":"success","session_id":"sess-1"}`

// The stub frames its argument log so the parse survives an argument that
// contains newlines — the prompt does, and flattening it would let a broken
// prompt still look right.
// defaultModel is the CHAT_MODEL every provider under test is built with.
const defaultModel = "sonnet"

const (
	callMark = "###\n"
	argMark  = "@@\n"
)

// stubCLI writes a script standing in for the claude binary and returns it with
// the file it logs its arguments to. body is the shell that produces the stub's
// output.
//
// This is the seam that keeps `make check` free of network, auth, and quota.
func stubCLI(t *testing.T, body string) (bin, argsFile string) {
	t.Helper()

	dir := t.TempDir()
	bin = filepath.Join(dir, "claude")
	argsFile = filepath.Join(dir, "args")

	script := "#!/bin/sh\n" +
		"{ echo '###'; for a in \"$@\"; do echo '@@'; printf '%s\\n' \"$a\"; done; } >> " + argsFile + "\n" +
		body + "\n"

	//nolint:gosec // G306: a stub the test then executes needs its executable bit.
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatalf("writing stub: %v", err)
	}

	return bin, argsFile
}

// emits wraps a canned stdout stream in a stub body.
func emits(stream string) string { return "cat <<'STREAM'\n" + stream + "\nSTREAM" }

// invocations splits the stub's argument log into one argument list per call.
func invocations(t *testing.T, argsFile string) [][]string {
	t.Helper()

	raw, err := os.ReadFile(argsFile) //nolint:gosec // G304: a path this test just created in t.TempDir().
	if err != nil {
		t.Fatalf("reading stub args: %v", err)
	}

	var calls [][]string

	for block := range strings.SplitSeq(string(raw), callMark) {
		if block == "" {
			continue
		}

		var args []string

		for arg := range strings.SplitSeq(block, argMark) {
			if arg == "" {
				continue
			}

			args = append(args, strings.TrimSuffix(arg, "\n"))
		}

		calls = append(calls, args)
	}

	return calls
}

// flag returns the value following name, and whether it was passed at all.
func flag(args []string, name string) (string, bool) {
	i := slices.Index(args, name)
	if i < 0 || i+1 >= len(args) {
		return "", false
	}

	return args[i+1], true
}

func newProvider(t *testing.T, bin string) *claudecli.Provider {
	t.Helper()

	t.Setenv("CLAUDE_BIN", bin)
	t.Setenv("CHAT_MODEL", defaultModel)

	p, err := claudecli.New()
	if err != nil {
		t.Fatalf("building provider: %v", err)
	}

	return p
}

// send runs one turn and collects everything emitted.
func send(t *testing.T, p *claudecli.Provider, turn chat.Turn) (string, error) {
	t.Helper()

	var b strings.Builder

	err := p.Send(t.Context(), turn, func(delta string) error {
		b.WriteString(delta)
		return nil
	})

	return b.String(), err
}

func TestNewValidatesItsConfiguration(t *testing.T) {
	bin, _ := stubCLI(t, emits(successStream))

	t.Run("missing binary", func(t *testing.T) {
		t.Setenv("CLAUDE_BIN", filepath.Join(t.TempDir(), "not-installed"))

		if _, err := claudecli.New(); err == nil {
			t.Fatal("New accepted a binary that does not exist")
		} else if !strings.Contains(err.Error(), "not-installed") {
			t.Errorf("error should name the binary it looked for, got: %v", err)
		}
	})

	t.Run("unknown model", func(t *testing.T) {
		t.Setenv("CLAUDE_BIN", bin)
		t.Setenv("CHAT_MODEL", "gpt-9")

		if _, err := claudecli.New(); err == nil {
			t.Fatal("New accepted a model the CLI does not offer")
		} else if !strings.Contains(err.Error(), "gpt-9") {
			t.Errorf("error should name the rejected model, got: %v", err)
		}
	})
}

func TestOptionsReportTheDocumentedModelsAndEfforts(t *testing.T) {
	bin, _ := stubCLI(t, emits(successStream))
	opts := newProvider(t, bin).Options()

	if got, want := opts.Models, []string{defaultModel, "opus", "haiku"}; !slices.Equal(got, want) {
		t.Errorf("Models = %v, want %v", got, want)
	}

	if got, want := opts.Efforts, []string{"low", "medium", "high", "xhigh", "max"}; !slices.Equal(got, want) {
		t.Errorf("Efforts = %v, want %v", got, want)
	}

	if opts.DefaultModel != defaultModel {
		t.Errorf("DefaultModel = %q, want the CHAT_MODEL the provider was built with", opts.DefaultModel)
	}
}

func TestSendStreamsTextDeltasInOrder(t *testing.T) {
	bin, _ := stubCLI(t, emits(successStream))

	got, err := send(t, newProvider(t, bin), chat.Turn{Message: "what is a pod?", Model: defaultModel})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if want := "A Pod is an ECS Task."; got != want {
		t.Errorf("emitted %q, want %q", got, want)
	}

	if strings.Contains(got, "not the answer") {
		t.Error("thinking deltas reached the user")
	}
}

func TestSendComposesTheInvocation(t *testing.T) {
	bin, argsFile := stubCLI(t, emits(successStream))
	p := newProvider(t, bin)

	turn := chat.Turn{
		Message:      "how does this relate to ECS?",
		CardContext:  "Card: what is a Pod?",
		SystemPrompt: "you are a tutor",
		Model:        "haiku",
	}

	if _, err := send(t, p, turn); err != nil {
		t.Fatalf("Send: %v", err)
	}

	args := invocations(t, argsFile)[0]

	for _, tt := range []struct {
		name, flag, expected string
	}{
		{"streaming output", "--output-format", "stream-json"},
		{"read-only tool set", "--tools", "Read,Grep,Glob"},
		{"requested model", "--model", "haiku"},
		{"tutoring instructions", "--append-system-prompt", "you are a tutor"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := flag(args, tt.flag); !ok || got != tt.expected {
				t.Errorf("%s = %q (present: %v), want %q", tt.flag, got, ok, tt.expected)
			}
		})
	}

	t.Run("no effort dial unless asked", func(t *testing.T) {
		if _, ok := flag(args, "--effort"); ok {
			t.Error("--effort was passed for a turn that set none")
		}
	})

	t.Run("card context precedes the question", func(t *testing.T) {
		prompt := args[len(args)-1]
		if !strings.HasPrefix(prompt, turn.CardContext) || !strings.HasSuffix(prompt, turn.Message) {
			t.Errorf("prompt = %q, want the card block then the question", prompt)
		}
	})
}

// A follow-up question is only a follow-up if it resumes the session the first
// turn opened — that is the whole of "how does this relate to the previous
// card?" working.
func TestSendResumesTheConversation(t *testing.T) {
	bin, argsFile := stubCLI(t, emits(successStream))
	p := newProvider(t, bin)

	for _, turn := range []chat.Turn{
		{Message: "first", Model: defaultModel},
		{Message: "second", Model: "opus", Effort: "high"},
	} {
		if _, err := send(t, p, turn); err != nil {
			t.Fatalf("Send(%q): %v", turn.Message, err)
		}
	}

	calls := invocations(t, argsFile)
	if len(calls) != 2 {
		t.Fatalf("stub was called %d times, want 2", len(calls))
	}

	if _, ok := flag(calls[0], "--resume"); ok {
		t.Error("the first turn resumed a session that did not exist yet")
	}

	if got, ok := flag(calls[1], "--resume"); !ok || got != "sess-1" {
		t.Errorf("second turn --resume = %q (present: %v), want the first turn's session id", got, ok)
	}

	// Switching models mid-conversation must not need a reset: --model overrides
	// the resumed session's model, so the session id survives the switch.
	if got, _ := flag(calls[1], "--model"); got != "opus" {
		t.Errorf("second turn --model = %q, want the newly selected model", got)
	}

	if got, ok := flag(calls[1], "--effort"); !ok || got != "high" {
		t.Errorf("second turn --effort = %q (present: %v), want high", got, ok)
	}

	p.Reset()

	if _, err := send(t, p, chat.Turn{Message: "third", Model: defaultModel}); err != nil {
		t.Fatalf("Send after Reset: %v", err)
	}

	if _, ok := flag(invocations(t, argsFile)[2], "--resume"); ok {
		t.Error("Reset left the old session in place")
	}
}

// The subprocess spends a subscription and reads files. When the request that
// asked for it goes away, it must go away too.
func TestSendKillsTheSubprocessWhenTheRequestIsCancelled(t *testing.T) {
	// exec, not a plain sleep: the shell must be *replaced* by the sleep, or
	// killing the child leaves the sleep holding the stdout pipe open.
	bin, _ := stubCLI(t, emits(successStream)+"\nexec sleep 30")
	p := newProvider(t, bin)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	start := time.Now()

	// Cancelling from the first delta makes this deterministic: the process is
	// provably alive and mid-stream at the moment the request is abandoned.
	err := p.Send(ctx, chat.Turn{Message: "what is a pod?"}, func(string) error {
		cancel()
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send returned %v, want a context.Canceled error", err)
	}

	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Send took %s: the subprocess outlived the request", elapsed)
	}
}

func TestSendReportsWhatTheCLIFailedWith(t *testing.T) {
	t.Run("non-zero exit", func(t *testing.T) {
		bin, _ := stubCLI(t, "echo 'Invalid API key · Please run /login' >&2\nexit 3")

		_, err := send(t, newProvider(t, bin), chat.Turn{Message: "hello"})
		if err == nil {
			t.Fatal("Send ignored a failed subprocess")
		}

		for _, want := range []string{"3", "Please run /login"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q should carry %q", err, want)
			}
		}
	})

	// The stream-json shape is the CLI's, not ours. If a release changes it, the
	// turn must fail loudly with what was actually read, so the log shows what
	// moved rather than the panel silently going quiet.
	t.Run("unparseable output", func(t *testing.T) {
		bin, _ := stubCLI(t, "echo 'a brand new plain-text format'")

		_, err := send(t, newProvider(t, bin), chat.Turn{Message: "hello"})
		if err == nil {
			t.Fatal("Send accepted output it could not parse")
		}

		if !strings.Contains(err.Error(), "a brand new plain-text format") {
			t.Errorf("error %q should carry the output tail", err)
		}
	})
}
