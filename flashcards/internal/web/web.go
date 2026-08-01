// Package web serves the flashcard UI: server-rendered HTML with htmx for the
// reveal/grade interactions. No build step, no npm, no CDN.
package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/iadk/k8s-flashcards/internal/deck"
	"github.com/iadk/k8s-flashcards/internal/review"
)

//go:embed templates/*.html static/*
var assets embed.FS

// Server holds everything the handlers need.
type Server struct {
	lib   *deck.Library
	store *review.Store
	tmpl  *template.Template
	md    goldmark.Markdown
	now   func() time.Time
}

// New builds a server over the given library and review store.
func New(lib *deck.Library, store *review.Store) (*Server, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"until": humanUntil,
		"add":   func(a, b int) int { return a + b },
	}).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}

	return &Server{
		lib:   lib,
		store: store,
		tmpl:  tmpl,
		md:    goldmark.New(goldmark.WithExtensions(extension.GFM)),
		now:   time.Now,
	}, nil
}

// Routes returns the mux. Method-based patterns need Go 1.22+.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /drill", s.handleDrill)
	mux.HandleFunc("POST /drill/{id}/reveal", s.handleReveal)
	mux.HandleFunc("POST /drill/{id}/grade", s.handleGrade)
	mux.HandleFunc("POST /drill/{id}/pick", s.handlePick)
	mux.HandleFunc("POST /drill/{id}/advance", s.handleAdvance)
	mux.HandleFunc("GET /checkpoint", s.handleCheckpoint)
	mux.HandleFunc("POST /checkpoint/{id}/reveal", s.handleCheckpointReveal)
	mux.HandleFunc("POST /checkpoint/{id}/grade", s.handleCheckpointGrade)
	mux.HandleFunc("GET /browse", s.handleBrowse)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.Handle("GET /static/", http.FileServerFS(assets))

	return mux
}

// filter is the drill scope, round-tripped through query params so every htmx
// fragment request keeps the same scope.
type filter struct {
	deck.Filter
	Cram bool
}

func filterFrom(r *http.Request) filter {
	q := r.URL.Query()

	return filter{
		Filter: deck.Filter{
			Module: q.Get("module"),
			Deck:   q.Get("deck"),
			Tag:    q.Get("tag"),
		},
		Cram: q.Get("cram") == "1",
	}
}

// Query renders the filter back into a query string for links and hx-post URLs.
func (f filter) Query() template.URL {
	v := url.Values{}
	if f.Module != "" {
		v.Set("module", f.Module)
	}

	if f.Deck != "" {
		v.Set("deck", f.Deck)
	}

	if f.Tag != "" {
		v.Set("tag", f.Tag)
	}

	if f.Cram {
		v.Set("cram", "1")
	}

	if len(v) == 0 {
		return ""
	}
	//nolint:gosec // G203: url.Values.Encode percent-encodes every key and value.
	return template.URL("?" + v.Encode())
}

// Label describes the current scope for the UI.
func (f filter) Label() string {
	var parts []string
	if f.Module != "" {
		parts = append(parts, f.Module)
	}

	if f.Deck != "" {
		parts = append(parts, f.Deck)
	}

	if f.Tag != "" {
		parts = append(parts, "#"+f.Tag)
	}

	if len(parts) == 0 {
		parts = append(parts, "all decks")
	}

	if f.Cram {
		parts = append(parts, "(cram)")
	}

	return strings.Join(parts, " · ")
}

// cardView is one card prepared for rendering.
type cardView struct {
	Card   deck.Card
	Q      template.HTML
	A      template.HTML
	ECS    template.HTML
	Filter filter
	Stats  review.Stats
	Terms  int // prerequisite terms pulled into this scope

	// Choices is the recognition rep's options, empty when the card is drilled
	// by free recall. Whether it is populated is the render mode.
	Choices []choiceView
	// Picked is the option chosen in a rep already answered, for the result
	// fragment. Empty while the question is still open.
	Picked string
}

// choiceView is one multiple-choice option: another glossary card's definition,
// standing in as a wrong answer unless it belongs to the card being drilled.
type choiceView struct {
	ID      string
	A       template.HTML
	Term    string
	Correct bool
}

// IsMultipleChoice reports whether this rep is graded by recognition.
func (v cardView) IsMultipleChoice() bool { return len(v.Choices) > 0 }

