package deck

import (
	"fmt"
	"hash/fnv"
	"maps"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
)

// validateRequires checks that every `requires:` id resolves to a real card and
// that the resulting graph is acyclic.
//
// Both failures are fatal at load time on purpose. A dangling edge locks its
// card forever, and a cycle locks every card in the cycle forever — a deck that
// loads but can never be studied is worse than one that refuses to start.
func (l *Library) validateRequires() error {
	for _, c := range l.Cards {
		for _, id := range c.Requires {
			if _, ok := l.byID[id]; !ok {
				return fmt.Errorf("card %q requires unknown card %q", c.ID, id)
			}
		}
	}

	// Iterative-friendly recursive DFS with a three-colour marking, so the error
	// can name the cycle rather than just reporting that one exists.
	const (
		unvisited = iota
		onStack
		done
	)

	state := make(map[string]int, len(l.Cards))

	var stack []string

	var visit func(id string) error

	visit = func(id string) error {
		switch state[id] {
		case done:
			return nil
		case onStack:
			from := 0

			for i, s := range stack {
				if s == id {
					from = i
					break
				}
			}

			return fmt.Errorf("requires cycle: %s", strings.Join(append(append([]string{}, stack[from:]...), id), " -> "))
		}

		state[id] = onStack
		stack = append(stack, id)

		for _, dep := range l.byID[id].Requires {
			if err := visit(dep); err != nil {
				return err
			}
		}

		stack = stack[:len(stack)-1]
		state[id] = done

		return nil
	}

	for _, c := range l.Cards {
		if err := visit(c.ID); err != nil {
			return err
		}
	}

	return nil
}

// WithPrerequisites returns cards with any prerequisites missing from it
// prepended, in dependency order. It is append-only: the input keeps its order
// and nothing is removed, so a set that already contains its prerequisites
// comes back unchanged and the unfiltered drill keeps authored order.
//
// This is candidacy, not satisfaction: it makes the terms a filtered drill
// depends on reachable from that same drill. Without it, `?module=M0` selects
// no glossary cards — terms carry no module, because a term like RBAC spans
// several — and every M0 card would stay locked forever.
//
// Checkpoint cards take no part in this. They are never added — a module drill
// must not pull the exam into the study queue — and they are never walked
// through, because a checkpoint's edges span its whole module and would turn any
// scope containing it into a full-module drill. The input is still returned
// untouched: filtering it would break the append-only contract, and the review
// layer is what withholds an unpassed checkpoint from the queue.
func (l *Library) WithPrerequisites(cards []Card) []Card {
	if len(cards) == 0 {
		return cards
	}

	inInput := make(map[string]bool, len(cards))
	for _, c := range cards {
		inInput[c.ID] = true
	}

	var (
		added   []Card
		visited = make(map[string]bool, len(cards))
	)

	var walk func(id string)

	walk = func(id string) {
		if visited[id] {
			return
		}

		visited[id] = true

		c := l.byID[id]
		if c.Checkpoint != "" {
			return
		}

		for _, dep := range c.Requires {
			walk(dep)
		}

		if !inInput[id] {
			added = append(added, c) // post-order: dependencies land before dependents
		}
	}

	for _, c := range cards {
		walk(c.ID)
	}

	if len(added) == 0 {
		return cards
	}

	return append(added, cards...)
}

// Glossary returns every glossary term and alias mapped to the card that
// teaches it. The keys are matched case sensitively, so "KIND" the tool and
// "kind" the object field are distinct entries.
func (l *Library) Glossary() map[string]Card {
	out := make(map[string]Card, len(l.byTerm))
	maps.Copy(out, l.byTerm)

	return out
}

// --- Multiple choice ---------------------------------------------------------

// distractorCount is how many wrong options sit alongside the right one. Four
// options is the usual recognition width: enough that a guess is worth about a
// quarter, few enough to read at a glance.
const distractorCount = 3

// validateDistractors checks that every `distractors:` id resolves to a real
// glossary card other than the card declaring it.
//
// Fatal at load time for the same reason a dangling `requires:` edge is: the
// alternative is a drill that quietly offers a short list of options, or offers
// a non-glossary card's prose as a one-line definition, and neither announces
// itself as a deck bug.
func (l *Library) validateDistractors() error {
	for _, c := range l.Cards {
		if len(c.Distractors) > distractorCount {
			return fmt.Errorf("card %q lists %d distractors, but a question offers %d",
				c.ID, len(c.Distractors), distractorCount)
		}

		for _, id := range c.Distractors {
			if id == c.ID {
				return fmt.Errorf("card %q lists itself as a distractor", c.ID)
			}

			other, ok := l.byID[id]
			if !ok {
				return fmt.Errorf("card %q lists unknown distractor %q", c.ID, id)
			}

			if other.Term == "" {
				return fmt.Errorf("card %q lists distractor %q, which is not a glossary card", c.ID, id)
			}
		}
	}

	return nil
}

