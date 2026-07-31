// Package web serves the flashcard UI: server-rendered HTML with htmx for the
// reveal/grade interactions. No build step, no npm, no CDN.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
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
}

// URL builds an action URL for this card, preserving the drill scope so htmx
// fragment requests stay inside the same filter.
func (v cardView) URL(action string) template.URL {
	//nolint:gosec // G203: the card ID goes through url.PathEscape; Query() is already escaped.
	return template.URL("/drill/" + url.PathEscape(v.Card.ID) + "/" + action + string(v.Filter.Query()))
}

// GradeURL is URL("grade") with the grade appended.
func (v cardView) GradeURL(g int) template.URL {
	sep := "?"
	if v.Filter.Query() != "" {
		sep = "&"
	}
	//nolint:gosec // G203: g is an int rendered by strconv.Itoa; no attacker-controlled text.
	return v.URL("grade") + template.URL(sep+"g="+strconv.Itoa(g))
}

func (s *Server) view(c deck.Card, f filter) cardView {
	return cardView{
		Card:   c,
		Q:      s.markdown(c.Q),
		A:      s.markdown(c.A),
		ECS:    s.markdown(c.ECS),
		Filter: f,
		Stats:  s.store.Stats(s.lib.Select(f.Filter), s.now()),
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
	cards := s.lib.Select(f.Filter)
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

	s.render(w, "index.html", map[string]any{
		"Decks":  rows,
		"Total":  s.store.Stats(s.lib.Cards, now),
		"Streak": s.store.Streak(now),
		"Tags":   s.lib.Tags(),
	})
}

func (s *Server) handleDrill(w http.ResponseWriter, r *http.Request) {
	f := filterFrom(r)
	card, ok := s.next(f, "")
	data := map[string]any{
		"Filter":  f,
		"Stats":   s.store.Stats(s.lib.Select(f.Filter), s.now()),
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
		// Persisting failed — say so rather than silently dropping the review.
		http.Error(w, "could not record review: "+err.Error(), http.StatusInternalServerError)
		return
	}

	f := filterFrom(r)
	card, ok := s.next(f, id)
	if !ok {
		s.renderFragment(w, "card-done", map[string]any{
			"Filter": f,
			"Stats":  s.store.Stats(s.lib.Select(f.Filter), s.now()),
		})
		return
	}
	s.renderFragment(w, "card-front", s.view(card, f))
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
		http.Error(w, "review store not writable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	_, _ = fmt.Fprintf(w, "ok: %d cards, %d decks\n", len(s.lib.Cards), len(s.lib.Decks))
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer // render first so a template error doesn't emit a half page
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
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