// WasCorrect reports whether the answered rep's pick was the right one.
func (v cardView) WasCorrect() bool { return v.Picked == v.Card.ID }

// choices builds the options for a glossary card still being recognised.
//
// The render mode is a pure function of FSRS state the store already holds —
// nothing new is persisted, so the state file's format is untouched. A card in
// Review has been retained and goes back to free recall, which stays the
// retention bar; recognition is only the on-ramp.
func (s *Server) choices(c deck.Card) []choiceView {
	if c.Term == "" || s.store.Mastered(c.ID) {
		return nil
	}

	opts := s.lib.Options(c, review.Day(s.now()))
	if len(opts) < 2 {
		return nil // a glossary too small to offer a real choice
	}

	out := make([]choiceView, 0, len(opts))
	for _, o := range opts {
		out = append(out, choiceView{
			ID:      o.ID,
			A:       s.markdown(o.A),
			Term:    o.Term,
			Correct: o.ID == c.ID,
		})
	}

	return out
}

// URL builds an action URL for this card, preserving the drill scope so htmx
// fragment requests stay inside the same filter.
func (v cardView) URL(action string) template.URL {
	//nolint:gosec // G203: the card ID goes through url.PathEscape; Query() is already escaped.
	return template.URL("/drill/" + url.PathEscape(v.Card.ID) + "/" + action + string(v.Filter.Query()))
}

// GradeURL is URL("grade") with the grade appended.
func (v cardView) GradeURL(g int) template.URL {
	//nolint:gosec // G203: g is an int rendered by strconv.Itoa; no attacker-controlled text.
	return v.URL("grade") + template.URL(v.sep()+"g="+strconv.Itoa(g))
}

// PickURL is URL("pick") naming the option chosen.
func (v cardView) PickURL(id string) template.URL {
	//nolint:gosec // G203: the option id goes through url.QueryEscape.
	return v.URL("pick") + template.URL(v.sep()+"p="+url.QueryEscape(id))
}

// sep is the separator that appends a parameter to an action URL, which already
// carries the drill scope as a query string when the scope is non-empty.
func (v cardView) sep() string {
	if v.Filter.Query() == "" {
		return "?"
	}

	return "&"
}

func (s *Server) view(c deck.Card, f filter) cardView {
	cards, terms := s.scope(f)

	return cardView{
		Card:    c,
		Q:       s.markdown(c.Q),
		A:       s.markdown(c.A),
		ECS:     s.markdown(c.ECS),
		Filter:  f,
		Stats:   s.store.Stats(cards, s.now()),
		Terms:   terms,
		Choices: s.choices(c),
	}
}

// scope is the set of cards a drill covers: the filter's own cards plus any
// prerequisite terms they depend on, and how many of those were pulled in.
//
// Cram is left unexpanded: it ignores gating entirely, so pulling in terms
// would only dilute the scope the reviewer actually asked for.
func (s *Server) scope(f filter) (cards []deck.Card, terms int) {
	selected := s.lib.Select(f.Filter)
	if f.Cram {
		return selected, 0
	}

	expanded := s.lib.WithPrerequisites(selected)

	return expanded, len(expanded) - len(selected)
}

func (s *Server) markdown(src string) template.HTML {
	if strings.TrimSpace(src) == "" {
		return ""
	}

	var buf bytes.Buffer
	if err := s.md.Convert([]byte(src), &buf); err != nil {
		// Fall back to escaped plain text rather than failing the request.
		//nolint:gosec // G203: src is run through template.HTMLEscapeString on this path.
		return template.HTML("<p>" + template.HTMLEscapeString(src) + "</p>")
	}
	// G203 is a real trust boundary here, not a false positive: goldmark passes raw
	// HTML in its input straight through, so this trusts deck content. That holds
	// because decks are authored in-repo and compiled into the binary via embed.FS.
	// If decks ever become user-supplied, this needs a sanitizer (e.g. bluemonday)
	// or goldmark configured with html.WithEscapedOutput.
	//nolint:gosec // G203: deck content is trusted, compile-time input. See comment above.
	return template.HTML(buf.String())
}

