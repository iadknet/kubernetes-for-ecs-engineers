package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/iadk/k8s-flashcards/internal/chat"
	"github.com/iadk/k8s-flashcards/internal/deck"
)

// tutorPrompt is the instruction every turn carries. It is the one place the
// panel's teaching stance lives: explain around the card, never through it.
//
// The last rule is the load-bearing one. A drill whose answers are one question
// away stops being recall practice and becomes reading.
const tutorPrompt = `You are a tutor embedded in a Kubernetes flashcard drill. The learner is an
engineer with deep AWS ECS/Fargate experience and no prior Kubernetes.

- Anchor every explanation to its ECS or Fargate equivalent, and say plainly
  when there is none rather than inventing one.
- Prefer production realism. Name the resource limits, probes, RBAC, disruption
  budgets, or observability a real deployment would need, even when the card's
  happy path does not.
- The card below is on screen and may not yet be flipped. Do not state its
  answer and do not grade an attempt at it. If the learner asks the card's own
  question, explain the surrounding concept and what to reason from, so the
  recall is still theirs.
- Be brief: a few sentences or a short list. This is a study aid, not a chapter.`

// maxChatRequestBytes caps one question. Generous for prose, small enough that
// a request body cannot be used to grow the heap.
const maxChatRequestBytes = 32 << 10

