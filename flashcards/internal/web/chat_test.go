package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/iadk/k8s-flashcards/internal/chat"
	"github.com/iadk/k8s-flashcards/internal/deck"
	"github.com/iadk/k8s-flashcards/internal/review"
	"github.com/iadk/k8s-flashcards/internal/web"
)

// The term is one the card's own text never mentions, so a context block that
// merely echoed the question could not pass the prerequisite assertion.
const chatDeck = `
deck: Chat test deck
module: M1
cards:
  - id: term-kubelet
    term: Kubelet
    q: |
      Kubelet
    a: |
      The node agent that runs Pods.
  - id: chat-card
    q: |
      Why would a Pod hold more than one container?
    a: |
      Sidecars share the network namespace.
    requires: [term-kubelet]
`

// fakeProvider stands in for a backend, recording the turn it was handed and
// replaying a canned answer. It is the same seam a future provider swap
// exercises — nothing above chat.Provider knows which one is installed.
type fakeProvider struct {
	options chat.Options
	deltas  []string

	mu     sync.Mutex
	turn   chat.Turn
	turns  int
	resets int
}

func (f *fakeProvider) Send(_ context.Context, turn chat.Turn, emit func(delta string) error) error {
	f.mu.Lock()
	f.turn = turn
	f.turns++
	f.mu.Unlock()

	for _, d := range f.deltas {
		if err := emit(d); err != nil {
			return err
		}
	}

	return nil
}

func (f *fakeProvider) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.resets++
}

func (f *fakeProvider) Options() chat.Options { return f.options }

func (f *fakeProvider) lastTurn() chat.Turn {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.turn
}

func (f *fakeProvider) resetCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.resets
}

func (f *fakeProvider) turnCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.turns
}

// newFake is a provider with both dials: three models and an effort scale.
func newFake() *fakeProvider {
	return &fakeProvider{
		options: chat.Options{
			Models:       []string{"sonnet", "opus", "haiku"},
			DefaultModel: "sonnet",
			Efforts:      []string{"low", "high"},
		},
		deltas: []string{"A Pod is ", "an ECS Task."},
	}
}

func chatServer(t *testing.T, p chat.Provider) http.Handler {
	t.Helper()

	lib, err := deck.Load(fstest.MapFS{"decks/chat.yaml": {Data: []byte(chatDeck)}}, "decks")
	if err != nil {
		t.Fatalf("loading chat deck: %v", err)
	}

	store, err := review.Open(filepath.Join(t.TempDir(), "review.json"), 20)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := web.New(lib, store, web.Config{Chat: p})
	if err != nil {
		t.Fatalf("building server: %v", err)
	}

	return srv.Routes()
}

// panelRequest is what the panel's own fetch looks like on the wire: loopback
// address, loopback Host, JSON body. The guard tests work by breaking exactly
// one of those at a time.
func panelRequest(t *testing.T, target, body string) *http.Request {
	t.Helper()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:54321"
	r.Host = "localhost:8080"
	r.Header.Set("Content-Type", "application/json")

	return r
}

func serve(t *testing.T, h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	return rec
}

// ask posts a well-formed chat request from the given address.
func ask(t *testing.T, h http.Handler, target, remoteAddr, body string) *httptest.ResponseRecorder {
	t.Helper()

	r := panelRequest(t, target, body)
	r.RemoteAddr = remoteAddr

	return serve(t, h, r)
}