// next picks the card to show under the current filter.
func (s *Server) next(f filter, exclude string) (deck.Card, bool) {
	cards, _ := s.scope(f)
	if len(cards) == 0 {
		return deck.Card{}, false
	}

	if f.Cram {
		return s.store.Cram(cards, exclude)
	}

	return s.store.Next(cards, s.now())
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	now := s.now()

	type deckRow struct {
		Name, Module, File string
		Stats              review.Stats
	}

	var rows []deckRow

	for _, d := range s.lib.Decks {
		f := deck.Filter{Deck: d.Cards[0].File}
		rows = append(rows, deckRow{
			Name:   d.Name,
			Module: d.Module,
			File:   d.Cards[0].File,
			Stats:  s.store.Stats(s.lib.Select(f), now),
		})
	}

	// Only modules that actually have an exam get a status line. The rest are
	// authored as each module is reached, and an empty line would read as a gap
	// rather than as work not yet due.
	var checkpoints []review.CheckpointStatus

	for _, m := range s.lib.Modules() {
		if cards := s.lib.Checkpoints(m); len(cards) > 0 {
			checkpoints = append(checkpoints, s.store.CheckpointStatus(m, cards, now))
		}
	}

	s.render(w, "index.html", map[string]any{
		"Decks":       rows,
		"Total":       s.store.Stats(s.lib.Cards, now),
		"Streak":      s.store.Streak(now),
		"Tags":        s.lib.Tags(),
		"Checkpoints": checkpoints,
	})
}

func (s *Server) handleDrill(w http.ResponseWriter, r *http.Request) {
	f := filterFrom(r)
	card, ok := s.next(f, "")
	cards, terms := s.scope(f)

	data := map[string]any{
		"Filter":  f,
		"Stats":   s.store.Stats(cards, s.now()),
		"Terms":   terms,
		"HasCard": ok,
	}
	if ok {
		data["Card"] = s.view(card, f)
	}

	s.render(w, "drill.html", data)
}

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	card, ok := s.lib.Get(id)
	if !ok {
		http.Error(w, "unknown card: "+id, http.StatusNotFound)
		return
	}

	s.renderFragment(w, "card-back", s.view(card, filterFrom(r)))
}

func (s *Server) handleGrade(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.lib.Get(id); !ok {
		http.Error(w, "unknown card: "+id, http.StatusNotFound)
		return
	}

	g, err := strconv.Atoi(r.URL.Query().Get("g"))
	if err != nil || g < int(review.Again) || g > int(review.Easy) {
		http.Error(w, "grade must be 1 (again), 2 (hard), 3 (good) or 4 (easy)", http.StatusBadRequest)
		return
	}

	if _, err := s.store.Grade(id, review.Grade(g), s.now()); err != nil {
		// Persisting failed — say so rather than silently dropping the review, but
		// keep the detail (which carries the store's paths) in the logs.
		slog.Error("recording review", "card", id, "grade", g, "error", err)
		http.Error(w, "could not record the review", http.StatusInternalServerError)

		return
	}

	s.renderNext(w, filterFrom(r), id)
}

// handlePick grades a recognition rep. The pick is objectively right or wrong,
// so the rating follows from it rather than from self-assessment: correct is
// Good, wrong is Again.
//
// The result fragment is shown rather than advancing straight on, because a
// wrong pick is worth a moment on which option was right.
func (s *Server) handlePick(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	card, ok := s.lib.Get(id)
	if !ok {
		http.Error(w, "unknown card: "+id, http.StatusNotFound)
		return
	}

	// The options are recomputed rather than trusted from the request, so a pick
	// can only ever name something actually offered. They are captured before
	// grading because grading moves the card, and with it the render mode: the
	// answered rep keeps the options it was asked with so the result can mark the
	// right one.
	opts := s.choices(card)
	if len(opts) == 0 {
		http.Error(w, "card "+id+" is not drilled by recognition", http.StatusBadRequest)
		return
	}

	pick := r.URL.Query().Get("p")

	if !slices.ContainsFunc(opts, func(c choiceView) bool { return c.ID == pick }) {
		http.Error(w, "pick must name one of the options offered", http.StatusBadRequest)
		return
	}

	g := review.Again
	if pick == id {
		g = review.Good
	}

	if _, err := s.store.Grade(id, g, s.now()); err != nil {
		slog.Error("recording pick", "card", id, "pick", pick, "error", err)
		http.Error(w, "could not record the review", http.StatusInternalServerError)

		return
	}

	// Built after grading so the counts are the post-answer ones, then given back
	// the options the question was actually asked with.
	v := s.view(card, filterFrom(r))
	v.Choices, v.Picked = opts, pick

	s.renderFragment(w, "card-picked", v)
}

