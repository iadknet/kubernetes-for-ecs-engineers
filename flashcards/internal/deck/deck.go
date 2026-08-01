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

	// Distractors overrides the wrong options a glossary card is drilled against
	// while it is still being recognised, naming other glossary cards by id. It
	// exists because an automatic pick can be accidentally also-correct; when set
	// it replaces the automatic selection entirely.
	Distractors []string `json:"distractors"`

	// Checkpoint names the module this card examines, e.g. "M0". A non-empty
	// Checkpoint marks the card as a checkpoint card: a synthesis question that
	// is deliberately not atomic, sat as a pass/fail session rather than drilled.
	Checkpoint string `json:"checkpoint"`

	// Requires lists card ids that must be mastered before this card is
	// introduced. The graph it forms is validated acyclic at load time.
	//
	// A checkpoint card has an edge to every card in its module added at load
	// time, so there is one prerequisite mechanism rather than two.
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
	}

	// Expansion runs over the decks, before Cards is flattened out of them, so
	// the two views can never disagree about a card's prerequisites.
	if err := lib.expandCheckpoints(); err != nil {
		return nil, err
	}

	for _, d := range lib.Decks {
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

	if err := lib.validateDistractors(); err != nil {
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

// expandCheckpoints gives every checkpoint card a `requires:` edge to each
// ordinary card in the module it examines.
//
// Doing this at load time rather than at query time means there is exactly one
// prerequisite mechanism: the existing dangling-edge and cycle validation, the
// gate in Store.Next, and WithPrerequisites all keep working unchanged. Any
// edges the card declared itself are kept — they are how a checkpoint reaches
// across modules for a synthesis question.
//
// Checkpoint cards are never edge targets. A module's exam must not be gated on
// another exam, and two checkpoint cards in the same module requiring each other
// would be a load-time cycle.
func (l *Library) expandCheckpoints() error {
	byModule := map[string][]string{}

	for _, d := range l.Decks {
		for _, c := range d.Cards {
			if c.Checkpoint == "" && c.Module != "" {
				byModule[c.Module] = append(byModule[c.Module], c.ID)
			}
		}
	}

	for i := range l.Decks {
		for j := range l.Decks[i].Cards {
			c := &l.Decks[i].Cards[j]
			if c.Checkpoint == "" {
				continue
			}

			members, ok := byModule[c.Checkpoint]
			if !ok {
				return fmt.Errorf("checkpoint card %q names module %q, which has no cards", c.ID, c.Checkpoint)
			}

			declared := make(map[string]bool, len(c.Requires))
			for _, id := range c.Requires {
				declared[id] = true
			}

			for _, id := range members {
				if id != c.ID && !declared[id] {
					c.Requires = append(c.Requires, id)
				}
			}
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
		if err := validateCard(c, filename, i); err != nil {
			return Deck{}, err
		}

		c.Deck = d.Name
		c.Module = d.Module
		c.File = stem
		c.Tags = append(append([]string{}, c.Tags...), d.Tags...)
	}

	return d, nil
}

// validateCard checks one card's own fields. Anything needing the rest of the
// library — duplicate ids, terms, `requires:` and `distractors:` targets — is
// checked in Load once every deck is in.
func validateCard(c *Card, filename string, i int) error {
	if c.ID == "" {
		return fmt.Errorf("%s: card #%d has no id", filename, i+1)
	}

	if strings.TrimSpace(c.Q) == "" || strings.TrimSpace(c.A) == "" {
		return fmt.Errorf("%s: card %q needs both 'q' and 'a'", filename, c.ID)
	}

	if c.Term == "" && len(c.Aliases) > 0 {
		return fmt.Errorf("%s: card %q has 'aliases' but no 'term'", filename, c.ID)
	}

	// Only glossary cards are drilled as multiple choice, so distractors on
	// anything else would be authored effort that never renders.
	if c.Term == "" && len(c.Distractors) > 0 {
		return fmt.Errorf("%s: card %q has 'distractors' but no 'term'", filename, c.ID)
	}

	// A glossary card teaches one term atomically; a checkpoint card is the
	// opposite by design. Nothing can be both.
	if c.Term != "" && c.Checkpoint != "" {
		return fmt.Errorf("%s: card %q cannot be both a glossary card and a checkpoint card", filename, c.ID)
	}

	return nil
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

// Checkpoints returns the checkpoint cards examining the given module, in
// authored order. An empty result means the module has no exam yet, which is
// the expected state for every module not yet reached.
func (l *Library) Checkpoints(module string) []Card {
	var out []Card

	for _, c := range l.Cards {
		if c.Checkpoint != "" && strings.EqualFold(c.Checkpoint, module) {
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
