# Vocabulary Tier and Prerequisite Gating — Technical Spec

**Status**: ⏳ PLANNED
**Last updated**: 2026-07-31

---

## Overview

Core Kubernetes vocabulary is used across the decks far more often than it is
taught. "API server" appears on 33 of 255 cards but is defined exactly once —
bolded inside `m0-control-plane-components`, a card that asks for four
components and their functions at the same time. The result is that concept
cards arrive before their terms are learnable, and the terms never stick.

This spec adds a **glossary tier** of atomic term cards and a `requires:`
prerequisite edge between cards, and gates card introduction on prerequisite
mastery. It changes the flashcards workload itself rather than a curriculum
module; it is a prerequisite for the decks being usable in M0 onward.

Gating becomes real for the glossary and M0, where the problem is sharpest.
The remaining modules keep their current behavior and are drained one at a
time as each is reached.

---

## Business Requirements

### Learning Objectives

- Every term used on three or more cards has exactly one card that teaches it
  in isolation, leading with its ECS anchor where one exists.
- A concept card is never introduced before the terms it depends on have been
  demonstrably retained, not merely seen.
- Drilling a module teaches that module's vocabulary as part of the module,
  rather than assuming it was acquired elsewhere.

### Production Bar

- Prerequisite edges are validated at load time; a dangling or cyclic reference
  fails startup rather than producing a silently unstudiable deck.
- The glossary/usage invariant is enforced by `make check`, not by review
  discipline, so the defect class cannot reappear.
- No filter can produce a queue that is empty because everything in it is
  locked — the blocking terms are always reachable from the same drill.

---

## Technical Requirements

### Kubernetes Resources

**Not applicable.** Decks are embedded at build time via
`flashcards/embed.go:14`; no manifest or cluster object changes.

### Go Components

```text
- `internal/deck/deck.go` — Card gains `Term`, `Aliases`, `Requires`.
  A non-empty `Term` marks a glossary card. Load-time validation: every
  `Requires` id resolves, terms and aliases are unique across the library,
  and the requires graph is acyclic (topological sort, error naming the cycle).
- `internal/deck/glossary.go` —
  `Library.WithPrerequisites(cards []Card) []Card` is append-only: the input
  keeps its order, and only prerequisites missing from it are prepended, in
  dependency order. A set that already contains its prerequisites comes back
  unchanged, so the unfiltered drill keeps authored order.
  `Library.Glossary()` returns the term cards indexed by term and alias, for
  the Phase 3 lint. Both pure and table-testable.
- `internal/review/review.go` — `Store.Next` gates the `fresh` branch on
  prerequisite mastery; `Store.Stats` gains `Locked int`; new
  `Store.Mastered(id string) bool` reports FSRS `Review` state, the
  WaniKani-"Guru" analogue already computed at `internal/review/review.go:268`.
  `Store.Cram` stays ungated on purpose — it is the night-before escape hatch,
  and a gate there would remove it.
- `internal/web/web.go` — the **drill** scope expands through
  `WithPrerequisites`: `Server.next`, `handleDrill`, `handleGrade`, `s.view`.
  The index page's per-deck rows keep the raw filter, because a deck row is an
  inventory of that deck and expanding it would count the same glossary cards
  in every row. The pulled-in term count travels in the template data beside
  `Stats`, not on `filter.Label` — that is a pure value method with no library
  access.
```

Prerequisite **satisfaction** is read from the store's own FSRS state, keyed by
card id, so it is evaluated globally and a filter can never make an unmastered
term look satisfied. Prerequisite **candidacy** is the expansion above: a
filtered drill pulls in the terms it depends on rather than dead-ending behind
them. Without that expansion, `?module=M0` selects no glossary cards at all —
glossary cards carry no `module:`, because terms like RBAC span M2 and M7 — and
every M0 card stays locked forever.

The persisted state format is unchanged. Gating is a read-time decision over
existing FSRS state, so `stateVersion` at `internal/review/review.go:36` stays
at 1 and no migration is needed.

