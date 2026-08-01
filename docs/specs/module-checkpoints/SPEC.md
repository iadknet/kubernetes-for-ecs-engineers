# Module Checkpoints — Technical Spec

**Status**: ⏳ PLANNED
**Last updated**: 2026-07-31

---

## Overview

Each module gains a small set of **checkpoint cards** — synthesis questions that
are deliberately not atomic ("a Pod is Pending and describe shows no events —
walk the diagnosis") — and a checkpoint session mode with a clean-sweep pass
rule. Passing a module's checkpoint is the knowledge half of that module's
"done"; the practical half stays with the module's Done-when criteria in
`docs/curriculum.md`. This changes the flashcards workload, not a curriculum
module, and applies from M0 onward.

FSRS mastery (built by the vocabulary-tier spec) measures retention of cards as
written; it cannot tell whether the material transfers to a novel problem.
Checkpoints test transfer, with an objective pass event instead of a
self-assessed feeling of readiness.

---

## Business Requirements

### Learning Objectives

- Every module has an explicit, repeatable knowledge check whose questions
  require combining that module's cards, not recalling any single one.
- Passing is a crisp event — all checkpoint cards answered cleanly in one
  session — not a gradual drift into FSRS `Review` state.
- A failed checkpoint tells me which cards to restudy, and can be retaken the
  next day, not the same session.

### Production Bar

- Checkpoints are a soft gate: they flip visible module status but never lock
  cards. Study-ahead stays possible; `Cram` stays untouched.
- A checkpoint session is only offered once its module's cards are mastered,
  so a failed attempt means a transfer gap, not an exposure gap.
- Checkpoint attempt state survives restarts and is versioned with the rest of
  the review state.

---

## Technical Requirements

### Kubernetes Resources

**Not applicable.** Decks are embedded at build time; no manifest changes.

### Go Components

```text
- `internal/deck/deck.go` — Card gains `Checkpoint string` (module id, e.g.
  "M0"). A non-empty `Checkpoint` marks a checkpoint card and expands at load
  time to `requires:` edges on every card tagged with that module, so the
  existing dangling/cycle validation applies unchanged. Explicit `requires:`
  entries are additive, for cross-module synthesis. Validation: `Checkpoint`
  must name a module that has cards; checkpoint cards must not carry `term:`.
- `internal/deck/glossary.go` — `WithPrerequisites` excludes checkpoint cards
  from expansion targets and sources: a module drill never pulls the exam into
  the study queue.
- `internal/review/review.go` — checkpoint cards are excluded from `Next`'s
  fresh branch and from `Stats` New/Due counts; they are not part of the daily
  review economy until passed (see session semantics below). Excluding them
  from `New` also keeps them out of `Locked`, which is a subset of `New` rather
  than a separate bucket (`internal/review/review.go:280-284`) — an unpassed
  checkpoint surfaces only via the per-module status line, never in the drill
  counts. New attempt state: per module, last attempt date and result.
  `stateVersion` bumps 1 → 2 with a migration that adds the empty attempts map.
- `internal/web/web.go` — new `/checkpoint?module=M0` route, sibling to drill
  and cram. The index shows per-module checkpoint status: locked (prerequisites
  unmastered, with count), ready, failed (retry date), or passed (date).
```

### Checkpoint session semantics

- **Offered** when every prerequisite card is mastered (`Store.Mastered`, the
  FSRS `Review` state from the vocabulary-tier spec).
- **Pass**: every checkpoint card for the module graded Good or Easy in a
  single session. Any Again or Hard fails the attempt immediately; remaining
  cards are still shown (full diagnostic), but the result is recorded as
  failed.
- **Cool-down**: a failed checkpoint can be retaken the next day. Same-session
  retakes would test short-term memory of the answer just read. Availability is
  exposed as queryable state, not as an error return — the index already needs
  the attempt record to render its status line, so the route decides from that
  data rather than round-tripping a sentinel. Day comparison uses the existing
  `day()` helper (`internal/review/review.go:98`).
- **After a pass**, the module's checkpoint cards enter normal FSRS rotation as
  ordinary cards — the pass grades are their first reviews — so the synthesis
  keeps being retained.
- Checkpoint sessions do not count toward the daily streak or `Stats`; a
  checkpoint is an event, not a review day.

### Card authoring rules

- One checkpoint card per learning objective in the module's
  `docs/curriculum.md` section — a mechanical completeness rule, curated where
  an objective is purely practical.
- Answers must be enumerable and checkable ("name the four components and one
  function each"), never open-ended "explain X". The answer side ends with a
  one-line rubric stating what full credit requires. Convention only; no schema.
- Checkpoint cards follow the Phase 3 glossary lint like any other card; the
  implicit module-wide `requires` satisfies it for transitively required terms.

### Observability

- Index page: per-module checkpoint status line (locked/ready/failed/passed
  with the relevant count or date).
- A `Checkpoint` naming an unknown module, or combined with `term:`, fails
  `deck.Load` → non-zero exit → `CrashLoopBackOff`, same posture as the
  vocabulary-tier spec.
- State migration logs the version bump via the existing slog logger.

---

## Implementation Phases

### Phase 1: Schema and validation — ⏳ PLANNED

**Objective**: Checkpoint cards parse, expand to prerequisite edges, and are
excluded from drill expansion.

Independent of `docs/specs/glossary-multiple-choice/SPEC.md`; either may land
first. Both touch `internal/deck/deck.go` and `glossary.go`, so implement them
on separate branches rather than interleaving.

**Tasks**:

- [ ] Add failing tests for `Checkpoint` parsing, module-edge expansion,
      unknown-module and term-conflict rejection — `internal/deck/deck_test.go`
- [ ] Implement `Checkpoint` on `Card` with load-time expansion and validation
- [ ] Add a `term:`+`checkpoint:` conflict seed to the fuzz corpus —
      `internal/deck/fuzz_test.go`
- [ ] Add failing tests that `WithPrerequisites` never emits or expands
      checkpoint cards — `internal/deck/glossary_test.go`
- [ ] Exclude checkpoint cards in `WithPrerequisites`
- [ ] Sync `README.md` deck/schema notes

**Deliverables**:

- `Checkpoint` parsed, expanded, and validated in `internal/deck/deck.go`
- Exclusion implemented in `internal/deck/glossary.go`
- `README.md` updated

### Phase 2: Sessions, state, and UI — ⏳ PLANNED

**Objective**: A checkpoint can be taken, passed, failed, retried next day, and
its status is visible.

**Tasks**:

- [ ] Add failing tests for attempt state: clean sweep passes, any Again/Hard
      fails, next-day retake allowed, same-day blocked, passed cards enter FSRS
      rotation, checkpoint cards absent from `Next`/`Stats` before a pass —
      `internal/review/review_test.go`
- [ ] Add failing test for the `stateVersion` 1→2 migration over a committed v1
      fixture — `internal/review/testdata/state_v1.json`, exercised from
      `internal/review/review_test.go`. The fixture is a real captured v1 file,
      not generated in-test; generating it would defeat the point.
- [ ] Implement attempt state, session grading, and the migration
- [ ] Add failing tests for `/checkpoint?module=M0`: offered only when
      prerequisites mastered, and the four index status states —
      `internal/web/web_test.go`
- [ ] Implement the route and index status line
- [ ] Sync `README.md` ("Drilling the vocabulary" / study-modes text) and
      `docs/curriculum.md` (checkpoint as each module's knowledge bar)

**Deliverables**:

- Session semantics and migration in `internal/review/review.go`
- `/checkpoint` route and index status in `internal/web/web.go`
- `README.md` and `docs/curriculum.md` updated

### Phase 3: M0 checkpoint content — ⏳ PLANNED

**Objective**: M0 has a real checkpoint; later modules author theirs as each
module is reached (not this spec's work).

M0's curriculum objectives are largely practical (install the toolchain, stand
up a multi-node cluster), so its checkpoint is expected to be small — two or
three cards on the control-plane/worker split and kubeconfig contexts. That is
correct, not a shortfall: the practical objectives are checked by the module's
Done-when criteria, not by a card.

Checkpoint cards live in the M0 deck file alongside that module's other cards,
not in a separate checkpoint deck. Placement is behavior-neutral because
checkpoint cards are excluded from the order-sensitive fresh queue.

**Tasks**:

- [ ] Author M0 checkpoint cards, one per non-practical M0 learning objective
      in `docs/curriculum.md`, with rubric lines — in the M0 deck file
- [ ] Verify a full pass and a full failure against a pre-mastered fixture
      store, driving the session through the exported API —
      `internal/review/review_test.go`
- [ ] Sync the card count in `README.md`

**Deliverables**:

- M0 checkpoint cards in the deck files
- `README.md` count updated

---

## Test-Driven Development Requirements

### TDD Plan

- Phase 1 tests: `internal/deck/deck_test.go` — parsing, expansion to module
  edges, unknown module, `term:`+`checkpoint:` conflict.
  `internal/deck/glossary_test.go` — checkpoint exclusion from expansion.
- Phase 2 tests: `internal/review/review_test.go` — pass/fail/cool-down state
  machine, post-pass FSRS entry, pre-pass exclusion from `Next`/`Stats`
  (including `Locked`), and the 1→2 migration over
  `internal/review/testdata/state_v1.json`. `internal/web/web_test.go` — route
  gating and the four index states. Time-dependent cases inject `now` through
  the existing `now time.Time` parameters rather than sleeping.
- Regression: `make check` in `flashcards/`.

### TDD Exceptions

- Checkpoint card content (Phase 3) has no failing-test-first step; it is
  verified by deck load validation, the glossary lint, and the fixture-driven
  pass/fail session test.

---

## Technical Implementation Details

**To be filled in as the code is written.**

### Key Files

- `flashcards/internal/deck/deck.go` — schema and module-edge expansion
- `flashcards/internal/review/review.go` — attempt state and migration
- `flashcards/internal/web/web.go` — `/checkpoint` route and status

### Implementation Patterns

The schema addition:

```yaml
- id: m0-checkpoint-pending-pod
  checkpoint: M0
  q: |
    A Pod has been Pending for five minutes and `kubectl describe pod` shows
    no events. Walk the diagnosis: which component has not acted, and what are
    the two most likely reasons?
  a: |
    No events means the scheduler has not placed it...
    Rubric: full credit = names the scheduler, plus both no-fit-node and
    scheduler-not-running.
```

### Important Notes

- If the four UI states become an iota enum, the zero value must be a sentinel
  (`CheckpointUnknown` at 0), not `Locked` — an uninitialized status must not
  read as a deliberate one.
- `Store.Mastered` keeps its existing name (`internal/review/review.go:217`)
  rather than the `Is`-prefixed form the naming skill prefers for boolean
  predicates. Matching the shipped code beats renaming mid-implementation.
- ECS/Fargate contrast: ECS has no analogue of a knowledge gate; the nearest
  habit is "the deploy worked". Checkpoints exist precisely because "it ran"
  is not the bar this program holds.

---

## Success Criteria

Every criterion below is demonstrated against a fixture store with pre-mastered
M0 state and an injected clock — not by waiting out real FSRS intervals, which
would take days and tempt a faked pass.

- [ ] `/checkpoint?module=M0` is locked on a fresh store and names the number
      of unmastered prerequisite cards
- [ ] With all M0 cards mastered, a session grading every checkpoint card
      Good/Easy records a pass and shows the pass date on the index
- [ ] A session with one Again records a failure; retake is refused the same
      day and offered the next
- [ ] Passed checkpoint cards subsequently appear in the normal drill queue
- [ ] Checkpoint cards never appear in a module drill, or in New/Due/Locked
      counts, before a pass
- [ ] An existing v1 state file loads, migrates to v2, and loses no review
      history
- [ ] `make check` passes

---

## Troubleshooting Guide

**Not applicable** — no failures encountered yet.

---

## Future Enhancements

- Auto-flip the README roadmap status marker when both the checkpoint and the
  module's practical Done-when are met (currently a human step by design).

---

## Dependencies

### External Dependencies

- `github.com/open-spaced-repetition/go-fsrs/v3` — already vendored; supplies
  the mastery signal and the post-pass scheduling.

### Internal Dependencies

- `docs/specs/vocabulary-tier-and-prerequisites/SPEC.md` Phases 1–2 —
  `Store.Mastered`, `requires:` validation, and `WithPrerequisites` must exist
  first.
- `docs/curriculum.md` learning objectives — the authoring rule's source of
  truth per module.

---

## Risks and Mitigation

### Technical Risks

- **Risk**: The `stateVersion` bump corrupts or strands existing review
  history.
- **Mitigation**: Migration test over a real v1 fixture file; migration only
  adds the attempts map and never rewrites card entries.

### Learning Risks

- **Risk**: Self-graded pass/fail invites lenient grading under a clean-sweep
  rule.
- **Mitigation**: Enumerable answers with rubric lines make correctness
  countable, and a next-day retake is cheap enough that honest "Hard" grades
  are affordable.

- **Risk**: Checkpoint questions drift into single-card recall, making the
  gate redundant with FSRS mastery.
- **Mitigation**: The one-per-objective rule ties each card to a synthesis
  objective, and review against the rubric convention happens at authoring
  time in the spec-first workflow.

---

## Notes for AI Agents

Follow the workflow in `AGENTS.md`. In particular: tick each checkbox as that
item actually completes, update **Status** and **Last updated** in the same
change, write tests before production code, fill **Technical Implementation
Details** and **Troubleshooting** as the work happens rather than up front, and
fix this spec first if the work reveals it is wrong.
