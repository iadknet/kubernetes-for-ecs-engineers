# Per-Request Scope Snapshot — Technical Spec

**Status**: ✅ COMPLETE
**Last updated**: 2026-08-02

---

## Overview

Every drill request recomputes its own scope three times and reads the wall
clock four times. `handleDrill` (`flashcards/internal/web/web.go:390`) calls
`s.scope(f)` via `next`, again directly, and a third time inside `view`; two of
those also run `store.Stats`, and one of the two results is always discarded.
The checkpoint path has the same shape, computing `CheckpointStatus` twice per
request.

This spec makes each request compute its scope once, from one clock reading,
and pass that snapshot down. It is a refactor of `internal/web` with no change
to any template, URL, or rendered output. It belongs to no curriculum module —
it is maintenance on the workload the modules deploy, taken now because the
duplicated scope is what makes `cardView` depend on the whole library just to
render one card.

---

## Business Requirements

### Learning Objectives

- I can explain why a request handler reads the clock once and passes the
  instant down, rather than calling `time.Now()` wherever a timestamp is
  needed. Two reads in one request are two different times, and any value
  keyed on the day — a review counter, a streak, a daily seed — can disagree
  with itself.
- I can recognise a view model that has been handed a dependency it does not
  own. `cardView` carries `Stats` and `Terms` only so an htmx out-of-band swap
  can repaint the stats bar, and that forced every card to know the entire
  drill scope. This spec makes that coupling cheap rather than removing it;
  removing it means rendering the `drill-stats` fragment independently, which
  is deferred to **Future Enhancements** because it changes the templates and
  this spec does not.

### Production Bar

- One request yields one consistent view of time. No rendered page mixes
  counts from two different days.
- Scope selection, prerequisite expansion, and stats are each computed at most
  once per request.
- Behavior is unchanged **except** within a request that crosses a day
  boundary. Today such a request renders counts from one day beside a card
  selected against another; that self-inconsistency is the defect being fixed,
  and it is the only output this spec changes. Everything else — markup,
  status codes, scheduling — is byte-identical, and the existing black-box
  suite is the proof.
- The clock is injectable, so time-dependent behavior is asserted rather than
  observed to be probably right.

---

## Technical Requirements

### Kubernetes Resources

**Not applicable** — a Go refactor inside `internal/web`.

### Go Components

```text
- internal/web/web.go — Config gains an optional Now clock; drillScope is
  added as the per-request snapshot; scope/view/choices/next/renderNext and
  the checkpoint helpers take it instead of rebuilding their own.
- internal/web/web_test.go — clock helpers, the new assertions, and the two
  existing checkpoint tests moved onto a fixed clock.
```

**The clock seam.** `Config` already documents itself as "the optional
collaborators a server is built with", which is what a clock is:

```go
type Config struct {
	Chat chat.Provider

	// Now is the clock the server reads. Nil — the default — is time.Now.
	// It exists so time-dependent behavior can be asserted; the process
	// never sets it.
	Now func() time.Time
}
```

`main.go` must not set it. Two existing tests
(`TestCheckpointPassIsRecordedAndShownOnTheIndex`,
`TestCheckpointFailureNamesTheRetryDate`) compare `time.Now()` in the test
against a date the server derived from its own independent clock read; they
are latently flaky across midnight today and move onto the fixed clock in
Phase 1.

**The snapshot.**

```go
// drillScope is one request's view of a drill: the cards in scope, the stats
// over them, and the instant all of it was computed at. It is built once per
// request and passed down, so the card, the stats bar, the scheduler and the
// multiple-choice seed all agree about which cards are in scope and what day
// it is.
//
// Built and read on one handler goroutine and never stored on Server, so it
// needs no synchronization. It is passed by value despite being ~190 bytes —
// over the usual threshold for a pointer — because a per-request snapshot
// that no callee may mutate is worth the copy at the handful of call sites
// per request.
type drillScope struct {
	filter filter
	now    time.Time
	cards  []deck.Card
	terms  int
	stats  review.Stats
}

func (s *Server) newDrillScope(f filter, now time.Time) drillScope
```

