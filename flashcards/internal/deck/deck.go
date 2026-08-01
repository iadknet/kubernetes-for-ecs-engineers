// Package deck loads and validates the flashcard decks.
package deck

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// Card is one flashcard. The ECS field carries the analogy to AWS ECS/Fargate
// and is deliberately empty for concepts that have no ECS equivalent.
//
// A non-empty Term marks the card as a glossary card: the one card in the
// library that teaches that term. Terms and aliases are matched case
// sensitively, so a card may claim "KIND" without also claiming "kind".
type Card struct {
	ID      string   `json:"id"`
	Q       string   `json:"q"`
	A       string   `json:"a"`
	ECS     string   `json:"ecs"`
	Tags    []string `json:"tags"`
	Term    string   `json:"term"`
	Aliases []string `json:"aliases"`

	// Requires lists card ids that must be mastered before this card is
	// introduced. The graph it forms is validated acyclic at load time.
	Requires []string `json:"requires"`

	// Derived from the deck the card was loaded from.
	Deck   string `json:"-"`
	Module string `json:"-"`
	File   string `json:"-"`
}

// Deck is one YAML file: a titled group of cards tied to a curriculum module.
type Deck struct {
	Name   string   `json:"deck"`
	Module string   `json:"module"`
	Tags   []string `json:"tags"`
	Cards  []Card   `json:"cards"`
}

// Library is every deck, loaded and indexed.
type Library struct {
	Decks []Deck
	Cards []Card

	byID   map[string]Card
	byTerm map[string]Card // every term and alias -> the card teaching it
}

// Filter narrows a set of cards. Zero value matches everything.
type Filter struct {
	Module string
	Deck   string
	Tag    string
}

// Load reads every *.yaml deck at the root of fsys.
func Load(fsys fs.FS, dir string) (*Library, error) {
	entries, err := fs.Glob(fsys, path.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("globbing decks: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no *.yaml decks found in %q", dir)
	}

	sort.Strings(entries)

	lib := &Library{byID: make(map[string]Card), byTerm: make(map[string]Card)}

	for _, name := range entries {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}

		d, err := parse(data, path.Base(name))
		if err != nil {
			return nil, err
		}

		lib.Decks = append(lib.Decks, d)
		lib.Cards = append(lib.Cards, d.Cards...)
	}

	for _, c := range lib.Cards {
		if _, dup := lib.byID[c.ID]; dup {
			return nil, fmt.Errorf("duplicate card id %q: ids must be unique across decks because review history is keyed on them", c.ID)
		}

		lib.byID[c.ID] = c
	}

	if err := lib.indexTerms(); err != nil {
		return nil, err
	}

	if err := lib.validateRequires(); err != nil {
		return nil, err
	}

	return lib, nil
}

// indexTerms builds the term -> card index, rejecting any term or alias claimed
// twice. Without that the mapping is ambiguous and no card definitively teaches
// the term.
func (l *Library) indexTerms() error {
	for _, c := range l.Cards {
		if c.Term == "" {
			continue
		}

		for _, t := range append([]string{c.Term}, c.Aliases...) {
			if prev, dup := l.byTerm[t]; dup {
				return fmt.Errorf("duplicate glossary term %q on cards %q and %q: exactly one card must teach each term", t, prev.ID, c.ID)
			}

			l.byTerm[t] = c
		}
	}

	return nil
}

// LoadDir reads decks from a directory on disk, for the DECKS_DIR override.
func LoadDir(dir string) (*Library, error) {
	return Load(os.DirFS(dir), ".")
}

func parse(data []byte, filename string) (Deck, error) {
	var d Deck
	if err := yaml.Unmarshal(data, &d); err != nil {
		return Deck{}, fmt.Errorf("parsing %s: %w", filename, err)
	}

	if d.Name == "" {
		return Deck{}, fmt.Errorf("%s: missing top-level 'deck:' title", filename)
	}

	if len(d.Cards) == 0 {
		return Deck{}, fmt.Errorf("%s: deck has no cards", filename)
	}

	stem := strings.TrimSuffix(filename, ".yaml")

	for i := range d.Cards {
		c := &d.Cards[i]
		if c.ID == "" {
			return Deck{}, fmt.Errorf("%s: card #%d has no id", filename, i+1)
		}

		if strings.TrimSpace(c.Q) == "" || strings.TrimSpace(c.A) == "" {
			return Deck{}, fmt.Errorf("%s: card %q needs both 'q' and 'a'", filename, c.ID)
		}

		if c.Term == "" && len(c.Aliases) > 0 {
			return Deck{}, fmt.Errorf("%s: card %q has 'aliases' but no 'term'", filename, c.ID)
		}

		c.Deck = d.Name
		c.Module = d.Module
		c.File = stem
		c.Tags = append(append([]string{}, c.Tags...), d.Tags...)
	}

	return d, nil
}

// Get returns a card by id.
func (l *Library) Get(id string) (Card, bool) {
	c, ok := l.byID[id]
	return c, ok
}

// Match reports whether a card satisfies the filter.
func (f Filter) Match(c Card) bool {
	if f.Module != "" && !strings.EqualFold(f.Module, c.Module) {
		return false
	}

	if f.Deck != "" && !strings.Contains(strings.ToLower(c.File), strings.ToLower(f.Deck)) {
		return false
	}

	if f.Tag != "" {
		var found bool

		for _, t := range c.Tags {
			if strings.EqualFold(t, f.Tag) {
				found = true
				break
			}
		}

		if !found {
			return false
		}
	}

	return true
}

// Select returns the cards matching the filter, in authored order.
func (l *Library) Select(f Filter) []Card {
	out := make([]Card, 0, len(l.Cards))
	for _, c := range l.Cards {
		if f.Match(c) {
			out = append(out, c)
		}
	}

	return out
}

// Modules returns the distinct module names, in first-seen order.
func (l *Library) Modules() []string {
	var out []string

	seen := map[string]bool{}
	for _, d := range l.Decks {
		if d.Module != "" && !seen[d.Module] {
			seen[d.Module] = true
			out = append(out, d.Module)
		}
	}

	return out
}

// Tags returns every distinct tag, sorted.
func (l *Library) Tags() []string {
	seen := map[string]bool{}

	for _, c := range l.Cards {
		for _, t := range c.Tags {
			seen[t] = true
		}
	}

	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}

	sort.Strings(out)

	return out
}
