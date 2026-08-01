# Glossary Multiple-Choice Introduction — Technical Spec

**Status**: ✅ COMPLETE
**Last updated**: 2026-07-31

---

## Overview

A glossary card is introduced as recognition before it is drilled as recall:
while the card is new, the drill asks you to pick the term's definition from
confusable siblings; once retained, it reverts to free recall. This changes the
flashcards workload, not a curriculum module. It was originally drafted as a
phase of the vocabulary-tier spec, but that spec was already mid-implementation,
so it lands here as a follow-on.

---

## Business Requirements

### Learning Objectives

- A fresh glossary term's first reps build discrimination between confusable
  siblings (apiserver vs controller-manager vs scheduler) instead of failing a
  cold free-recall rep.
- Recognition reps are objectively graded — a pick is right or wrong, with no
  self-assessment involved.
- Free recall remains the retention bar: recognition is only the on-ramp.

### Production Bar

- No new persisted state: render mode derives from existing FSRS state, so
  `stateVersion` is untouched and no migration is needed. (It reads 2 as of
  `docs/specs/module-checkpoints/SPEC.md`, which landed first; this feature adds
  nothing to it.)
- Distractor selection is mechanical and validated at load time; a
  `distractors:` override fails startup if it names an unknown id, a
  non-glossary card, the card itself, or more distractors than a question
  offers — silently dropping the extras would hide the authoring mistake.
- Multiple choice applies to glossary cards only — concept answers are prose,
  and prose distractors would be hand-written falsehoods rehearsed as reading.

---

## Technical Requirements

### Kubernetes Resources

**Not applicable.** Decks are embedded at build time; no manifest changes.

### Go Components

```text
- `internal/deck/deck.go` — Card gains optional `Distractors []string`
  (card ids), validated to resolve to glossary cards.
- `internal/deck/glossary.go` — distractor selection: three other glossary
  cards' answer texts, preferring terms sharing a `requires:` consumer or tag,
  falling back to other glossary terms; the card's own answer excluded;
  `distractors:` overrides the automatic picks.
- `internal/web/web.go` — the drill renders a glossary card in FSRS
  `New`/`Learning` as multiple choice (four options, shuffled) and grades a
  correct pick `Good`, a wrong pick `Again`; a card in `Review` renders as
  free recall exactly as today.
```

Render mode is a pure function of FSRS state the store already holds. The known
trade-off: early reps measure recognition, so a freshly graduated card's first
recall rep may fail and reset — accepted as self-correcting.

Selection is deterministic: the shuffle and the fallback picks are seeded from
the card id plus the `day()` string (`internal/review/review.go:98`), so the
same card yields the same options all day and re-rolls tomorrow. That makes the
determinism test a pure input/output check with no clock injection and no
`math/rand` global state.

### Observability

- The multiple-choice rep shows which option was correct after grading, so a
  wrong pick is a taught moment, not just an `Again`.
- An unresolved `distractors:` id fails `deck.Load` → non-zero exit →
  `CrashLoopBackOff`, matching the vocabulary-tier spec's posture.

---

## Implementation Phases

### Phase 1: Distractor selection — ✅ COMPLETE

**Objective**: Every glossary card can produce three valid, useful distractors.

Independent of `docs/specs/module-checkpoints/SPEC.md`; either may land first.
Both touch `internal/deck/deck.go` and `glossary.go`, so implement them on
separate branches rather than interleaving.

**Tasks**:

- [x] Add failing tests: three picked, neighborhood preferred, own answer
      excluded, `distractors:` override honored, unknown override id rejected,
      same card id and day yields identical options while a different day
      re-rolls — `internal/deck/glossary_test.go`
- [x] Parse and validate `Distractors` in `internal/deck/deck.go`
- [x] Implement selection in `internal/deck/glossary.go`
- [x] Sync the schema notes in `README.md`

**Deliverables**:

- Selection implemented and validated in `internal/deck`
- `README.md` updated

### Phase 2: Drill rendering and grading — ✅ COMPLETE

**Objective**: New glossary cards drill as multiple choice; retained ones as
free recall.

**Tasks**:

- [x] Add failing tests: a `New`/`Learning` glossary card renders four shuffled
      options and grades correct→Good, wrong→Again; a `Review` card renders
      free recall; concept cards are unaffected — `internal/web/web_test.go`
- [x] Implement the rendering and pick-grading path in `internal/web`
- [x] Sync `README.md` "Drilling the vocabulary" with the recognition-first
      behavior

**Deliverables**:

- Multiple-choice path in `internal/web`
- `README.md` updated

---

## Test-Driven Development Requirements

### TDD Plan

- Phase 1 tests: `internal/deck/glossary_test.go` — distractor count,
  neighborhood preference, own-answer exclusion, override, rejection, and
  seed determinism (same id + day identical, different day re-rolls).
- Phase 2 tests: `internal/web/web_test.go` — render mode follows FSRS state;
  pick grading maps correct→Good, wrong→Again; concept cards untouched.
- Regression: `make check` in `flashcards/`.

### TDD Exceptions

- None.

---

## Technical Implementation Details

### Key Files

- `flashcards/internal/deck/deck.go` — `Card.Distractors`, and the per-card
  field checks split out of `parse` into `validateCard`
- `flashcards/internal/deck/glossary.go` — `validateDistractors`, `Distractors`,
  `Options`, and the seeded `shuffle`
- `flashcards/internal/review/review.go` — `Day`, the exported form of the
  existing `day` key
