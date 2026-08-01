# Module Checkpoints — Technical Spec

**Status**: ✅ COMPLETE
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

### Phase 1: Schema and validation — ✅ COMPLETE

**Objective**: Checkpoint cards parse, expand to prerequisite edges, and are
excluded from drill expansion.

Independent of `docs/specs/glossary-multiple-choice/SPEC.md`; either may land
first. Both touch `internal/deck/deck.go` and `glossary.go`, so implement them
on separate branches rather than interleaving.

**Tasks**:

- [x] Add failing tests for `Checkpoint` parsing, module-edge expansion,
      unknown-module and term-conflict rejection — `internal/deck/deck_test.go`
- [x] Implement `Checkpoint` on `Card` with load-time expansion and validation
- [x] Add a `term:`+`checkpoint:` conflict seed to the fuzz corpus —
      `internal/deck/fuzz_test.go`
- [x] Add failing tests that `WithPrerequisites` never emits or expands
      checkpoint cards — `internal/deck/glossary_test.go`
- [x] Exclude checkpoint cards in `WithPrerequisites`
- [x] Sync `README.md` deck/schema notes

**Deliverables**:

- `Checkpoint` parsed, expanded, and validated in `internal/deck/deck.go`
- Exclusion implemented in `internal/deck/glossary.go`
- `README.md` updated

### Phase 2: Sessions, state, and UI — ✅ COMPLETE

**Objective**: A checkpoint can be taken, passed, failed, retried next day, and
its status is visible.

**Tasks**:

- [x] Add failing tests for attempt state: clean sweep passes, any Again/Hard
      fails, next-day retake allowed, same-day blocked, passed cards enter FSRS
      rotation, checkpoint cards absent from `Next`/`Stats` before a pass —
      `internal/review/review_test.go`
- [x] Add failing test for the `stateVersion` 1→2 migration over a committed v1
      fixture — `internal/review/testdata/state_v1.json`, exercised from
      `internal/review/review_test.go`. The fixture is a real captured v1 file,
      not generated in-test; generating it would defeat the point.
- [x] Implement attempt state, session grading, and the migration
- [x] Add failing tests for `/checkpoint?module=M0`: offered only when
      prerequisites mastered, and the four index status states —
      `internal/web/web_test.go`
