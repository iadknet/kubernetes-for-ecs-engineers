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
	"go.abhg.dev/goldmark/mermaid"

	"github.com/iadk/k8s-flashcards/internal/chat"
	"github.com/iadk/k8s-flashcards/internal/deck"
	"github.com/iadk/k8s-flashcards/internal/review"
)

//go:embed templates/*.html static/*
var assets embed.FS

// Config carries the optional collaborators a server is built with. Its zero
// value is the shipped configuration: no chat, no extra routes, the real clock.
type Config struct {
	// Chat enables the drill view's chat panel. Nil — the default — leaves the
	// chat routes unregistered and the panel markup unrendered, so a build with
	// the feature compiled in is byte-for-byte the build without it.
	Chat chat.Provider

	// Now is the clock the server reads. Nil — the default — is time.Now. It
	// exists so time-dependent behavior can be asserted rather than observed to
	// be probably right; the process never sets it.
	Now func() time.Time
}

// Server holds everything the handlers need.
type Server struct {
	lib   *deck.Library
	store *review.Store
	tmpl  *template.Template
	md    goldmark.Markdown
	now   func() time.Time
	chat  chat.Provider
}

// New builds a server over the given library and review store.
func New(lib *deck.Library, store *review.Store, cfg Config) (*Server, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"until": humanUntil,
		"add":   func(a, b int) int { return a + b },
	}).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}

	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &Server{
		lib:   lib,
		store: store,
		tmpl:  tmpl,
		md:    goldmark.New(goldmark.WithExtensions(extension.GFM, mermaidExtender())),
		now:   now,
		chat:  cfg.Chat,
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
	s.chatRoutes(mux)

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

// drillScope is one request's view of a drill: the cards in scope, the stats
// over them, and the instant all of it was computed at. It is built once per
// request and passed down, so the card, the stats bar, the scheduler and the
// multiple-choice seed all agree about which cards are in scope and what day it
// is.
//
// Built and read on one handler goroutine and never stored on Server, so it
// needs no synchronization. It is passed by value despite being ~190 bytes —
// over the usual threshold for a pointer — because a per-request snapshot that
// no callee may mutate is worth the copy at the handful of call sites per
// request.
type drillScope struct {
	filter filter
	now    time.Time
	cards  []deck.Card
	terms  int
	stats  review.Stats
}

// newDrillScope computes the set of cards a drill covers — the filter's own
// cards plus any prerequisite terms they depend on, and how many of those were
// pulled in — along with the stats over the result.
//
// Cram is left unexpanded: it ignores gating entirely, so pulling in terms would
// only dilute the scope the reviewer actually asked for.
//
// now is a parameter rather than a clock read, because a handler that records an
// answer needs the instant before it needs the snapshot: it reads the clock,
// writes, and only then snapshots, so the counts it renders include the answer
// just given.
//
// stats is computed eagerly rather than lazily because every drill fragment
// renders the drill-stats bar, so it is always needed exactly once. handleBrowse
// needs neither stats nor expansion and calls lib.Select directly.
func (s *Server) newDrillScope(f filter, now time.Time) drillScope {
	selected := s.lib.Select(f.Filter)
	sc := drillScope{filter: f, now: now, cards: selected}

	if !f.Cram {
		expanded := s.lib.WithPrerequisites(selected)
		sc.cards, sc.terms = expanded, len(expanded)-len(selected)
	}

	sc.stats = s.store.Stats(sc.cards, now)

	return sc
}

// cardView is one card prepared for rendering.
//
// Stats and Terms belong to the drill scope, not to the card: every card
// fragment re-renders the drill-stats bar as an htmx out-of-band swap, so the
// scope's counts have to travel with the card. They are copied off the request's
// drillScope rather than recomputed per card.
type cardView struct {
	Card deck.Card
	cardText
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

// cardText is one card's prose, markdown-converted and safe to emit. Every view
// that renders a card embeds it rather than listing the fields itself: the
// alternative is one assembly site per surface, where adding a field compiles
// fine on the surfaces that forgot it and silently renders nothing there.
type cardText struct {
	Q             template.HTML
	A             template.HTML
	ECS           template.HTML
	ECSComparison *ecsComparisonView
}

func (s *Server) cardText(c deck.Card) cardText {
	return cardText{
		Q:             s.markdown(c.Q),
		A:             s.markdown(c.A),
		ECS:           s.markdown(c.ECS),
		ECSComparison: s.ecsComparisonView(c.ECSComparison),
	}
}

type ecsComparisonView struct {
	Scenario       template.HTML
	ECSJSON        string
	KubernetesYAML string
	Alignments     []ecsAlignmentView
	Consequence    template.HTML
	Omissions      template.HTML
}

type ecsAlignmentView struct {
	ECS        template.HTML
	Kubernetes template.HTML
	Mapping    string
	Caveat     template.HTML
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
//
// now is passed rather than read here so the option seed and the review counters
// share one definition of "today" — which is what review.Day exists for.
func (s *Server) choices(c deck.Card, now time.Time) []choiceView {
	if c.Term == "" || s.store.Mastered(c.ID) {
		return nil
	}

	opts := s.lib.Options(c, review.Day(now))
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

func (s *Server) view(c deck.Card, sc drillScope) cardView {
	return cardView{
		Card:     c,
		cardText: s.cardText(c),
		Filter:   sc.filter,
		Stats:    sc.stats,
		Terms:    sc.terms,
		Choices:  s.choices(c, sc.now),
	}
}

func (s *Server) ecsComparisonView(comparison *deck.ECSComparison) *ecsComparisonView {
	if comparison == nil {
		return nil
	}

	alignments := make([]ecsAlignmentView, 0, len(comparison.Alignments))
	for _, alignment := range comparison.Alignments {
		alignments = append(alignments, ecsAlignmentView{
			ECS:        s.markdown(alignment.ECS),
			Kubernetes: s.markdown(alignment.Kubernetes),
			Mapping:    alignment.Mapping,
			Caveat:     s.markdown(alignment.Caveat),
		})
	}

	return &ecsComparisonView{
		Scenario:       s.markdown(comparison.Scenario),
		ECSJSON:        comparison.ECSJSON,
		KubernetesYAML: comparison.KubernetesYAML,
		Alignments:     alignments,
		Consequence:    s.markdown(comparison.Consequence),
		Omissions:      s.markdown(comparison.Omissions),
	}
}

// mermaidExtender renders ```mermaid blocks as <pre class="mermaid"> for the
// MermaidJS runtime in layout.html to draw.
//
// Every field set here is load-bearing, and the defaults are all wrong for this
// app — do not simplify this to &mermaid.Extender{}:
//
//   - RenderMode defaults to RenderModeAuto, which switches to server-side
//     rendering whenever `mmdc` happens to be on $PATH. A developer with
//     mermaid-cli installed would silently get different output, produced by
//     shelling out to Node.
//   - NoScript suppresses the extension's own <script> pair. markdown() runs
//     once per card *field*, so without it a card with a diagram re-injects a
//     CDN script tag and a startOnLoad initialize call into every htmx
//     fragment. layout.html carries the one copy of each instead.
//
// Theme is deliberately unset: the extender documents it as ignored in client
// mode, so layout.html picks the theme at page load.
func mermaidExtender() *mermaid.Extender {
	return &mermaid.Extender{
		RenderMode: mermaid.RenderModeClient,
		NoScript:   true,
	}
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
	// G203 is a real trust boundary, and what makes it safe is not the caller: it
	// is that goldmark's HTML renderer is left in its default non-Unsafe mode, so
	// raw HTML in *any* input is replaced with an omitted-HTML comment and a
	// javascript: or data: URL renders with an empty href. That has to hold for
	// every input, because two callers arrive here and only one is trusted —
	// cards are authored in-repo and compiled in via embed.FS, while the chat
	// panel renders model output (chat.go, handleChat).
	//
	// So: do not add html.WithUnsafe() to the goldmark instance above. It would
	// read as a card-authoring convenience and would quietly make an answer from
	// the chat provider executable. TestChatRenderedAnswerOmitsRawHTML is what
	// turns that edit into a failing build.
	//nolint:gosec // G203: goldmark is non-Unsafe, so raw HTML is omitted. See comment above.
	return template.HTML(buf.String())
}

// next picks the card to show from the scope already computed.
func (s *Server) next(sc drillScope, exclude string) (deck.Card, bool) {
	if len(sc.cards) == 0 {
		return deck.Card{}, false
	}

	if sc.filter.Cram {
		return s.store.Cram(sc.cards, exclude)
	}

	return s.store.Next(sc.cards, sc.now)
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
	sc := s.newDrillScope(filterFrom(r), s.now())

	card, ok := s.next(sc, "")

	data := map[string]any{
		"Filter":  sc.filter,
		"Stats":   sc.stats,
		"Terms":   sc.terms,
		"HasCard": ok,
		"Chat":    s.chatPanel(),
	}
	if ok {
		data["Card"] = s.view(card, sc)
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

	s.renderFragment(w, "card-back", s.view(card, s.newDrillScope(filterFrom(r), s.now())))
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

	// The clock is read before the write and the snapshot built after it, so the
	// counts the next fragment renders include the review just recorded.
	now := s.now()

	if _, err := s.store.Grade(id, review.Grade(g), now); err != nil {
		// Persisting failed — say so rather than silently dropping the review, but
		// keep the detail (which carries the store's paths) in the logs.
		slog.Error("recording review", "card", id, "grade", g, "error", err)
		http.Error(w, "could not record the review", http.StatusInternalServerError)

		return
	}

	s.renderNext(w, s.newDrillScope(filterFrom(r), now), id)
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

	// One clock read for the whole request, taken first. A second read across
	// midnight would re-seed the options between offering them and marking them.
	now := s.now()

	// The options are recomputed rather than trusted from the request, so a pick
	// can only ever name something actually offered. They are captured before
	// grading because grading moves the card, and with it the render mode: the
	// answered rep keeps the options it was asked with so the result can mark the
	// right one.
	opts := s.choices(card, now)
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

	if _, err := s.store.Grade(id, g, now); err != nil {
		slog.Error("recording pick", "card", id, "pick", pick, "error", err)
		http.Error(w, "could not record the review", http.StatusInternalServerError)

		return
	}

	// Built after grading so the counts are the post-answer ones, then given back
	// the options the question was actually asked with.
	v := s.view(card, s.newDrillScope(filterFrom(r), now))
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

	s.renderNext(w, s.newDrillScope(filterFrom(r), s.now()), id)
}

// renderNext serves whatever comes after an answered card: the next card's
// front, or the done fragment when the scope is exhausted. exclude is the card
// just answered, which only cram mode can otherwise hand straight back.
func (s *Server) renderNext(w http.ResponseWriter, sc drillScope, exclude string) {
	card, ok := s.next(sc, exclude)
	if !ok {
		s.renderFragment(w, "card-done", map[string]any{
			"Filter": sc.filter,
			"Stats":  sc.stats,
			"Terms":  sc.terms,
		})

		return
	}

	s.renderFragment(w, "card-front", s.view(card, sc))
}

// checkpointView is one module's checkpoint prepared for rendering: its status
// line, and the card being sat if a session is in progress.
//
// The status is handed in rather than read here, because every handler that
// builds this view has already read one — to decide whether to offer a sitting,
// or to see what grading the last answer did to the attempt. Status.Module is
// the module identity for the whole view: it names the deck's canonical
// spelling, and every action URL and store lookup below is built from it.
type checkpointView struct {
	Status  review.CheckpointStatus
	HasCard bool
	Card    deck.Card
	cardText
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

// checkpointCards resolves the module in the query to its exam, returning the
// deck's own spelling of the module rather than the query's. A module with no
// checkpoint cards is a 404 rather than an empty page: there is nothing to sit,
// and an empty exam that "passes" would be a silently useless gate.
//
// The card's own Checkpoint field is the deck's spelling; the query's is
// whatever the caller typed, and Checkpoints matched it case-insensitively.
// Returning the former is what keeps the store's attempt key, the index and
// the drill's withholding gate all naming the same module.
func (s *Server) checkpointCards(w http.ResponseWriter, r *http.Request) (string, []deck.Card, bool) {
	module := r.URL.Query().Get("module")

	cards := s.lib.Checkpoints(module)
	if len(cards) == 0 {
		http.Error(w, "no checkpoint for module: "+module, http.StatusNotFound)
		return "", nil, false
	}

	return cards[0].Checkpoint, cards, true
}

func (s *Server) checkpointView(cards []deck.Card, status review.CheckpointStatus) checkpointView {
	v := checkpointView{
		Status: status,
		Total:  len(cards),
	}

	card, ok := s.store.NextCheckpoint(status.Module, cards)
	if !ok {
		v.Answered = v.Total
		return v
	}

	v.HasCard = true
	v.Card = card
	v.cardText = s.cardText(card)

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

	now := s.now()

	// Availability is queryable state, so the route decides from it rather than
	// provoking the store's sentinel.
	status := s.store.CheckpointStatus(module, cards, now)
	if status.Available() {
		if err := s.store.StartCheckpoint(module, cards, now); err != nil {
			slog.Error("starting checkpoint", "module", module, "error", err)
			http.Error(w, "could not start the checkpoint", http.StatusInternalServerError)

			return
		}
	}

	// Starting an attempt cannot change the status: StartCheckpoint writes one
	// with Done and Failed false, which still reads as ready. So the status read
	// above is the one the page renders, and no second read is needed.
	s.render(w, "checkpoint.html", s.checkpointView(cards, status))
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

		v := s.checkpointView(cards, s.store.CheckpointStatus(module, cards, s.now()))
		v.HasCard = true
		v.Card = c
		v.cardText = s.cardText(c)

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

	now := s.now()

	switch err := s.store.GradeCheckpoint(module, id, review.Grade(g), cards, now); {
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

	// Read after grading, not before: the answer just recorded may have failed the
	// attempt or completed it, and the status line has to say so.
	v := s.checkpointView(cards, s.store.CheckpointStatus(module, cards, now))
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
		views = append(views, cardView{Card: c, cardText: s.cardText(c)})
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
