package deck_test

import (
	"strings"
	"testing"
	"testing/fstest"

	flashcards "github.com/iadk/k8s-flashcards"
	"github.com/iadk/k8s-flashcards/internal/deck"
)

// The embedded decks are the actual product here, so validating them is a real
// test rather than a formality: a typo'd id silently resets review history.
func TestEmbeddedDecksAreValid(t *testing.T) {
	t.Parallel()

	lib, err := deck.Load(flashcards.Decks, "decks")
	if err != nil {
		t.Fatalf("embedded decks failed to load: %v", err)
	}

	if len(lib.Cards) < 150 {
		t.Errorf("expected at least 150 cards, got %d", len(lib.Cards))
	}

	if len(lib.Decks) < 10 {
		t.Errorf("expected at least 10 decks, got %d", len(lib.Decks))
	}

	for _, c := range lib.Cards {
		if c.Deck == "" || c.File == "" {
			t.Errorf("card %q missing derived deck metadata", c.ID)
		}

		if len(c.Tags) == 0 {
			t.Errorf("card %q has no tags", c.ID)
		}
	}
}

// Every module in the curriculum should be drillable by name.
func TestEmbeddedDecksCoverCurriculumModules(t *testing.T) {
	t.Parallel()

	lib, err := deck.Load(flashcards.Decks, "decks")
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range []string{"M0", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "CAP"} {
		if got := lib.Select(deck.Filter{Module: m}); len(got) == 0 {
			t.Errorf("no cards for module %s", m)
		}
	}
}

func loadOne(t *testing.T, body string) (*deck.Library, error) {
	t.Helper()
	return deck.Load(fstest.MapFS{"decks/test.yaml": {Data: []byte(body)}}, "decks")
}

func TestParseRejectsMalformedDecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, yaml, wantErr string
	}{
		{
			name:    "no deck title",
			yaml:    "cards:\n  - id: a\n    q: q\n    a: a\n",
			wantErr: "missing top-level",
		},
		{
			name:    "no cards",
			yaml:    "deck: Empty\ncards: []\n",
			wantErr: "no cards",
		},
		{
			name:    "card without id",
			yaml:    "deck: D\ncards:\n  - q: q\n    a: a\n",
			wantErr: "has no id",
		},
		{
			name:    "card without answer",
			yaml:    "deck: D\ncards:\n  - id: a\n    q: q\n",
			wantErr: "needs both",
		},
		{
			name:    "not yaml at all",
			yaml:    "deck: [unclosed\n",
			wantErr: "parsing",
		},
		{
			name:    "two cards claim the same term",
			yaml:    "deck: D\ncards:\n  - id: a\n    term: Pod\n    q: q\n    a: a\n  - id: b\n    term: Pod\n    q: q\n    a: a\n",
			wantErr: `duplicate glossary term "Pod"`,
		},
		{
			name:    "an alias collides with another card's term",
			yaml:    "deck: D\ncards:\n  - id: a\n    term: Pod\n    q: q\n    a: a\n  - id: b\n    term: Deployment\n    aliases: [Pod]\n    q: q\n    a: a\n",
			wantErr: `duplicate glossary term "Pod"`,
		},
		{
			name:    "two cards share an alias",
			yaml:    "deck: D\ncards:\n  - id: a\n    term: Pod\n    aliases: [workload]\n    q: q\n    a: a\n  - id: b\n    term: Deployment\n    aliases: [workload]\n    q: q\n    a: a\n",
			wantErr: `duplicate glossary term "workload"`,
		},
		{
			name:    "an alias repeats its own term",
			yaml:    "deck: D\ncards:\n  - id: a\n    term: Pod\n    aliases: [Pod]\n    q: q\n    a: a\n",
			wantErr: `duplicate glossary term "Pod"`,
		},
		{
			name:    "aliases without a term",
			yaml:    "deck: D\ncards:\n  - id: a\n    aliases: [Pod]\n    q: q\n    a: a\n",
			wantErr: "has 'aliases' but no 'term'",
		},
		{
			name:    "a card is both a term and a checkpoint",
			yaml:    "deck: D\nmodule: M0\ncards:\n  - id: a\n    term: Pod\n    checkpoint: M0\n    q: q\n    a: a\n",
			wantErr: "cannot be both a glossary card and a checkpoint card",
		},
		{
			name:    "a checkpoint names a module with no cards",
			yaml:    "deck: D\nmodule: M0\ncards:\n  - id: a\n    checkpoint: M9\n    q: q\n    a: a\n",
			wantErr: `checkpoint card "a" names module "M9", which has no cards`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := loadOne(t, tt.yaml)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestDuplicateIDsAcrossDecksAreRejected(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"decks/a.yaml": {Data: []byte("deck: A\ncards:\n  - id: dupe\n    q: q\n    a: a\n")},
		"decks/b.yaml": {Data: []byte("deck: B\ncards:\n  - id: dupe\n    q: q\n    a: a\n")},
	}

	_, err := deck.Load(fsys, "decks")
	if err == nil || !strings.Contains(err.Error(), "duplicate card id") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestGlossaryTermsAndAliasesAreParsed(t *testing.T) {
	t.Parallel()

	lib, err := loadOne(t, "deck: D\ncards:\n  - id: term-a\n    term: kube-apiserver\n    aliases: [API server, apiserver]\n    q: q\n    a: a\n")
	if err != nil {
		t.Fatal(err)
	}

	c, _ := lib.Get("term-a")
	if c.Term != "kube-apiserver" {
		t.Errorf("Term = %q, want kube-apiserver", c.Term)
	}

	if len(c.Aliases) != 2 || c.Aliases[0] != "API server" {
		t.Errorf("Aliases = %v", c.Aliases)
	}
}

func TestRequiresResolvesAndRejectsBadGraphs(t *testing.T) {
	t.Parallel()

	card := func(id string, requires ...string) string {
		s := "  - id: " + id + "\n    q: q\n    a: a\n"
		if len(requires) > 0 {
			s += "    requires: [" + strings.Join(requires, ", ") + "]\n"
		}

		return s
	}

	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name:  "a resolvable edge across two decks",
			files: map[string]string{"a": card("term-x"), "b": card("concept", "term-x")},
		},
		{
			name:    "dangling requires",
			files:   map[string]string{"a": card("concept", "term-nope")},
			wantErr: `card "concept" requires unknown card "term-nope"`,
		},
		{
			name:    "self require",
			files:   map[string]string{"a": card("concept", "concept")},
			wantErr: "requires cycle",
		},
		{
			name:    "two-node cycle across files",
			files:   map[string]string{"a": card("one", "two"), "b": card("two", "one")},
			wantErr: "requires cycle",
		},
		{
			name:    "three-node cycle across files",
			files:   map[string]string{"a": card("one", "two"), "b": card("two", "three"), "c": card("three", "one")},
			wantErr: "requires cycle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fsys := fstest.MapFS{}
			for name, cards := range tt.files {
				fsys["decks/"+name+".yaml"] = &fstest.MapFile{Data: []byte("deck: " + name + "\ncards:\n" + cards)}
			}

			_, err := deck.Load(fsys, "decks")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected the graph to load, got %v", err)
				}

				return
			}

			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

// The cycle error has to name the cards in it, or a 60-card glossary graph is
// undebuggable.
func TestCycleErrorNamesTheCycle(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"decks/a.yaml": {Data: []byte("deck: A\ncards:\n  - id: one\n    q: q\n    a: a\n    requires: [two]\n")},
		"decks/b.yaml": {Data: []byte("deck: B\ncards:\n  - id: two\n    q: q\n    a: a\n    requires: [one]\n")},
	}

	_, err := deck.Load(fsys, "decks")
	if err == nil {
		t.Fatal("expected a cycle error")
	}

	for _, want := range []string{"one", "two"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("cycle error %q does not name card %q", err, want)
		}
	}
}