- `flashcards/internal/web/web.go` — `Server.choices`, `handlePick`,
  `handleAdvance`, `renderNext`
- `flashcards/internal/web/templates/fragments.html` — the `card-front` branch
  and the `card-picked` result fragment

### Implementation Patterns

The optional override:

```yaml
- id: term-kube-apiserver
  term: kube-apiserver
  distractors: [term-kube-controller-manager, term-kube-scheduler, term-etcd]
```

**Neighborhood as a score, not a set.** The first cut treated "shares a tag" as
a boolean neighborhood, which is inert on the real decks: `parse` appends the
deck's tags to every card, so all 72 glossary cards share `glossary` and the
whole tier reads as one neighborhood. Candidates are instead scored — 2 per card
that pulls both terms in through `requires:`, 1 per shared tag — and the top
three win. A broad deck tag then adds a point to everyone and changes no
ordering, while a specific shared tag or a shared consumer still does.

**Seeding.** `seed(id, day, purpose)` is FNV-64a over the three parts,
NUL-separated, feeding a `math/rand/v2` PCG source. The purpose salt keeps the
choice of distractors and the shuffle of the options from moving together. No
clock is injected anywhere and no global `rand` state is touched, so the
determinism test is a plain input/output check.

**Two routes, not one.** `POST /drill/{id}/pick` grades and renders the result;
`POST /drill/{id}/advance` moves on. Splitting them is what lets a wrong pick
show which option was right instead of vanishing into the next card. The options
are recomputed server-side on `pick` and the submitted id must be one of them, so
a hand-crafted request cannot grade a card against something never offered.

### Important Notes

- A retained card that lapses to FSRS `Relearning` drops back to recognition
  until it returns to `Review`. The render mode is exactly "is this card in
  `Review`", the same `Store.Mastered` predicate prerequisite gating uses — one
  definition of retained rather than two. Sending a forgotten term back to the
  on-ramp is the intended behavior, not a side effect worth special-casing.
- A glossary with fewer than four cards yields fewer options rather than
  repeating a term, and fewer than two falls back to free recall. This only
  bites fixture decks, but a drill that offers one option is worse than a
  reveal button.

- ECS/Fargate contrast: K8s vocabulary is a set of confusable siblings in a
  way ECS's flat surface never was — discrimination between them *is* the
  learning task recognition reps train.

---

## Success Criteria

- [x] A `New` glossary card renders as multiple choice with three distractors
      drawn from other glossary answers; the same card in `Review` renders as
      free recall — with the state file unchanged in format
- [x] A `distractors:` entry naming a missing id fails `deck.Load` with an
      error naming the card and the id
- [x] Concept cards — and checkpoint cards, if that spec has landed — render
      exactly as before
- [x] `make check` passes

---

## Troubleshooting Guide

**A glossary card renders as free recall when it should be recognition.**
Either the card is already in FSRS `Review` — which is correct, that is the
graduation — or the library has fewer than two glossary cards, which is only
reachable in a fixture deck. Check `Store.Mastered(id)` before suspecting the
selection.

**Every term offers the same distractors.** The neighborhood score has
collapsed: something is giving every candidate the same total, so the seeded
shuffle is the only thing ordering them. Deck-level tags do this on purpose and
harmlessly; a per-card tag applied to the whole glossary would not.

**`deck.Load` fails naming a distractor.** The id is missing, names a
non-glossary card, or is the card itself. This is deliberate — the process exits
non-zero and the Pod goes `CrashLoopBackOff` rather than serving a drill with
three options, matching the vocabulary-tier spec's posture.

**A pick returns 400.** The submitted `p` is not one of the options the server
computes for that card today. Options re-roll at the day boundary, so a tab left
open overnight will do this; reloading the drill fixes it.

---

## Future Enhancements

- Anki-style note/card split — independently scheduled recognition and recall
  variants per term — if the single-schedule on-ramp proves too coarse.
  Requires a `stateVersion` bump and migration.

---

## Dependencies

### External Dependencies

- `github.com/open-spaced-repetition/go-fsrs/v3` — already vendored; supplies
  the state the render mode derives from.

### Internal Dependencies

- `docs/specs/vocabulary-tier-and-prerequisites/SPEC.md` Phases 1–2 — Phase 1
  supplies the glossary cards (`Term`, the glossary deck) and Phase 2 the
  `Requires` graph the neighborhood preference reads. Without Phase 2,
  selection has no neighborhood signal and degrades to the tag-and-random
  fallback.
- `flashcards/internal/deck` and `internal/web` — the packages changed.

---

## Risks and Mitigation

### Technical Risks

- **Risk**: Automatic distractors are accidentally also-correct (several
  definitions plausibly describe the API server), rewarding a "wrong" right
  answer with `Again`.
- **Mitigation**: The `distractors:` override, applied during glossary
  authoring review where the auto-picks are eyeballed per term.

### Learning Risks

- **Risk**: Recognition inflates early FSRS stability, so graduated cards fail
  their first recall rep more often than a recall-trained card would.
- **Mitigation**: Accepted by design — the failed rep resets the card into
  recall-mode learning, and the cost is one extra rep, not lost history.

---

## Notes for AI Agents

Follow the workflow in `AGENTS.md`. In particular: tick each checkbox as that
item actually completes, update **Status** and **Last updated** in the same
change, write tests before production code, fill **Technical Implementation
Details** and **Troubleshooting** as the work happens rather than up front, and
fix this spec first if the work reveals it is wrong.