`stats` is computed eagerly rather than lazily: every drill fragment renders
`drill-stats` (`internal/web/templates/fragments.html:22`, `53`, `78`, `95`),
so it is always needed exactly once. `handleBrowse` needs neither stats nor
prerequisite expansion and keeps calling `lib.Select` directly.

**Ordering — the snapshot comes after the mutation.** `newDrillScope` takes
`now` rather than reading the clock itself, because a handler that records an
answer needs the instant *before* it needs the snapshot. `handleGrade`,
`handlePick` and `handleCheckpointGrade` all write and then render counts that
must include the write; today every `Stats` read happens after it. The shape
is therefore:

```go
now := s.now()                       // once, first
_, err := s.store.Grade(id, g, now)  // the mutation, where the handler has one
sc := s.newDrillScope(f, now)        // then the snapshot
```

Building the snapshot at the top of the handler instead renders `ReviewsToday`
one behind and leaves the just-graded card in its pre-answer Due/New/Learning
bucket. **Nothing in the existing suite catches this** — no test asserts any
value in the `drill-stats` bar — which is why Phase 2 adds one.

**Signature changes.**

| Before | After |
| --- | --- |
| `scope(f filter) ([]deck.Card, int)` | replaced by `newDrillScope(f filter, now time.Time) drillScope` |
| `view(c deck.Card, f filter) cardView` | `view(c deck.Card, sc drillScope) cardView` |
| `choices(c deck.Card) []choiceView` | `choices(c deck.Card, now time.Time) []choiceView` |
| `next(f filter, exclude string) (deck.Card, bool)` | `next(sc drillScope, exclude string) (deck.Card, bool)` |
| `renderNext(w, f filter, exclude string)` | `renderNext(w, sc drillScope, exclude string)` |
| `checkpointView(module string, cards []deck.Card, status review.CheckpointStatus) checkpointView` | takes the status the handler already read, instead of computing one |

`choices` takes `now` rather than the whole snapshot so it keeps calling
`review.Day` itself — that function exists precisely to give the option seed
and the review counters one definition of "today" (`internal/review/review.go:152`).

`handleCheckpoint` reads `CheckpointStatus` once and passes it in. Starting an
attempt cannot change the status — `StartCheckpoint` writes an attempt with
`Done` and `Failed` false, which `checkpointStatus` still reads as
`CheckpointReady` — so no re-read is needed after the start.

`handleIndex` already reads the clock once (`web.go:351`) and is the pattern
the rest of the file adopts. It is unchanged.

**Error paths are preserved verbatim.** Restructuring the checkpoint handlers
must not touch how they report failure: the `slog.Error` + generic
`http.Error` pairing (detail to the log, nothing to the client) and the
`errors.Is(err, review.ErrCheckpointUnavailable)` → 409 branch at
`web.go:670` survive as they are.

**Unchanged**: every template, every URL, `cardView` and `checkpointView`
field sets, `handleBrowse`, `handleHealthz`, `handleReadyz`, `markdown()` and
its G203 trust-boundary comment (`web.go:320`), and everything in
`internal/deck`, `internal/review`, and `internal/chat`.

### Observability

No new signals. The per-request duration already logged by `logging` in
`cmd/flashcards/main.go:176` is where the reduced work surfaces; the
benchmark in the TDD plan is what actually measures it.

---

## Implementation Phases

Each phase ends green. The clock-read table grows one group of routes per
phase rather than being written up front and left failing across three.

### Phase 1: Clock seam — ✅ DONE

**Objective**: The server's clock is injectable, and the routes that already
comply are pinned so regressions are caught.

**Tasks**:

- [x] Add `Config.Now` and wire it through `New`, defaulting to `time.Now`
- [x] Add the `steppingClock` and `fixedClock` helpers —
      `internal/web/web_test.go`
- [x] Add `TestHandlersReadTheClockOnce` covering only the routes that already
      comply: one read for `/`, zero for `/browse`, `/healthz` and `/readyz`