### Observability

The service has no metrics endpoint yet, so the signals here are the ones a
reviewer and the build can see:

- Drill and index pages surface `Stats.Locked` alongside New/Due/Known.
- The drill scope label names how many prerequisite terms were pulled in.
- A cyclic or dangling `requires` fails `deck.Load`, which `run()` returns and
  `cmd/flashcards/main.go:35` exits non-zero on. Under Kubernetes that is
  `CrashLoopBackOff`, not a readiness failure — the deck is part of the image,
  so a broken graph must never reach a serving Pod.
- The glossary lint failure is a `make check` failure naming card id and term.

---

## Implementation Phases

### Phase 1: Glossary tier — ⏳ PLANNED

**Objective**: Every term used on 3+ cards has an atomic card that teaches it,
and those cards are the first thing the scheduler introduces.

Candidate terms are shortlisted mechanically — a bolded term appearing on three
or more cards, which yields 91 candidates — then curated by hand, because the
mechanical rule also catches English words ("one", "must", "set") and
case-collisions ("Kind" the object field vs "KIND" the tool). Expect roughly 60
real terms. Candidates rejected during curation are recorded with a reason in
`glossary-candidates.md` alongside this spec, so the judgment is reviewable.

Deck files are renumbered so the glossary sorts first. `deck.Load` globs and
`sort.Strings` the filenames (`internal/deck/deck.go:55-64`) and `Store.Next`
introduces `fresh[0]` in that order, so a `glossary.yaml` would sort *after*
every `NN-` deck and land behind all 255 existing cards. Renaming is safe:
review history is keyed on card id, not filename, so no scheduling state is
lost. Only `?deck=` URLs change.

**Tasks**:

- [ ] Add failing tests for `Term`/`Aliases` parsing and uniqueness —
      `internal/deck/deck_test.go`
- [ ] Add `Term` and `Aliases` to `Card`, with load-time uniqueness validation
- [ ] Curate the glossary term list from the 91 mechanical candidates; record
      rejections in `glossary-candidates.md`
- [ ] Author `decks/00-glossary.yaml` — one card per term, ECS anchor first
      where one exists, no term defined inside another term's card
- [ ] Renumber the existing decks to `01-`…`12-` so the glossary sorts first
- [ ] Split the four control-plane terms out of `m0-control-plane-components`
      into their own glossary cards