// handleAdvance moves on from an answered recognition rep. It is separate from
// grading because the pick was already recorded when the result was shown.
func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.lib.Get(id); !ok {
		http.Error(w, "unknown card: "+id, http.StatusNotFound)
		return
	}

	s.renderNext(w, filterFrom(r), id)
}

// renderNext serves whatever comes after an answered card: the next card's
// front, or the done fragment when the scope is exhausted. exclude is the card
// just answered, which only cram mode can otherwise hand straight back.
func (s *Server) renderNext(w http.ResponseWriter, f filter, exclude string) {
	card, ok := s.next(f, exclude)
	if !ok {
		cards, terms := s.scope(f)
		s.renderFragment(w, "card-done", map[string]any{
			"Filter": f,
			"Stats":  s.store.Stats(cards, s.now()),
			"Terms":  terms,
		})

		return
	}

	s.renderFragment(w, "card-front", s.view(card, f))
}

// checkpointView is one module's checkpoint prepared for rendering: its status
// line, and the card being sat if a session is in progress.
type checkpointView struct {
	Status  review.CheckpointStatus
	HasCard bool
	Card    deck.Card
	Q       template.HTML
	A       template.HTML
	ECS     template.HTML
	// Answered and Total track progress through the sitting, so a checkpoint
	// shows how much of the exam is left rather than an open-ended queue.
	Answered int
	Total    int
}

// URL builds an action URL for the card being sat, carrying the module so the
// htmx fragment requests stay inside the same attempt.
func (v checkpointView) URL(action string) template.URL {
	//nolint:gosec // G203: the card id goes through url.PathEscape and the module through url.Values.
	return template.URL("/checkpoint/" + url.PathEscape(v.Card.ID) + "/" + action +
		"?" + url.Values{"module": {v.Status.Module}}.Encode())
}

// GradeURL is URL("grade") with the grade appended.
func (v checkpointView) GradeURL(g int) template.URL {
	//nolint:gosec // G203: g is an int rendered by strconv.Itoa; no attacker-controlled text.
	return v.URL("grade") + template.URL("&g="+strconv.Itoa(g))
}

// checkpointCards resolves the module in the query to its exam. A module with no
// checkpoint cards is a 404 rather than an empty page: there is nothing to sit,
// and an empty exam that "passes" would be a silently useless gate.
func (s *Server) checkpointCards(w http.ResponseWriter, r *http.Request) (string, []deck.Card, bool) {
	module := r.URL.Query().Get("module")

	cards := s.lib.Checkpoints(module)
	if len(cards) == 0 {
		http.Error(w, "no checkpoint for module: "+module, http.StatusNotFound)
		return "", nil, false
	}

	return module, cards, true
}

func (s *Server) checkpointView(module string, cards []deck.Card) checkpointView {
	v := checkpointView{
		Status: s.store.CheckpointStatus(module, cards, s.now()),
		Total:  len(cards),
	}

	card, ok := s.store.NextCheckpoint(module, cards)
	if !ok {
		v.Answered = v.Total
		return v
	}

	v.HasCard = true
	v.Card = card
	v.Q = s.markdown(card.Q)
	v.A = s.markdown(card.A)
	v.ECS = s.markdown(card.ECS)

	for i, c := range cards {
		if c.ID == card.ID {
			v.Answered = i
			break
		}
	}

	return v
}

// handleCheckpoint opens a module's checkpoint: the sitting itself when it is
// offered, otherwise the status explaining why it is not.
func (s *Server) handleCheckpoint(w http.ResponseWriter, r *http.Request) {
	module, cards, ok := s.checkpointCards(w, r)
	if !ok {
		return
	}

	// Availability is queryable state, so the route decides from it rather than
	// provoking the store's sentinel.
	if s.store.CheckpointStatus(module, cards, s.now()).Available() {
		if err := s.store.StartCheckpoint(module, cards, s.now()); err != nil {
			slog.Error("starting checkpoint", "module", module, "error", err)
			http.Error(w, "could not start the checkpoint", http.StatusInternalServerError)

			return
		}
	}

	s.render(w, "checkpoint.html", s.checkpointView(module, cards))
}