- [x] Move `TestCheckpointPassIsRecordedAndShownOnTheIndex` and
      `TestCheckpointFailureNamesTheRetryDate` onto `fixedClock`, removing
      their dependence on the test and the server agreeing about the date
- [x] Sync the `Config` doc comment with the clock seam

**Deliverables**:

- `Config.Now`, plus the `steppingClock` / `fixedClock` helpers
- `TestHandlersReadTheClockOnce` green over four routes
- Two latently-flaky tests made deterministic
- Docs updated: `Config` doc comment in `internal/web/web.go`

### Phase 2: drillScope — ✅ DONE

**Objective**: A drill request reads the clock once and computes its scope and
stats once.

**Tasks**:

- [x] Record the pre-change `BenchmarkDrill` baseline in **Important Notes**
      (add the benchmark first, on unchanged code)
- [x] Extend `TestHandlersReadTheClockOnce` to the five drill routes
      (`/drill`, `/drill/{id}/reveal`, `/drill/{id}/grade`,
      `/drill/{id}/pick`, `/drill/{id}/advance`) — expected to fail
- [x] Add failing `TestDrillIsConsistentAcrossMidnight` and
      `TestGradeCountsTheAnswerItJustRecorded`
- [x] Add `drillScope` and `newDrillScope`; convert `view`, `choices`, `next`
      and `renderNext` to the signatures in the table above
- [x] Delete `Server.scope` and the discarded `Stats` call in `handleDrill`
- [x] Record the post-change benchmark alongside the baseline
- [x] Sync the `cardView` doc comment, which no longer needs to explain why a
      card view carries the whole scope's stats

**Deliverables**:

- `drillScope` built once per drill request, after any mutation
- `scope` removed; `store.Stats` called at most once per request
- Nine of twelve routes pinned by the clock table
- Benchmark delta recorded in **Important Notes**
- Docs updated: `cardView` and `drillScope` doc comments in
  `internal/web/web.go`

### Phase 3: Checkpoint path — ✅ DONE

**Objective**: A checkpoint request reads the clock once and its status once.

**Tasks**:

- [x] Extend `TestHandlersReadTheClockOnce` to `/checkpoint`,
      `/checkpoint/{id}/reveal` and `/checkpoint/{id}/grade` — expected to
      fail for the first and third, which read three and two times today
- [x] Thread one `now` through the three checkpoint handlers
- [x] Pass the already-read status into `checkpointView` and remove the
      duplicate `CheckpointStatus` call in `handleCheckpoint` (`web.go:612`)
- [x] Sync the `checkpointView` doc comment

**Deliverables**:

- `checkpointView` takes a status rather than computing one
- All twelve routes pinned by the clock table
- Docs updated: `checkpointView` doc comment in `internal/web/web.go`

**No repo documentation changes in any phase.** The README hardening backlog is
module-scoped, its Layout section is package-level, and `docs/curriculum.md`
has no entry for this work. The doc-sync tasks above are Go doc comments, and
they are the whole of it.

---

## Test-Driven Development Requirements

### TDD Plan

- **Helpers** (Phase 1), in `internal/web/web_test.go`:

  ```go
  // steppingClock returns a clock starting at start that advances by step on
  // every read, and a func reporting the read count.
  func steppingClock(start time.Time, step time.Duration) (func() time.Time, func() int)
  func fixedClock(at time.Time) func() time.Time
  ```

  Build `start` with an **explicit `time.UTC`**. `review.Day` formats in the
  instant's own location (`review.go:146`), so a `time.Local` start puts the
  day boundary somewhere different on a CI box in another zone — the same
  class of latent flakiness Phase 1 exists to remove. The read counter is an
  `atomic.Int64`: every test in this file calls `t.Parallel()`, each builds
  its own server with its own clock, and the counter stays correct if a test
  ever moves from `httptest.NewRecorder` to `httptest.NewServer`.