- [x] Implement the route and index status line
- [x] Sync `README.md` ("Drilling the vocabulary" / study-modes text) and
      `docs/curriculum.md` (checkpoint as each module's knowledge bar)

**Deliverables**:

- Session semantics and migration in `internal/review/review.go`
- `/checkpoint` route and index status in `internal/web/web.go`
- `README.md` and `docs/curriculum.md` updated

### Phase 3: M0 checkpoint content — ✅ COMPLETE

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

- [x] Author M0 checkpoint cards, one per non-practical M0 learning objective
      in `docs/curriculum.md`, with rubric lines — in the M0 deck file
- [x] Verify a full pass and a full failure against a pre-mastered fixture
      store, driving the session through the exported API —
      `internal/review/review_test.go`
- [x] Sync the card count in `README.md`

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

**Checkpoint edges are synthesized at load time, over the decks rather than
over the flattened card list.** `Load` now parses every deck, runs
`expandCheckpoints`, and only then flattens `Library.Cards` and builds `byID`.
Expanding after the flatten would leave `Library.Decks` and `Library.Cards`
holding different `Requires` for the same card — the index page reads one and
the drill the other. `TestCheckpointExpansionReachesTheDeckCards` pins that.

**A checkpoint card is never an edge target.** `expandCheckpoints` skips
checkpoint cards when collecting a module's members, so two exams in the same
module cannot require each other (a load-time cycle) and an exam is never gated
on another exam.

**`WithPrerequisites` treats checkpoint cards as opaque, not as removable.**
They are never *added* as prerequisites, and they are never *walked through* —
a checkpoint's edges span its whole module, so expanding one would turn any
scope containing it into a full-module drill. A checkpoint card present in the
input still comes back in the output: the function is append-only by contract,
and withholding an unpassed exam from the queue is the review layer's job, not
the deck layer's. That is the one place where "never emits checkpoint cards" is
read narrowly, and it is why `Store.withheld` exists.

**Grades are buffered for the duration of the attempt.** `GradeCheckpoint`
records into `checkpointAttempt.Grades` and only replays them into FSRS —
through the new `Store.schedule`, which skips the day counters — once the last
card completes a clean sweep. A failed attempt therefore leaves no FSRS trace
at all; without buffering, the exam would enter the drill queue through the
back door of a failure.

**`Store.schedule` is `Grade` minus the counters.** `Grade` now increments
`Reviews`/`NewSeen` and delegates the FSRS step to it. That split is what makes
"a checkpoint is an event, not a review day" true of the streak as well as of
`Stats`.

**Unpassed checkpoint cards leave `Stats` entirely, `Total` included.** The
spec asked for exclusion from New/Due; excluding them from `Total` too is what
keeps the buckets summing to `Total`, which the index deck-row test relies on.

**Availability is queryable, and the sentinel is the backstop.**
`CheckpointStatus` is what the route and the index render from.
`ErrCheckpointUnavailable` exists so a stale tab or a hand-crafted POST cannot
erase a recorded pass; the route maps it to `409 Conflict` rather than
depending on it for flow control.

**Days are compared as `YYYY-MM-DD` strings** via the existing `day()` helper,
end to end — persisted, compared, and rendered — so no timezone conversion can
slip in between recording an attempt and deciding whether the cool-down has
passed.

**The M0 checkpoint is two cards**, one per non-practical M0 objective: the
control-plane/worker split and kubeconfig contexts. The other two objectives
(install the toolchain, stand up a multi-node cluster) are practical and are
checked by M0's Done-when bar. Both cards carry explicit `requires:` for the
glossary terms they use, because the synthesized module edges only cover M0
cards and the glossary lint demands direct edges.

### Key Files

- `flashcards/internal/deck/deck.go` — `Card.Checkpoint`, `expandCheckpoints`,
  `Library.Checkpoints`
- `flashcards/internal/deck/glossary.go` — checkpoint exclusion in
  `WithPrerequisites`
- `flashcards/internal/review/review.go` — attempt state, session grading,
  `CheckpointStatus`, the v1→v2 migration
- `flashcards/internal/web/web.go` — `/checkpoint` routes and the index status
- `flashcards/internal/web/templates/checkpoint.html`,
  `checkpoint-fragments.html` — the sitting and the shared status line
- `flashcards/internal/review/testdata/state_v1.json` — the captured v1 fixture
- `flashcards/decks/01-foundations.yaml` — the M0 checkpoint cards

### Implementation Patterns

The schema addition:

```yaml
- id: m0-checkpoint-wrong-cluster
  checkpoint: M0
  q: |
    A command you have run a hundred times returns "no resources found"...
  a: |
    A context selects three things: cluster, user, namespace...

    *Rubric: full credit = all three parts of the context, the namespace named
    as the usual cause, and both commands.*
  requires: [term-kubeconfig, term-cluster, term-namespace]
```

`requires:` here is *additive*: `deck.Load` adds an edge to every ordinary M0
card on top of it.

### Important Notes

- The four UI states are an iota enum with `CheckpointUnknown` at 0, so an
  uninitialized status reads as "no exam yet" rather than as "locked".
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

- [x] `/checkpoint?module=M0` is locked on a fresh store and names the number
      of unmastered prerequisite cards
- [x] With all M0 cards mastered, a session grading every checkpoint card
      Good/Easy records a pass and shows the pass date on the index
- [x] A session with one Again records a failure; retake is refused the same
      day and offered the next
- [x] Passed checkpoint cards subsequently appear in the normal drill queue
- [x] Checkpoint cards never appear in a module drill, or in New/Due/Locked
      counts, before a pass
- [x] An existing v1 state file loads, migrates to v2, and loses no review
      history
- [x] `make check` passes

---

## Troubleshooting Guide

**`requires cycle` at startup after adding a checkpoint card.** The card names
a module it is itself an ordinary member of, or two checkpoint cards were made
to require each other by hand. Checkpoint cards are excluded from each other's
synthesized edges, so the cycle can only come from an explicit `requires:`
entry — remove it.

**`make lint-decks` fails on a new checkpoint card.** The synthesized edges
cover the module's cards, not the glossary. Every glossary term the question or
answer uses still needs its own explicit `requires:` entry, exactly as for an
ordinary card.

**`/checkpoint?module=M1` returns 404.** That module has no checkpoint cards
yet. Authoring them is part of reaching the module, not part of this spec; the
route 404s rather than offering an empty exam that would "pass" on the first
click.

**The checkpoint says locked but the module drill says nothing is due.** Locked
counts prerequisite cards that are not in FSRS `Review` state — scheduled but
not yet retained is still unmastered. The count shrinks as reviews come due
over the following days; there is nothing to do but wait and drill.

**A grade returns 409 Conflict.** No attempt is open: the tab is stale, the
checkpoint was already passed, or the cool-down is in force. Reload
`/checkpoint?module=…`, which renders the current status.

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