// chatRequest is what the panel posts: the question, the card it is about, and
// the dials. Model and effort are validated against the provider's own Options,
// so no list of model names exists in this package.
type chatRequest struct {
	Card    string `json:"card"`
	Message string `json:"message"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
}

// chatRoutes registers the panel's endpoints, and only when a provider is
// configured: an unconfigured build serves 404 rather than an endpoint that
// fails on its first use.
func (s *Server) chatRoutes(mux *http.ServeMux) {
	if s.chat == nil {
		return
	}

	mux.Handle("POST /chat", localOnly(http.HandlerFunc(s.handleChat)))
	mux.Handle("POST /chat/reset", localOnly(http.HandlerFunc(s.handleChatReset)))
}

// localOnly restricts the chat routes to a request the panel on this machine
// actually made. These two routes are the only ones that spend a subscription
// and read the filesystem through the provider's tools.
//
// A loopback check on RemoteAddr is the obvious control and is not sufficient
// on its own, which is the subtle part: the user's own browser is on this
// machine, so a page on any website can POST to 127.0.0.1:8080 and arrive
// looking like a local caller. The browser will not hand the attacker the
// answer, but the subscription is spent and the tools have run regardless.
// Three more checks close that:
//
//   - Content-Type must be application/json. A cross-origin form post or
//     no-cors fetch may only send a safelisted content type, so anything that
//     arrives as JSON has been through a CORS preflight this server never
//     answers. This is the control that actually stops the attack.
//   - Origin and Sec-Fetch-Site, when the browser sends them, must say the
//     request came from here.
//   - Host must name loopback, which is what catches DNS rebinding: a domain
//     pointed at 127.0.0.1 is same-origin with itself and its requests really
//     do arrive on loopback, so only the Host header gives it away.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.RemoteAddr) || !isLoopbackHost(r.Host) {
			http.Error(w, "chat is available from this machine only", http.StatusForbidden)
			return
		}

		// Both headers are absent for a non-browser client such as curl, which
		// cannot be tricked into making a request on someone else's behalf.
		if origin := r.Header.Get("Origin"); origin != "" && !isLocalOrigin(origin) {
			http.Error(w, "cross-origin chat requests are refused", http.StatusForbidden)
			return
		}

		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			http.Error(w, "cross-origin chat requests are refused", http.StatusForbidden)
			return
		}

		if !isJSONRequest(r.Header.Get("Content-Type")) {
			http.Error(w, "chat requests must be application/json", http.StatusUnsupportedMediaType)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost reports whether a host — with or without a port, from
// RemoteAddr or from the Host header — names this machine.
//
// Only the conventional loopback name is accepted. Any other name could be an
// attacker's domain pointed at 127.0.0.1, which is the whole of a rebinding
// attack. It fails closed: anything unparseable is not loopback.
func isLoopbackHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(strings.Trim(host, "[]"))

	return ip != nil && ip.IsLoopback()
}

// isLocalOrigin reports whether an Origin header names this machine. The
// literal "null" of an opaque origin — a sandboxed iframe, a file:// page —
// parses to no host and is refused.
func isLocalOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}

	return isLoopbackHost(u.Host)
}

// isJSONRequest reports whether a Content-Type is application/json, ignoring
// any parameters such as charset.
func isJSONRequest(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)

	return err == nil && media == "application/json"
}

// handleChat runs one turn and relays the answer as it arrives.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	turn, card, ok := s.chatTurn(w, r)
	if !ok {
		return
	}

	start := s.now()

	// WriteTimeout covers the whole response, and a considered answer outlasts
	// the 30s that suits every other route. Clearing the deadline here beats
	// loosening it globally for handlers that should still be quick.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		slog.Warn("clearing the chat write deadline", "error", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	err := s.chat.Send(r.Context(), turn, func(delta string) error {
		if err := writeEvent(w, "delta", delta); err != nil {
			return err
		}

		// A writer that cannot flush still delivers the answer, just all at once
		// at the end. That is a degraded panel, not a failed turn — see
		// statusRecorder.Unwrap in cmd/flashcards for how it stops being one.
		if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return fmt.Errorf("flushing the answer: %w", err)
		}

		return nil
	})

	dur := s.now().Sub(start).Round(time.Millisecond)

	// The status line left long ago, so a failure is reported inside the stream.
	// The detail stays in the log: a provider's diagnostics are the operator's
	// business, not the browser's.
	if err != nil {
		slog.Error("chat turn failed",
			"card", card, "model", turn.Model, "effort", turn.Effort, "dur", dur, "error", err)

		_ = writeEvent(w, "error", "the chat provider failed — see the server log")
	} else {
		slog.Info("chat turn", "card", card, "model", turn.Model, "effort", turn.Effort, "dur", dur)

		_ = writeEvent(w, "done", "")
	}

	_ = rc.Flush() // the turn is over; a failed flush means the client already left
}

// handleChatReset starts a fresh conversation.
func (s *Server) handleChatReset(w http.ResponseWriter, _ *http.Request) {
	s.chat.Reset()
	w.WriteHeader(http.StatusNoContent)
}

// chatTurn validates a request into a turn, answering the client itself and
// reporting false when it cannot. It returns the card id alongside, for the log.
func (s *Server) chatTurn(w http.ResponseWriter, r *http.Request) (chat.Turn, string, bool) {
	var req chatRequest

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChatRequestBytes))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		http.Error(w, "the chat request could not be read", http.StatusBadRequest)
		return chat.Turn{}, "", false
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		http.Error(w, "ask a question", http.StatusBadRequest)
		return chat.Turn{}, "", false
	}

	card, found := s.lib.Get(req.Card)
	if !found {
		http.Error(w, "unknown card: "+req.Card, http.StatusBadRequest)
		return chat.Turn{}, "", false
	}

	// Both dials are checked against what the provider says it accepts, so
	// installing a provider with a different model list needs no change here.
	opts := s.chat.Options()

	model := req.Model
	if model == "" {
		model = opts.DefaultModel
	}

	if !slices.Contains(opts.Models, model) {
		http.Error(w, "unknown model: "+model, http.StatusBadRequest)
		return chat.Turn{}, "", false
	}

	if req.Effort != "" && !slices.Contains(opts.Efforts, req.Effort) {
		http.Error(w, "unknown effort: "+req.Effort, http.StatusBadRequest)
		return chat.Turn{}, "", false
	}

	return chat.Turn{
		Message:      message,
		CardContext:  s.cardContext(card),
		SystemPrompt: tutorPrompt,
		Model:        model,
		Effort:       req.Effort,
	}, card.ID, true
}

// cardContext describes what is on screen. It is assembled here because the
// shape of a card is flashcards domain knowledge, which internal/chat
// deliberately has none of — a provider only ever sees the finished block.
func (s *Server) cardContext(c deck.Card) string {
	header := "The card currently on screen, from the deck " + c.Deck
	if c.Module != "" {
		header += " (" + c.Module + ")"
	}

	lines := []string{
		header + ":",
		"",
		"Front: " + strings.TrimSpace(c.Q),
		"Back: " + strings.TrimSpace(c.A),
	}

	if ecs := strings.TrimSpace(c.ECS); ecs != "" {
		lines = append(lines, "ECS anchor: "+ecs)
	}

	if terms := s.requiredTerms(c); len(terms) > 0 {
		lines = append(lines, "Vocabulary it builds on: "+strings.Join(terms, ", "))
	}

	return strings.Join(lines, "\n")
}

// requiredTerms names the glossary terms a card depends on. A prerequisite that
// teaches no term falls back to its id, which is still more use than dropping it.
func (s *Server) requiredTerms(c deck.Card) []string {
	terms := make([]string, 0, len(c.Requires))

	for _, id := range c.Requires {
		if req, ok := s.lib.Get(id); ok && req.Term != "" {
			terms = append(terms, req.Term)
			continue
		}

		terms = append(terms, id)
	}

	return terms
}

// chatPanel is a provider's Options prepared for the template. Nil when chat is
// off, so `{{with .Chat}}` renders nothing at all.
type chatPanel struct {
	Models       []string
	DefaultModel string
	Efforts      []string
}

func (s *Server) chatPanel() *chatPanel {
	if s.chat == nil {
		return nil
	}

	opts := s.chat.Options()

	return &chatPanel{
		Models:       opts.Models,
		DefaultModel: opts.DefaultModel,
		Efforts:      opts.Efforts,
	}
}

// writeEvent frames one SSE event. The payload is JSON-encoded rather than
// written raw, so answer text containing a blank line cannot terminate its own
// event — which is exactly what markdown prose is full of.
func writeEvent(w io.Writer, name, payload string) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding the %s event: %w", name, err)
	}

	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, encoded); err != nil {
		return fmt.Errorf("writing the %s event: %w", name, err)
	}

	return nil
}