- **Phase 1 tests**: `TestHandlersReadTheClockOnce` — a table over `/`
  (one read) and `/browse`, `/healthz`, `/readyz` (zero). Named subtests, one
  per route.
- **Phase 2 tests**:
  - The Phase 1 table extended to the five drill routes.
  - `TestDrillIsConsistentAcrossMidnight` — grade a card at 23:00 UTC so
    `Reviews[day1]` is 1, then serve `/drill` with a clock starting at
    `23:59:59.9` UTC stepping by one second. Today the rendered "reviewed
    today" count comes from `view`'s `Stats`, which reads past midnight and
    renders `0`; afterwards the single read lands on day 1 and renders `1`.
  - `TestGradeCountsTheAnswerItJustRecorded` — POST a grade on a fixed clock
    and assert the returned fragment's "reviewed today" count includes it.
    This is the guard for the ordering rule in **Go Components**; it fails if
    the snapshot is built before the mutation.
  - `BenchmarkDrill`, using `b.Loop()` (Go 1.24+; the module is on 1.26.5).
- **Phase 3 tests**: the same table extended to the three checkpoint routes.
- **Regression**: the existing black-box suite in `internal/web` is the
  behavior-preservation proof. Any test that needs editing beyond the two
  Phase 1 clock conversions means behavior changed outside the day-boundary
  carve-out in **Production Bar**, and the change is wrong.
- **Benchmark procedure**: run the benchmark on unchanged code and again after
  Phase 2, `-count=6` each, and compare:

  ```sh
  go test -bench=BenchmarkDrill -benchmem -count=6 ./internal/web/ > /tmp/before.txt
  # ... implement ...
  go test -bench=BenchmarkDrill -benchmem -count=6 ./internal/web/ > /tmp/after.txt
  go run golang.org/x/perf/cmd/benchstat@latest /tmp/before.txt /tmp/after.txt
  ```

  If `benchstat` cannot be fetched, compare the `allocs/op` medians by hand.
- **Commands**:
  - `go test -race ./internal/web/ -run 'TestHandlersReadTheClockOnce|TestDrillIsConsistentAcrossMidnight|TestGradeCountsTheAnswerItJustRecorded' -v`
  - `make check`

### TDD Exceptions