func TestChatStreamsAnAnswerGroundedInTheCard(t *testing.T) {
	t.Parallel()

	fake := newFake()
	h := chatServer(t, fake)

	rec := ask(t, h, "/chat", "127.0.0.1:54321",
		`{"card":"chat-card","message":"how does this relate to ECS?"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want an SSE stream", ct)
	}

	body := rec.Body.String()
	for _, want := range []string{"event: delta", `"A Pod is "`, `"an ECS Task."`, "event: done"} {
		if !strings.Contains(body, want) {
			t.Errorf("stream is missing %s:\n%s", want, body)
		}
	}

	turn := fake.lastTurn()

	t.Run("card context", func(t *testing.T) {
		t.Parallel()

		for _, want := range []string{
			"Why would a Pod hold more than one container?", // the front
			"Sidecars share the network namespace.",         // the back
			"Kubelet",                                       // the terms it requires
			"Chat test deck",
		} {
			if !strings.Contains(turn.CardContext, want) {
				t.Errorf("card context is missing %q:\n%s", want, turn.CardContext)
			}
		}
	})

	t.Run("question and tutoring instructions", func(t *testing.T) {
		t.Parallel()

		if turn.Message != "how does this relate to ECS?" {
			t.Errorf("Message = %q", turn.Message)
		}

		if turn.SystemPrompt == "" {
			t.Error("the turn carried no tutoring system prompt")
		}
	})

	// Omitted knobs fall back to what the provider itself reports, not to a
	// default duplicated in the web layer.
	t.Run("provider defaults", func(t *testing.T) {
		t.Parallel()

		if turn.Model != "sonnet" {
			t.Errorf("Model = %q, want the provider's DefaultModel", turn.Model)
		}

		if turn.Effort != "" {
			t.Errorf("Effort = %q, want empty when the request named none", turn.Effort)
		}
	})
}

func TestChatPassesTheSelectedModelAndEffort(t *testing.T) {
	t.Parallel()

	fake := newFake()
	h := chatServer(t, fake)

	rec := ask(t, h, "/chat", "127.0.0.1:54321",
		`{"card":"chat-card","message":"explain","model":"opus","effort":"high"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	if turn := fake.lastTurn(); turn.Model != "opus" || turn.Effort != "high" {
		t.Errorf("provider got model %q effort %q, want opus/high", turn.Model, turn.Effort)
	}
}

// Every allowlist is the provider's Options — the web layer holds no list of
// model or effort names of its own.
func TestChatRejectsWhatTheProviderDoesNotOffer(t *testing.T) {
	t.Parallel()

	h := chatServer(t, newFake())

	for _, tt := range []struct {
		name string
		body string
	}{
		{"a card not in the library", `{"card":"no-such-card","message":"explain"}`},
		{"unknown model", `{"card":"chat-card","message":"explain","model":"gpt-9"}`},
		{"unknown effort", `{"card":"chat-card","message":"explain","effort":"ludicrous"}`},
		{"empty message", `{"card":"chat-card","message":"   "}`},
		{"malformed body", `not json`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if rec := ask(t, h, "/chat", "127.0.0.1:54321", tt.body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

// A provider with no effort dial must not be sent one, and must not have a
// selector rendered for it.
func TestChatWithoutAnEffortDial(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.options.Efforts = nil
	h := chatServer(t, fake)

	t.Run("any effort is rejected", func(t *testing.T) {
		t.Parallel()

		rec := ask(t, h, "/chat", "127.0.0.1:54321", `{"card":"chat-card","message":"explain","effort":"low"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("no selector is rendered", func(t *testing.T) {
		t.Parallel()

		if body := do(t, h, http.MethodGet, "/drill").Body.String(); strings.Contains(body, "chat-effort") {
			t.Error("an effort selector was rendered for a provider that has no effort dial")
		}
	})
}

// The server binds every interface so a port-forward works. The chat routes
// spend a subscription and read the filesystem, so they must not follow it out
// onto the network.
func TestChatAnswersLoopbackOnly(t *testing.T) {
	t.Parallel()

	fake := newFake()
	h := chatServer(t, fake)

	for _, tt := range []struct {
		name, remoteAddr string
		expected         int
	}{
		{"ipv4 loopback", "127.0.0.1:54321", http.StatusOK},
		{"ipv6 loopback", "[::1]:54321", http.StatusOK},
		{"another host on the network", "192.0.2.7:54321", http.StatusForbidden},
		{"unparseable address", "not-an-address", http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := ask(t, h, "/chat", tt.remoteAddr, `{"card":"chat-card","message":"explain"}`)
			if rec.Code != tt.expected {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.expected, rec.Body)
			}
		})
	}

	t.Run("reset is guarded too", func(t *testing.T) {
		t.Parallel()

		if rec := ask(t, h, "/chat/reset", "192.0.2.7:54321", ""); rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}

// A loopback check alone is not a security boundary: the user's own browser is
// on this machine, so a page on any website can POST to 127.0.0.1 and arrive
// looking local. The attacker never sees the answer — but the subscription is
// spent and the provider's file-reading tools have run, which is enough.
func TestChatRefusesRequestsAWebsiteCouldForge(t *testing.T) {
	t.Parallel()

	fake := newFake()
	h := chatServer(t, fake)

	// Registered on the parent, so it runs after every parallel subtest.
	t.Cleanup(func() {
		if got := fake.turnCount(); got != 0 {
			t.Errorf("the provider ran %d turns for requests that should never have reached it", got)
		}
	})

	for _, tt := range []struct {
		name     string
		forge    func(*http.Request)
		expected int
	}{
		{
			// The attack itself. A no-cors fetch may only set a safelisted
			// content type, so requiring application/json means anything that
			// arrives with it has passed a CORS preflight this server never
			// answers.
			name:     "json body smuggled as text/plain",
			forge:    func(r *http.Request) { r.Header.Set("Content-Type", "text/plain;charset=UTF-8") },
			expected: http.StatusUnsupportedMediaType,
		},
		{
			name:     "no content type at all",
			forge:    func(r *http.Request) { r.Header.Del("Content-Type") },
			expected: http.StatusUnsupportedMediaType,
		},
		{
			name:     "a hostile page's origin",
			forge:    func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
			expected: http.StatusForbidden,
		},
		{
			// A sandboxed iframe or a file:// page.
			name:     "an opaque origin",
			forge:    func(r *http.Request) { r.Header.Set("Origin", "null") },
			expected: http.StatusForbidden,
		},
		{
			name:     "the browser itself says cross-site",
			forge:    func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
			expected: http.StatusForbidden,
		},
		{
			// DNS rebinding: evil.example resolves to 127.0.0.1, so the page is
			// same-origin with itself and the request genuinely arrives on
			// loopback. Only the Host header gives it away.
			name: "a domain rebound to loopback",
			forge: func(r *http.Request) {
				r.Host = "evil.example:8080"
				r.Header.Set("Origin", "http://evil.example:8080")
			},
			expected: http.StatusForbidden,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, target := range []string{"/chat", "/chat/reset"} {
				r := panelRequest(t, target, `{"card":"chat-card","message":"explain"}`)
				tt.forge(r)

				if rec := serve(t, h, r); rec.Code != tt.expected {
					t.Errorf("POST %s = %d, want %d (body: %s)", target, rec.Code, tt.expected, rec.Body)
				}
			}
		})
	}
}

// The panel's own request carries the headers a browser attaches to a
// same-origin fetch, and must not be caught by the guards above.
func TestChatAcceptsThePanelsOwnRequest(t *testing.T) {
	t.Parallel()

	h := chatServer(t, newFake())

	for _, tt := range []struct{ name, origin, host string }{
		{"localhost", "http://localhost:8080", "localhost:8080"},
		{"ipv4 loopback", "http://127.0.0.1:8080", "127.0.0.1:8080"},
		{"ipv6 loopback", "http://[::1]:8080", "[::1]:8080"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := panelRequest(t, "/chat", `{"card":"chat-card","message":"explain"}`)
			r.Host = tt.host
			r.Header.Set("Origin", tt.origin)
			r.Header.Set("Sec-Fetch-Site", "same-origin")

			if rec := serve(t, h, r); rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body)
			}
		})
	}
}

func TestChatResetStartsANewConversation(t *testing.T) {
	t.Parallel()

	fake := newFake()
	h := chatServer(t, fake)

	if rec := ask(t, h, "/chat/reset", "127.0.0.1:54321", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNoContent, rec.Body)
	}

	if got := fake.resetCount(); got != 1 {
		t.Errorf("provider was reset %d times, want 1", got)
	}
}

// The selectors are rendered from what the provider reports, so a provider with
// a different model list needs no template change.
func TestChatPanelRendersTheProvidersOptions(t *testing.T) {
	t.Parallel()

	h := chatServer(t, newFake())
	body := do(t, h, http.MethodGet, "/drill").Body.String()

	for _, want := range []string{
		`id="chat-form"`,
		"/static/chat.js",
		`id="chat-effort"`,
		`<option value="opus"`,
		`<option value="haiku"`,
		`<option value="low"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("drill page is missing %q", want)
		}
	}

	// The provider's default must come up preselected rather than whichever
	// option happens to be first.
	if !strings.Contains(body, `<option value="sonnet" selected`) {
		t.Error("the provider's default model is not preselected")
	}
}

// The panel needs the id of the card currently on screen, and htmx swaps that
// card without reloading the page — so the id has to travel with the fragment.
func TestDrillFragmentsCarryTheCardID(t *testing.T) {
	t.Parallel()

	h := chatServer(t, newFake())

	for _, tt := range []struct{ name, method, target string }{
		{"drill page", http.MethodGet, "/drill"},
		{"revealed card", http.MethodPost, "/drill/chat-card/reveal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if body := do(t, h, tt.method, tt.target).Body.String(); !strings.Contains(body, `data-card="`) {
				t.Errorf("%s carries no card id:\n%s", tt.name, body)
			}
		})
	}
}
