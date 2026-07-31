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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	fsys := fstest.MapFS{
		"decks/a.yaml": {Data: []byte("deck: A\ncards:\n  - id: dupe\n    q: q\n    a: a\n")},
		"decks/b.yaml": {Data: []byte("deck: B\ncards:\n  - id: dupe\n    q: q\n    a: a\n")},
	}
	_, err := deck.Load(fsys, "decks")
	if err == nil || !strings.Contains(err.Error(), "duplicate card id") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestLoadEmptyDirIsAnError(t *testing.T) {
	if _, err := deck.Load(fstest.MapFS{}, "decks"); err == nil {
		t.Fatal("expected an error for a directory with no decks")
	}
}

func TestDeckTagsMergeIntoCards(t *testing.T) {
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
			if got := len(lib.Select(tt.filter)); got != tt.want {
				t.Errorf("got %d cards, want %d", got, tt.want)
			}
		})
	}
}
