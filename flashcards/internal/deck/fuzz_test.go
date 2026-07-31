package deck_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/iadk/k8s-flashcards/internal/deck"
)

// FuzzLoad drives the YAML deck parser with arbitrary input.
//
// The parser is the widest input surface in the binary: with DECKS_DIR set it
// reads files the operator supplies (a ConfigMap mount, from M2 onward), so it
// has to reject anything malformed rather than panic. A panic here is a crash
// loop in a Pod, not a bad error message.
//
// The contract being asserted is deliberately narrow — parse either fails, or
// returns a library whose invariants hold. Anything else is a bug:
//
//   - never panic, whatever the bytes
//   - on success, every card has a non-empty ID, question and answer
//   - on success, byID lookup agrees with the card list (review history is
//     keyed on card IDs, so a disagreement silently corrupts scheduling)
func FuzzLoad(f *testing.F) {
	seeds := []string{
		"deck: Valid\ncards:\n  - id: a\n    q: question\n    a: answer\n",
		"deck: Tagged\ntags: [core]\nmodule: m1\ncards:\n  - id: b\n    q: q\n    a: a\n    tags: [extra]\n",
		"cards:\n  - id: a\n    q: q\n    a: a\n", // no deck title
		"deck: Empty\ncards: []\n",                // no cards
		"deck: D\ncards:\n  - q: q\n    a: a\n",   // card without id
		"deck: D\ncards:\n  - id: a\n    q: q\n",  // card without answer
		"deck: [unclosed\n",                       // not YAML at all
		"",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, body string) {
		lib, err := deck.Load(fstest.MapFS{"decks/fuzz.yaml": {Data: []byte(body)}}, "decks")
		if err != nil {
			return // rejecting bad input is the expected outcome, not a failure
		}

		if lib == nil {
			t.Fatal("Load returned a nil library and a nil error")
		}

		for _, c := range lib.Cards {
			if c.ID == "" {
				t.Errorf("accepted a card with an empty id from %q", body)
			}

			if strings.TrimSpace(c.Q) == "" || strings.TrimSpace(c.A) == "" {
				t.Errorf("accepted card %q with an empty question or answer", c.ID)
			}

			got, ok := lib.Get(c.ID)
			if !ok {
				t.Errorf("card %q is in Cards but missing from byID lookup", c.ID)
				continue
			}

			if got.ID != c.ID {
				t.Errorf("byID lookup for %q returned card %q", c.ID, got.ID)
			}
		}
	})
}