func (s *Server) handleCheckpointReveal(w http.ResponseWriter, r *http.Request) {
	module, cards, ok := s.checkpointCards(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")

	for _, c := range cards {
		if c.ID != id {
			continue
		}

		v := s.checkpointView(module, cards)
		v.HasCard = true
		v.Card = c
		v.Q, v.A, v.ECS = s.markdown(c.Q), s.markdown(c.A), s.markdown(c.ECS)

		s.renderFragment(w, "checkpoint-back", v)

		return
	}

	http.Error(w, "unknown checkpoint card: "+id, http.StatusNotFound)
}

func (s *Server) handleCheckpointGrade(w http.ResponseWriter, r *http.Request) {
	module, cards, ok := s.checkpointCards(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")

	if !slices.ContainsFunc(cards, func(c deck.Card) bool { return c.ID == id }) {
		http.Error(w, "unknown checkpoint card: "+id, http.StatusNotFound)
		return
	}

	g, err := strconv.Atoi(r.URL.Query().Get("g"))
	if err != nil || g < int(review.Again) || g > int(review.Easy) {
		http.Error(w, "grade must be 1 (again), 2 (hard), 3 (good) or 4 (easy)", http.StatusBadRequest)
		return
	}

	switch err := s.store.GradeCheckpoint(module, id, review.Grade(g), cards, s.now()); {
	case errors.Is(err, review.ErrCheckpointUnavailable):
		// No attempt is open — a stale tab, or a hand-crafted request against a
		// locked or already-passed checkpoint.
		http.Error(w, "no checkpoint attempt is open for "+module, http.StatusConflict)

		return
	case err != nil:
		slog.Error("recording checkpoint answer", "module", module, "card", id, "error", err)
		http.Error(w, "could not record the answer", http.StatusInternalServerError)

		return
	}

	v := s.checkpointView(module, cards)
	if v.HasCard {
		s.renderFragment(w, "checkpoint-front", v)
		return
	}

	s.renderFragment(w, "checkpoint-result", v)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	f := filterFrom(r)
	cards := s.lib.Select(f.Filter)

	views := make([]cardView, 0, len(cards))
	for _, c := range cards {
		views = append(views, cardView{
			Card: c,
			Q:    s.markdown(c.Q),
			A:    s.markdown(c.A),
			ECS:  s.markdown(c.ECS),
		})
	}

	s.render(w, "browse.html", map[string]any{"Cards": views, "Filter": f})
}

// handleHealthz is liveness: is this process wedged? It deliberately checks
// nothing external — a liveness probe that fails on a dependency outage turns
// that outage into a cluster-wide restart storm.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintln(w, "ok") // a failed write means the client hung up; nothing to do
}

// handleReadyz is readiness: can this instance actually serve? Decks must have
// loaded and the review store must be writable, because an instance that can't
// record reviews shouldn't be in the Service's endpoints.
func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if len(s.lib.Cards) == 0 {
		http.Error(w, "no cards loaded", http.StatusServiceUnavailable)
		return
	}

	if err := s.store.Writable(); err != nil {
		// Warn, not Error: readiness failing is a degraded state the probe is meant
		// to catch, and it is polled every few seconds.
		slog.Warn("review store not writable", "error", err)
		http.Error(w, "review store not writable", http.StatusServiceUnavailable)

		return
	}

	_, _ = fmt.Fprintf(w, "ok: %d cards, %d decks\n", len(s.lib.Cards), len(s.lib.Decks))
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer // render first so a template error doesn't emit a half page
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		slog.Error("rendering template", "template", name, "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w) // headers are already sent; a write failure is the client's disconnect
}

func (s *Server) renderFragment(w http.ResponseWriter, name string, data any) {
	s.render(w, name, data)
}

// humanUntil renders a duration the way a study app should: "in 3 days".
func humanUntil(t time.Time) string {
	if t.IsZero() {
		return "—"
	}

	d := time.Until(t)
	switch {
	case d < 0:
		return "now"
	case d < time.Minute:
		return "in under a minute"
	case d < time.Hour:
		return fmt.Sprintf("in %d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("in %d hours", int(d.Hours()))
	default:
		return fmt.Sprintf("in %d days", int(d.Hours()/24))
	}
}