// Distractors returns the wrong options a glossary card is drilled against: up
// to three other glossary cards, preferring confusable siblings. day is the
// YYYY-MM-DD the drill is happening on and, with the card id, is the whole
// seed — the same card yields the same picks all day and re-rolls tomorrow.
//
// A non-glossary card returns nil: concept and checkpoint answers are prose, and
// prose distractors would be hand-written falsehoods rehearsed as reading.
func (l *Library) Distractors(c Card, day string) []Card {
	if c.Term == "" {
		return nil
	}

	if len(c.Distractors) > 0 {
		out := make([]Card, 0, len(c.Distractors))
		for _, id := range c.Distractors {
			out = append(out, l.byID[id]) // resolved at load time
		}

		return out
	}

	cands := make([]Card, 0, len(l.Cards))

	for _, other := range l.Cards {
		if other.Term != "" && other.ID != c.ID {
			cands = append(cands, other)
		}
	}

	// Shuffle before ranking so that everything sharing a score is ordered by the
	// seed rather than by authored position, which would otherwise hand the same
	// deck neighbours to every term in it.
	shuffle(cands, seed(c.ID, day, "distractors"))
	scores := l.neighbourScores(c)

	sort.SliceStable(cands, func(i, j int) bool {
		return scores[cands[i].ID] > scores[cands[j].ID]
	})

	if len(cands) > distractorCount {
		cands = cands[:distractorCount]
	}

	return cands
}

// neighbourScores rates every other glossary card on how confusable it is with
// c. Being pulled in as a prerequisite by the same card outweighs a shared tag,
// because it is evidence one explanation uses both terms together, where a tag
// may be as broad as the deck.
func (l *Library) neighbourScores(c Card) map[string]int {
	const (
		perConsumer = 2
		perTag      = 1
	)

	// Only glossary cards are ever read back out, so scoring anything else is
	// work spent on entries no caller can reach.
	out := make(map[string]int, len(l.Cards))

	for _, other := range l.Cards {
		if other.ID == c.ID || other.Term == "" {
			continue
		}

		for _, tag := range other.Tags {
			if slices.Contains(c.Tags, tag) {
				out[other.ID] += perTag
			}
		}
	}

	for _, consumer := range l.Cards {
		if !slices.Contains(consumer.Requires, c.ID) {
			continue
		}

		for _, id := range consumer.Requires {
			if id != c.ID && l.byID[id].Term != "" {
				out[id] += perConsumer
			}
		}
	}

	return out
}

// Options returns the cards whose answers make up one multiple-choice question:
// the right one and its distractors, shuffled. Seeded like Distractors, so the
// right answer does not sit in a learnable position but does stay put while the
// question is on screen.
//
// A glossary smaller than four cards yields a shorter list rather than repeating
// a term, which keeps a tiny fixture deck drillable.
func (l *Library) Options(c Card, day string) []Card {
	if c.Term == "" {
		return nil
	}

	out := append([]Card{c}, l.Distractors(c, day)...)
	shuffle(out, seed(c.ID, day, "options"))

	return out
}

// seed derives a stable 64-bit seed from the card id, the day and a purpose, so
// the shuffle of the options and the choice of the distractors do not move in
// lockstep. Deriving it rather than reading a clock is what makes the selection
// a pure function, and keeps math/rand global state out of it.
func seed(id, day, purpose string) uint64 {
	h := fnv.New64a()
	for _, part := range []string{id, day, purpose} {
		_, _ = h.Write([]byte(part)) // hash.Hash never returns an error
		_, _ = h.Write([]byte{0})    // separator: "ab"+"c" must not collide with "a"+"bc"
	}

	return h.Sum64()
}

func shuffle(cards []Card, s uint64) {
	//nolint:gosec // G404: this is a deterministic shuffle of study options, not a secret.
	r := rand.New(rand.NewPCG(s, s>>32))
	r.Shuffle(len(cards), func(i, j int) { cards[i], cards[j] = cards[j], cards[i] })
}