- [ ] Update the card count and deck listing in `README.md` (repo layout,
      "Drilling the vocabulary") and `docs/curriculum.md` ("The example
      workload")

**Deliverables**:

- `flashcards/decks/00-glossary.yaml` — the term cards
- `docs/specs/vocabulary-tier-and-prerequisites/glossary-candidates.md`
- `Term`/`Aliases` parsed and validated in `internal/deck/deck.go`
- `README.md` and `docs/curriculum.md` updated

### Phase 2: Prerequisites and gating — ⏳ PLANNED

**Objective**: Cards are introduced only once their prerequisites are mastered,
and a filtered drill teaches the prerequisites it depends on.

**Tasks**:

- [ ] Add failing tests for `Requires` resolution and cycle detection, using a
      multi-file `fstest.MapFS` for the cross-deck cases —
      `internal/deck/deck_test.go`
- [ ] Add `Requires` to `Card` with resolution and acyclicity validation
- [ ] Add a single-file cyclic-requires seed to the fuzz corpus —
      `internal/deck/fuzz_test.go`
- [ ] Add failing tests for `WithPrerequisites` transitive closure and ordering
      — `internal/deck/glossary_test.go`
- [ ] Implement `Library.WithPrerequisites`
- [ ] Add failing tests for gated introduction and `Stats.Locked` —
      `internal/review/review_test.go`
- [ ] Gate the `fresh` branch of `Store.Next`; add `Store.Mastered` and
      `Stats.Locked`
- [ ] Add failing tests that `?module=M0` on a fresh store introduces glossary
      cards, and that index deck rows stay unexpanded —
      `internal/web/web_test.go`
- [ ] Expand the drill scope through `WithPrerequisites` in `Server.next`,
      `handleDrill`, `handleGrade` and `s.view`, leaving the index rows on the
      raw filter; surface the pulled-in term count in the template data
- [ ] Add `requires:` to every M0 card, and rewrite
      `m0-control-plane-components` as an enumeration card that requires its
      four parts
- [ ] Update `docs/curriculum.md` M0 with the vocabulary-first study order

**Deliverables**:

- Gating implemented in `internal/review/review.go`
- Prerequisite expansion in `internal/deck/glossary.go` and `internal/web`
- M0 fully re-authored with prerequisite edges
- `docs/curriculum.md` updated

### Phase 3: Glossary lint — ⏳ PLANNED

**Objective**: A card that uses a glossary term without requiring it fails the
build.

Matching runs against the curated term and alias list only — never a naive
substring scan, which would flag `Service` inside `ServiceAccount`.

The lint ships with an explicit per-deck allowlist, seeded with every deck
except the glossary and M0, which Phase 2 leaves clean. Draining a deck's
allowlist entry is part of reaching that module; it is not this spec's work.

**Tasks**:

- [ ] Add the failing lint test over the embedded decks, with fixtures for a
      required term, an unrequired term, and a term inside a longer word —
      `internal/deck/glossary_test.go`
- [ ] Implement `Library.Glossary()` and the check: for each card, every
      glossary term or alias occurring in `q`/`a` is either required by that
      card or defined by it
- [ ] Add the per-deck allowlist, with the glossary and M0 excluded from it
- [ ] Add a `lint-decks` target running just this check —
      `flashcards/Makefile`
- [ ] Document the allowlist-draining step in `AGENTS.md` under the module
      workflow

**Deliverables**:

- `internal/deck/glossary_test.go` — the enforced invariant and its allowlist
- `lint-decks` target, reached by `make check` via `test`
- `AGENTS.md` updated

---

## Test-Driven Development Requirements

### TDD Plan

- Phase 1 tests: `internal/deck/deck_test.go` — term/alias parsing, duplicate
  term rejection, alias colliding with another card's term.
- Phase 2 tests: `internal/deck/deck_test.go` — dangling requires, self-require,
  and two- and three-node cycles spanning separate deck files.
  `internal/deck/glossary_test.go` — `WithPrerequisites` returns a set that
  already contains its prerequisites unchanged, and otherwise prepends only the
  missing ones in dependency order.
  `internal/review/review_test.go` — a card with an unmastered prerequisite is
  not returned by `Next`; it is returned once the prerequisite reaches `Review`;
  `Stats.Locked` counts it; `Cram` returns it regardless.
  `internal/web/web_test.go` — `?module=M0` on a fresh store introduces a
  glossary card rather than reporting an empty queue, and index deck row counts
  sum to the library total.
- Phase 3 tests: `internal/deck/glossary_test.go` — the lint over the real
  embedded decks, plus the three fixtures named in the Phase 3 tasks.
- Regression: `make check` in `flashcards/`.

### TDD Exceptions

- Deck content authoring (the term cards themselves) has no failing-test-first
  step. It is verified by the Phase 3 lint and by `make test`, which loads and
  validates every embedded deck.

---

## Technical Implementation Details

**To be filled in as the code is written.**

### Key Files

- `flashcards/decks/00-glossary.yaml` — the term cards
- `flashcards/internal/deck/deck.go` — schema and graph validation
- `flashcards/internal/deck/glossary.go` — prerequisite closure
- `flashcards/internal/review/review.go` — gated introduction
- `flashcards/internal/deck/glossary_test.go` — the enforced invariant

### Implementation Patterns

The card schema addition:

```yaml
- id: term-kube-apiserver
  term: kube-apiserver
  aliases: [API server, apiserver]
  q: |
    kube-apiserver
  a: |
    The cluster's only front door. Every read and write of cluster state goes
    through it, and it is the only component that talks to etcd.
  ecs: |
    The ECS control plane API you hit with `aws ecs ...` — except this one is a
    process you can read logs from, and everything in the cluster
    authenticates to it.

- id: m0-only-apiserver-touches-etcd
  requires: [term-kube-apiserver, term-etcd, term-kubelet]
```

### Important Notes

- Glossary content renders on the **answer** side only. On the question side it
  hands over the answer and turns retrieval into reading.
- ECS/Fargate contrast: the ECS anchor leads the term card. Attaching a new
  label to an existing schema is cheaper than memorizing a new definition, and
  that is the whole thesis of this program.

---

## Success Criteria

- [ ] `make lint-decks` fails on a card that uses a glossary term it does not
      require, and passes on the glossary and M0
- [ ] Every mechanical candidate without a glossary card appears in
      `glossary-candidates.md` with a reason
- [ ] `?module=M0` on a fresh review store introduces a glossary card, and the
      scope label names the number of terms pulled in
- [ ] `m0-only-apiserver-touches-etcd` is not introduced until its four
      prerequisite terms are in FSRS `Review` state
- [ ] No glossary or M0 card teaches more than one term;
      `m0-control-plane-components` requires its four parts rather than
      defining them
- [ ] Index deck row counts sum to the library total — the drill expansion does
      not leak into the inventory
- [ ] A deck with a cyclic `requires` spanning two files fails `deck.Load` with
      an error naming the cycle
- [ ] `make check` passes

---

## Troubleshooting Guide

**Not applicable** — no failures encountered yet. Entries get added as they are
hit.

---

## Future Enhancements

- Multiple-choice as the recognition-first introduction step for glossary
  terms, before free recall. Needs the Anki-style note/card split so one term
  carries two independently scheduled variants — which does require a
  `stateVersion` bump and a migration.
- Drain the per-deck lint allowlist for M1–M7 and the capstone, splitting the
  remaining 31 multi-term cards as each module is reached.

---

## Dependencies

### External Dependencies

- `github.com/open-spaced-repetition/go-fsrs/v3` — already vendored; the
  `Review` state it computes is the mastery signal gating reads.

### Internal Dependencies

- `flashcards/decks/*.yaml` — the corpus being restructured and renumbered
- `flashcards/internal/deck` and `internal/review` — the packages changed
- `docs/curriculum.md` M0 section — the objectives the re-authored M0 serves

---

## Risks and Mitigation

### Technical Risks

- **Risk**: The Phase 1 deck renumbering changes every `?deck=` URL and the
  index page links that build them.
- **Mitigation**: Links are generated from `Card.File` at
  `internal/web/web.go:223`, so they follow the rename automatically. Review
  history is keyed on card id and is unaffected. Only external bookmarks break.

- **Risk**: `WithPrerequisites` expands a narrow filter into a much larger set,
  so `?tag=storage` quietly becomes a general drill.
- **Mitigation**: The expansion adds only unmastered prerequisites and shrinks
  to nothing as terms are learned. The scope label states the count, and the
  `WithPrerequisites` test asserts the closure is exactly the transitive
  requires set, not the whole library.

### Learning Risks

- **Risk**: Gating shrinks the daily queue sharply at first, since most of M0
  locks behind unlearned vocabulary. This reads as regression.
- **Mitigation**: Expected and stated here. `Stats.Locked` makes the backlog
  visible rather than invisible, and the glossary tier is ungated so there is
  always something to study.

- **Risk**: The rendered vocabulary key becomes passive re-reading — the study
  mode that already failed to move retention.
- **Mitigation**: Answer side only, as links to the canonical term card rather
  than inline copies, so the term is still retrieved rather than supplied.

---

## Notes for AI Agents

Follow the workflow in `AGENTS.md`. In particular: tick each checkbox as that
item actually completes, update **Status** and **Last updated** in the same
change, write tests before production code, fill **Technical Implementation
Details** and **Troubleshooting** as the work happens rather than up front, and
fix this spec first if the work reveals it is wrong.