// A checkpoint card is the module's exam, so it depends on the whole module.
// Expanding that at load time keeps one rule for prerequisites: everything is a
// `requires:` edge, and the existing dangling/cycle validation covers it.
func TestCheckpointExpandsToModuleEdges(t *testing.T) {
	t.Parallel()

	const glossary = `
deck: Glossary
cards:
  - id: term-pod
    term: Pod
    q: q
    a: a
`

	const module = `
deck: Foundations
module: M0
cards:
  - id: m0-one
    q: q
    a: a
    requires: [term-pod]
  - id: m0-two
    q: q
    a: a
  - id: m0-checkpoint
    checkpoint: M0
    q: q
    a: a
    requires: [term-pod]
  - id: m0-checkpoint-two
    checkpoint: M0
    q: q
    a: a
`

	lib, err := deck.Load(fstest.MapFS{
		"decks/00-glossary.yaml":    {Data: []byte(glossary)},
		"decks/01-foundations.yaml": {Data: []byte(module)},
	}, "decks")
	if err != nil {
		t.Fatal(err)
	}

	c, ok := lib.Get(checkpointCard)
	if !ok {
		t.Fatal("no checkpoint card")
	}

	// Explicit edges are additive, and the two checkpoint cards must not require
	// each other or themselves — that would be a cycle, or an exam gated on an
	// exam.
	want := map[string]bool{"term-pod": true, moduleCard: true, "m0-two": true}

	got := map[string]bool{}
	for _, id := range c.Requires {
		got[id] = true
	}

	if len(got) != len(want) {
		t.Fatalf("requires = %v, want %v", c.Requires, want)
	}

	for id := range want {
		if !got[id] {
			t.Errorf("checkpoint card does not require %q; got %v", id, c.Requires)
		}
	}

	// Non-checkpoint cards keep exactly what they declared.
	if one, _ := lib.Get(moduleCard); len(one.Requires) != 1 || one.Requires[0] != "term-pod" {
		t.Errorf("m0-one.Requires = %v, want [term-pod]", one.Requires)
	}
}

// The Deck view of a card must not disagree with the Library view, or the index
// page and the drill would be reading two different graphs.
func TestCheckpointExpansionReachesTheDeckCards(t *testing.T) {
	t.Parallel()

	lib, err := loadOne(t, "deck: D\nmodule: M0\ncards:\n  - id: a\n    q: q\n    a: a\n  - id: cp\n    checkpoint: M0\n    q: q\n    a: a\n")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range lib.Decks[0].Cards {
		if c.ID != "cp" {
			continue
		}

		if len(c.Requires) != 1 || c.Requires[0] != "a" {
			t.Errorf("deck view of the checkpoint card has Requires = %v, want [a]", c.Requires)
		}
	}
}

func TestLoadEmptyDirIsAnError(t *testing.T) {
	t.Parallel()

	if _, err := deck.Load(fstest.MapFS{}, "decks"); err == nil {
		t.Fatal("expected an error for a directory with no decks")
	}
}

func TestDeckTagsMergeIntoCards(t *testing.T) {
	t.Parallel()

	lib, err := loadOne(t, "deck: D\ntags: [shared]\ncards:\n  - id: a\n    q: q\n    a: a\n    tags: [own]\n")
	if err != nil {
		t.Fatal(err)
	}

	c, _ := lib.Get("a")
	if len(c.Tags) != 2 || c.Tags[0] != "own" || c.Tags[1] != "shared" {
		t.Errorf("expected card and deck tags merged, got %v", c.Tags)
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"decks/01-one.yaml": {Data: []byte("deck: One\nmodule: M1\ncards:\n  - id: a\n    q: q\n    a: a\n    tags: [rbac]\n")},
		"decks/02-two.yaml": {Data: []byte("deck: Two\nmodule: M2\ncards:\n  - id: b\n    q: q\n    a: a\n    tags: [probes]\n")},
	}

	lib, err := deck.Load(fsys, "decks")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		filter deck.Filter
		want   int
	}{
		{"empty matches all", deck.Filter{}, 2},
		{"by module", deck.Filter{Module: "M1"}, 1},
		{"module is case insensitive", deck.Filter{Module: "m1"}, 1},
		{"by deck substring", deck.Filter{Deck: "02"}, 1},
		{"by tag", deck.Filter{Tag: "rbac"}, 1},
		{"combined miss", deck.Filter{Module: "M1", Tag: "probes"}, 0},
		{"unknown module", deck.Filter{Module: "M9"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := len(lib.Select(tt.filter)); got != tt.want {
				t.Errorf("got %d cards, want %d", got, tt.want)
			}
		})
	}
}
