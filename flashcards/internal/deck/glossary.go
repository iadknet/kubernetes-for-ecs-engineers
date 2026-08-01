package deck

import (
	"fmt"
	"maps"
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