- **"Scope is computed once" has no direct black-box assertion.** The
  recomputation is invisible from outside because every copy returns the same
  answer. It is evidenced by three things together: the one-clock-read-per-request
  table (scope derives from the snapshot's `now`), the `BenchmarkDrill`
  allocation delta, and `Server.scope` no longer existing for a second caller
  to reach. A white-box test asserting call counts was considered and rejected
  as testing an implementation detail.
- **"CheckpointStatus is read once" has no assertion at all.** It takes `now`
  as a parameter, so a second call with the same instant reads no clock and
  returns the same value — invisible to the clock counter and to the response.
  Asserting it would need an interface over `review.Store` purely for the
  test, which is a boundary change this spec forbids (**Notes for AI
  Agents** #3). Phase 3's clock reduction *is* asserted; the status dedup
  rides along as an unverified cleanup, and that is accepted.

---

## Technical Implementation Details

### Key Files

- `flashcards/internal/web/web.go` — `Config`, `drillScope`, the drill and
  checkpoint handlers
- `flashcards/internal/web/web_test.go` — clock helpers and the new assertions
- `flashcards/internal/web/templates/fragments.html` — unchanged; the reason
  `stats` is computed eagerly rather than lazily

### Implementation Patterns

- **Read, write, then snapshot.** The three mutating handlers all take the same
  shape, and it is the shape the ordering rule in **Go Components** describes:

  ```go
  now := s.now()                                       // once, first
  if _, err := s.store.Grade(id, review.Grade(g), now); err != nil { ... }
  s.renderNext(w, s.newDrillScope(filterFrom(r), now), id)
  ```

  `handleCheckpointGrade` is the same with the status re-read after the write
  rather than a scope built after it, because grading the last answer is exactly
  what changes the status the page renders.

- **Test fixtures compose rather than repeat.** `serverOver` delegates to a new
  `serverWith(tb, decks, cfg)`, so no existing call site moved to add a clock;
  `master`/`masterM0` gained `masterAt`/`masterM0At` the same way, for tests
  whose server runs on a fixed clock and whose reviews must land on the day the
  server thinks it is. `serverWith` and `do` take `testing.TB` so `BenchmarkDrill`
  shares the fixture instead of growing its own.

- **One fixture reaches every route.** `clockDecks` pairs a two-term glossary
  that is still being recognised — what `/drill` and `/pick` need — with the
  checkpoint module, whose ordinary cards `clockServer` masters so the three
  checkpoint routes are actually offered. Terms carry no module, so mastering M0
  leaves them new and the drill still has something to serve.

- **The clock table measures a delta, not a total**, so a route needing state
  another route creates (`/checkpoint/{id}/reveal` and `/grade` need an open
  attempt) can run its setup through the same server without its setup's reads
  being counted.

### Important Notes

- The four clock reads in `handleDrill` are `store.Next`, the discarded
  `store.Stats`, `view`'s `store.Stats`, and `choices`' `review.Day`. The last
  one is the subtle one: `handlePick` captures options before grading
  specifically so the answered rep keeps the options it was asked with
  (`web.go:461`), and a second clock read across midnight would re-seed them
  and defeat that.
- `BenchmarkDrill`, `-count=6`, darwin/arm64 Apple M1 Pro, over the two-deck
  clock fixture. `benchstat` of before against after:

  | | before | after | delta |
  | --- | --- | --- | --- |
  | sec/op | 40.12µ ± 3% | 36.83µ ± 3% | −8.21% (p=0.002, n=6) |
  | B/op | 63.77Ki ± 0% | 60.75Ki ± 0% | −4.73% (p=0.002, n=6) |
  | allocs/op | 286.0 ± 0% | 283.0 ± 0% | −1.05% (p=0.002, n=6) |

  The three allocations are the second `Select` plus the second `Stats` the
  request no longer does. They are a small share of the total because template
  execution dominates a drill response, and the fixture is six cards — the
  saving scales with the library, the rendering does not.

---

## Success Criteria

- [x] `go test -race ./internal/web/ -run TestHandlersReadTheClockOnce` passes
      over all twelve routes: one read each for the nine clock-reading routes,
      zero for `/browse`, `/healthz` and `/readyz`
- [x] `go test -race ./internal/web/ -run TestDrillIsConsistentAcrossMidnight`
      passes
- [x] `go test -race ./internal/web/ -run TestGradeCountsTheAnswerItJustRecorded`
      passes
- [x] `Server.scope` no longer exists
- [x] `benchstat /tmp/before.txt /tmp/after.txt` shows a decrease in
      `allocs/op` for `BenchmarkDrill`, with both figures written into
      **Important Notes** — 286 → 283
- [x] No template file is modified (`git diff --stat` lists nothing under
      `internal/web/templates/`) — the diff is `web.go` and `web_test.go` only
- [x] The only pre-existing tests modified are the two clock conversions in
      Phase 1. Growing `TestHandlersReadTheClockOnce`, which this spec
      introduces, does not count
- [x] `make check` passes

---

## Troubleshooting Guide

**Not applicable** — no problems encountered yet. Add entries as they are hit.

---

## Future Enhancements

- Split `web.go` into `server.go` / `markdown.go` / `filter.go` / `drill.go` /
  `checkpoint.go` / `browse.go` / `health.go`. This spec deliberately does not
  move code between files, so its diff stays reviewable as a behavior-preserving
  refactor.
- Collapse the four `markdown(c.Q)/markdown(c.A)/markdown(c.ECS)` sites onto a
  shared embedded `cardText`, which `cardView` and `checkpointView` both want.
  Whatever does this must carry the G203 trust-boundary comment at
  `web.go:320` with it; separating the comment from the call sites is how that
  protection gets deleted by accident later.
- Render the out-of-band `drill-stats` fragment separately from the card, which
  would let `cardView` stop carrying `Stats` and `Terms` at all. That is the
  full fix for the coupling this spec only makes cheap.

---

## Dependencies

### External Dependencies

**None added to the module.** `benchstat` is fetched on demand with
`go run golang.org/x/perf/cmd/benchstat@latest` for the Phase 2 comparison and
is not a build or test dependency; the benchmark itself runs without it.

### Internal Dependencies

- `flashcards/internal/review` — `Store.Stats`, `Store.Next`,
  `Store.CheckpointStatus` and `review.Day` already take `now` as a parameter,
  which is what makes a single request-level clock read possible without
  touching the store.
- `flashcards/internal/deck` — `Library.Select`, `WithPrerequisites` and
  `Options` are unchanged and already pure.

---

## Risks and Mitigation

### Technical Risks

- **Risk**: The snapshot gets built at the top of a handler that mutates, so
  the stats bar reports the state before the answer just recorded.
- **Mitigation**: The ordering rule in **Go Components**, and
  `TestGradeCountsTheAnswerItJustRecorded`, which exists for exactly this and
  is the one guard the current suite lacks.

- **Risk**: `Config.Now` is an exported seam that later gets wired to a config
  value, putting a fake clock in production.
- **Mitigation**: The doc comment says the process never sets it, and
  `cmd/flashcards/main.go` constructs `web.Config{Chat: ...}` with no `Now`.
  A grep for `Now:` outside `_test.go` should return nothing.

- **Risk**: Threading one `now` through `choices` changes which day seeds the
  multiple-choice options for a request straddling midnight, so a card's
  options could differ from a previous request's within the same sitting.
- **Mitigation**: That is the intended behavior — the options are already
  documented as stable for a day and re-rolled the next
  (`internal/deck/glossary.go:198`). The change makes one request internally
  consistent; it does not claim consistency across requests, and
  `TestPickShowsTheCorrectAnswer` already pins the within-request contract.

### Learning Risks

- **Risk**: Treating "read the clock once per request" as a style preference
  rather than a correctness rule, and reintroducing `time.Now()` calls deeper
  in the call stack later.
- **Mitigation**: `TestHandlersReadTheClockOnce` fails the build when a second
  read appears, which is the only durable form this rule takes.

---

## Notes for AI Agents

Follow the workflow in `docs/specs/TEMPLATE.md` under **Notes for AI Agents**.
Specific to this spec:

1. This is behavior-preserving apart from one carve-out: a request that
   crosses a day boundary. If a pre-existing test in `internal/web` fails,
   the refactor is wrong — do not adjust the test. The only sanctioned edits
   to pre-existing tests are the two clock conversions in Phase 1.
2. Do not touch any file under `internal/web/templates/`. `cardView` and
   `checkpointView` keep their exported field sets precisely so the templates
   do not move.
3. Do not move code between files, and do not introduce an interface over
   `review.Store`. The file split is **Future Enhancements**; a store
   interface was considered for testing the status dedup and rejected in
   **TDD Exceptions**.
4. `newDrillScope` takes `now`; it does not read the clock. That is what lets
   a mutating handler read the clock first, write, and then snapshot. See the
   ordering rule in **Go Components** — getting it backwards is the one way to
   pass every existing test and still be wrong.
5. `stats` on `drillScope` is eager, not lazy. Laziness was considered and
   rejected: every drill fragment renders `drill-stats`, and `handleBrowse` —
   the one path that needs no stats — does not use `drillScope`.
6. `drillScope` is per-request and goroutine-confined. Do not cache it on
   `Server`, and do not add a mutex to it; either change means it is being
   shared, which it must not be.
7. Do not move the clock into `context.Context`. A context carries
   request-scoped metadata, never dependencies or parameters, and a clock is a
   collaborator — which is why it lives on `Config`.
8. Load the `golang-code-style`, `golang-naming`, `golang-testing` and
   `golang-concurrency` skills before implementing, per `AGENTS.md`.
