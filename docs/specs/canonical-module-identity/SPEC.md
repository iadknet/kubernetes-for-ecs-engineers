# Canonical Module Identity — Technical Spec

**Status**: ✅ DONE
**Last updated**: 2026-08-02

---

## Overview

A module name reaches the checkpoint routes from a URL query, where nothing
constrains its spelling. `Library.Checkpoints` matches it case-insensitively
(`flashcards/internal/deck/deck.go:332`), but `review.Store` keys its attempts
map on the string it is handed (`flashcards/internal/review/review.go:485`). The
two disagree, and a checkpoint sat as `?module=m0` records its pass under a key
nothing else reads.

This spec resolves the query's module to the library's own spelling once, at the
route boundary, so a request carries one canonical spelling — which then makes
`checkpointView`'s separate `module` parameter redundant, and it goes. It belongs
to no curriculum module: it is a correctness fix on the workload the modules
deploy, found while reviewing the signature the `per-request-scope-snapshot` spec
left behind.

---

## Business Requirements

### Learning Objectives

- I can explain why a lenient lookup paired with a strict key is a defect even
  when neither half is wrong on its own. `Checkpoints` is deliberately
  case-insensitive so a hand-typed URL still finds the exam; the store is
  deliberately exact so a map key means one thing. The bug lives in the seam,
  and the fix is to normalize at the boundary rather than to loosen the store or
  tighten the lookup.
- I can recognise when a redundant parameter is load-bearing. `checkpointView`
  taking both `module` and a `status` that already carries `Module` is harmless
  while the two are copies of the same string, and becomes a way for a caller to
  key on one module and render another the moment anything normalizes one of
  them. Collapsing it is what stops this fix from being undone by accident.

### Production Bar

- A checkpoint passed through any accepted spelling of its module is a passed
  checkpoint everywhere: the module's own page, the index, and the drill's
  withholding gate.
- Module identity has one source. No handler passes a module string to the store
  that did not come from the deck data.
- Behavior is unchanged for every canonical URL, which is every URL the UI
  generates. The existing suite is the proof.

---

## Technical Requirements

### Kubernetes Resources

**Not applicable** — a Go fix inside `internal/web`.

### Go Components

```text
- internal/web/web.go — checkpointCards returns the library's spelling of the
  module rather than the query's; checkpointView drops its now-redundant
  module parameter and reads status.Module.
- internal/web/web_test.go — sitCheckpointAs helper and the two new assertions.
```

**The defect.** All three checkpoint routes resolve their module through
`checkpointCards` (`web.go:620`), which returns `r.URL.Query().Get("module")`
verbatim. Sitting M0's exam as `?module=m0` therefore:

- passes and records under `Checkpoints["m0"]`,
- leaves `?module=M0` and the index reading `Checkpoints["M0"]`, which is empty,
- leaves `withheld` reading `Checkpoints[c.Checkpoint]` — the deck's own
  spelling (`review.go:616`) — so the exam cards never enter the drill queue,
- and propagates: every link the page renders is built from `Status.Module`
  (`templates/checkpoint-fragments.html:50`, `55`, `60`), so once a
  non-canonical spelling is in, the UI carries it forward.

Confirmed by hand before writing this spec: sitting the exam through
`?module=m0` renders `Passed` on the lowercase page while `?module=M0` and `/`
both still report it unsat.

**The fix.** A checkpoint card's own `Checkpoint` field is the canonical
spelling, and `Library.Checkpoints` only ever returns cards whose `Checkpoint`
is non-empty and matches the query case-insensitively. So the resolved module is
already in hand at the one place every checkpoint route passes through:

```go
func (s *Server) checkpointCards(w http.ResponseWriter, r *http.Request) (string, []deck.Card, bool) {
	module := r.URL.Query().Get("module")

	cards := s.lib.Checkpoints(module)
	if len(cards) == 0 {
		http.Error(w, "no checkpoint for module: "+module, http.StatusNotFound)
		return "", nil, false
	}

	// The card's own Checkpoint field is the deck's spelling; the query's is
	// whatever the caller typed, and Checkpoints matched it case-insensitively.
	// Returning the former is what keeps the store's attempt key, the index and
	// the drill's withholding gate all naming the same module.
	return cards[0].Checkpoint, cards, true
}
```

The 404 still names the module the caller asked for, because that is the string
they need to see corrected.

No new deck or store API: the resolution is a read of data the route already
holds. `handleIndex` needs no change — it iterates `lib.Modules()`, which is the
deck spelling already.

**Signature change.**

| Before | After |
| --- | --- |
| `checkpointView(module string, cards []deck.Card, status review.CheckpointStatus) checkpointView` | `checkpointView(cards []deck.Card, status review.CheckpointStatus) checkpointView` |

`status.Module` replaces the dropped parameter at its one use,
`store.NextCheckpoint`. The three call sites already pass a status built from
the same module, so this is behavior-identical on its own — its value is that
after Phase 1 there is one canonical module per request and no second parameter
able to carry a different one.

**Unchanged**: every template — they render `.Status.Module` and become correct
for free; `internal/deck` and `internal/review`, including the deliberate
`EqualFold` in `Checkpoints`; the drill and browse filters; and the checkpoint
handlers' error paths.

### Observability

No new signals. A rejected module already surfaces as the 404 at `web.go:625`,
and an accepted non-canonical one is no longer an event worth reporting once it
resolves to the same module as any other spelling.

---

## Implementation Phases

### Phase 1: Canonical resolution — ✅ DONE

**Objective**: A checkpoint sat through any accepted spelling of its module is
recorded and read back under the deck's spelling.

**Tasks**:

- [x] Add `sitCheckpointAs(t, h, module, grade)` and delegate `sitCheckpoint`
      to it — `internal/web/web_test.go`
- [x] Add failing `TestCheckpointPassIsRecordedUnderTheCanonicalModule` and
      `TestCheckpointPageLinksUseTheCanonicalModule`
- [x] Return `cards[0].Checkpoint` from `checkpointCards`
- [x] Sync the `checkpointCards` doc comment with what it now resolves

**Deliverables**:

- `checkpointCards` returning the deck's spelling
- Two tests green that fail on the current code
- Docs updated: `checkpointCards` doc comment in `internal/web/web.go`

### Phase 2: Collapse the redundant parameter — ✅ DONE

**Objective**: One parameter carries the module into the view, and it is the one
the status was built from.

**Tasks**:

- [x] Drop `module` from `checkpointView`, reading `status.Module` instead
- [x] Update the three call sites in `handleCheckpoint`,
      `handleCheckpointReveal` and `handleCheckpointGrade`
- [x] Sync the `checkpointView` doc comment, which can now say the status is
      the module identity rather than merely carrying it

**Deliverables**:

- `checkpointView(cards, status)`
- Phase 1's tests and the existing suite still green
- Docs updated: `checkpointView` doc comment in `internal/web/web.go`

**No repo documentation changes in either phase.** This is a defect fix inside
one package; `docs/curriculum.md` has no entry for it and the README roadmap
tracks module status, not workload fixes. The doc-sync tasks above are Go doc
comments, and they are the whole of it.

---

## Test-Driven Development Requirements

### TDD Plan

- **Helper** (Phase 1), in `internal/web/web_test.go`:

  ```go
  // sitCheckpointAs is sitCheckpoint with the module spelled as the caller
  // chooses, which is the whole point: the URL is where a non-canonical name
  // gets in.
  func sitCheckpointAs(t *testing.T, h http.Handler, module string, grade int) string
  ```

  `sitCheckpoint(t, h, grade)` delegates with `"M0"`, so no existing caller
  moves.

- **Phase 1 tests.** Both build their server with
  `serverWith(t, checkpointDecks(), web.Config{Now: fixedClock(clockStart)})`
  rather than `checkpointServerAt`, because both need the `*deck.Library` it
  also returns. Both then call `masterM0At(t, store, clockStart)` — mastering is
  a precondition, not a detail: an unmastered M0 renders a locked page with no
  card and no action URLs, and half of what these tests assert would not be on
  it. Every helper and fixture named in this bullet already exists, from the
  `per-request-scope-snapshot` work.
  - `TestCheckpointPassIsRecordedUnderTheCanonicalModule` — sit the exam clean
    through `?module=m0`, then assert all three readers named in the
    **Production Bar** see the pass: `/checkpoint?module=M0` and `/` both report
    it passed, and the drill's withholding gate has released the exam —
    `store.Stats(lib.Checkpoints("M0"), clockStart).Total` is 2, because
    `withheld` excludes an unpassed checkpoint card from `Total` entirely
    (`review.go:616`). Fails today on all three: the pass sits under `m0` and no
    reader looks there.

    Assert the release through the store, not through `/drill`. A card just
    passed is scheduled forward and is not yet due, so the drill page renders
    identically either way and the assertion would pass vacuously.
  - `TestCheckpointPageLinksUseTheCanonicalModule` — `GET /checkpoint?module=m0`
    and assert both link families in the body name `M0`: the status line's
    `/drill?module=` (`checkpoint.html:4`) and the action URLs from
    `checkpointView.URL()`. The second family is the one that matters — it is
    what puts a spelling into the `hx-post` targets the sitting runs on — and it
    only renders once a card is being sat. This is what pins `Status.Module` as
    canonical, which is the assumption Phase 2's collapse rests on.
- **Phase 2 tests**: none. See **TDD Exceptions**.
- **Regression**: the existing `internal/web` suite. Every URL it uses is
  already canonical, so any failure means the resolution changed behavior for
  canonical input, and the change is wrong.
- **Commands**:
  - `go test -race ./internal/web/ -run 'TestCheckpoint' -v`
  - `make check`

### TDD Exceptions

- **Phase 2 adds no test.** Dropping a parameter whose value is provably equal
  to `status.Module` at all three call sites is behavior-identical by
  construction — there is no input that distinguishes before from after, so a
  test asserting the difference cannot be written. It is verified by the
  compiler and by Phase 1's tests staying green. The reason to do it is that it
  removes the way this fix gets silently undone, and that is a structural
  property, not an observable one.

---

## Technical Implementation Details

### Key Files

- `flashcards/internal/web/web.go` — `checkpointCards`, `checkpointView`, the
  three checkpoint handlers
- `flashcards/internal/web/web_test.go` — `sitCheckpointAs` and the two new
  assertions

### Implementation Patterns

Fill in as the code is written.

### Important Notes

- `cards[0]` is always present and always carries a non-empty `Checkpoint`: the
  `len(cards) == 0` guard precedes it, and `Library.Checkpoints` only appends
  cards whose `Checkpoint` is non-empty (`deck.go:332`). A checkpoint card is
  also validated at load time to name a module that has cards (`deck.go:193`).
- **Which spelling `cards[0]` carries is not validated — do not assume the
  invariant while implementing.** `expandCheckpoints` looks up
  `byModule[c.Checkpoint]` as an exact map key (`deck.go:191`), so a deck
  declaring `module: m0` beside one declaring `module: M0` loads cleanly, while
  `Filter.Match` and `Checkpoints` both `EqualFold` them into one module.
  `Checkpoints("M0")` would then return cards from both decks and
  `cards[0].Checkpoint` would fall to deck filename order. The decks as they
  stand use one spelling per module, so the resolution is deterministic today;
  nothing enforces that it stays so. The load-time check that would is in
  **Future Enhancements**.
- The checkpoint fixture's card ids are `m0-checkpoint-split` and
  `m0-checkpoint-contexts`, so a bare `strings.Contains(body, "m0")` proves
  nothing about the module. The link assertion must require `module=M0` to be
  present and `module=m0` to be absent, rather than matching the bare spelling.
- **Existing state files are left alone.** A `review.json` written before this
  fix may hold an attempt under a non-canonical key. It becomes unreachable
  rather than wrong: every read after this change is canonical, so the entry is
  inert JSON. Migrating it would need the library inside `review.Open`, which
  takes a path and a daily cap and deliberately knows nothing about decks. The
  remedy for anyone who hit this is to re-sit the checkpoint, which is the
  correct outcome — the pass was never validly recorded.

---

## Success Criteria

- [x] `go test -race ./internal/web/ -run TestCheckpointPassIsRecordedUnderTheCanonicalModule`
      passes, covering all three readers in the **Production Bar**: the module's
      page, the index, and the drill's withholding gate
- [x] `go test -race ./internal/web/ -run TestCheckpointPageLinksUseTheCanonicalModule`
      passes, over a mastered M0 so the action URLs are in the body
- [x] `checkpointView` takes no `module` parameter
- [x] No module string reaches `review.Store` that did not come from deck data:
      the checkpoint routes resolve through `checkpointCards`, `handleIndex`
      through `lib.Modules()`
- [x] No template file is modified (`git diff --stat` lists nothing under
      `internal/web/templates/`)
- [x] No file under `internal/deck/` or `internal/review/` is modified
- [x] No pre-existing test is modified. `sitCheckpoint` gaining a delegate is a
      helper change, not a test change
- [x] `make check` passes

---

## Troubleshooting Guide

**Not applicable** — no problems encountered yet. Add entries as they are hit.

---

## Future Enhancements

- Validate at load time that no two decks declare module names differing only by
  case. `expandCheckpoints` matches `c.Checkpoint` to `c.Module` exactly while
  `Filter.Match` and `Checkpoints` `EqualFold`, so the loader permits a pair of
  spellings the rest of the app treats as one module — which is what leaves
  `cards[0].Checkpoint` deterministic by convention rather than by construction.
  Out of scope here because it changes `internal/deck`, which this spec keeps
  closed.
- Push the derivation into `review.Store`. Its four checkpoint methods already
  take the module's `cards`, so they could key on `cards[0].Checkpoint` and drop
  the `module` parameter entirely, making a non-canonical key unrepresentable
  rather than merely unreachable. Not done here because it changes
  `internal/review`'s API and its tests, where this spec fixes the defect at the
  one boundary that creates it.
- Canonicalize `filter.Module` the same way, so `/drill?module=m0` labels its
  scope `M0`. Cosmetic only: `Filter.Match` is already case-insensitive and
  nothing about a drill filter is persisted, which is why it is not part of this
  fix.

---

## Dependencies

### External Dependencies

**None added to the module.**

### Internal Dependencies

- `docs/specs/per-request-scope-snapshot/SPEC.md` — gave `checkpointView` the
  `status` parameter this spec collapses `module` into. Phase 2 assumes that
  three-argument signature exists.
- `flashcards/internal/deck` — `Library.Checkpoints` and `Card.Checkpoint` are
  unchanged and are what makes the canonical spelling available at the boundary.

---

## Risks and Mitigation

### Technical Risks

- **Risk**: The resolution is added to `checkpointCards` but some later route
  reads `r.URL.Query().Get("module")` directly and hands it to the store,
  reopening the defect one handler at a time.
- **Mitigation**: `checkpointCards` is the single entry point for all three
  checkpoint routes and returns the module as its first value, so the direct
  read has to be written deliberately. Phase 2's collapse removes the second
  place a module string could travel.

- **Risk**: Being lenient and normalizing, rather than strict and rejecting, is
  the wrong call — `?module=m0` could 404 instead.
- **Mitigation**: Strictness would mean making `Checkpoints` exact-match, which
  changes `internal/deck` and diverges from `Filter.Match`, the other
  case-insensitive module lookup. Matching the existing leniency and
  canonicalizing once keeps one rule across both.

### Learning Risks

- **Risk**: Reading this as a casing bug, and fixing future instances with a
  `strings.ToUpper` at the point of use rather than a resolution at the
  boundary.
- **Mitigation**: The canonical name is deck data, not a transformation of the
  input — there is no `ToUpper` that produces it, only a lookup. The test that
  sits the exam through a non-canonical URL is what keeps that distinction
  honest.

---

## Notes for AI Agents

Follow the workflow in `docs/specs/TEMPLATE.md` under **Notes for AI Agents**.
Specific to this spec:

1. Do not change `internal/deck` or `internal/review`. The `EqualFold` in
   `Library.Checkpoints` is deliberate and stays; the store's exact keying is
   deliberate and stays. The fix belongs between them.
2. Do not touch any file under `internal/web/templates/`. They already render
   `.Status.Module` and become correct the moment it is canonical — a template
   edit here means the fix was made in the wrong place.
3. `checkpointCards` returns the canonical module; nothing else re-derives it.
   If a handler needs the module, it takes the one `checkpointCards` gave it.
4. Phase 2 is behavior-identical and must stay that way. If dropping the
   `module` parameter changes any rendered byte, Phase 1 was incomplete.
5. Every existing test uses canonical URLs. If one fails, the resolution broke
   canonical input — do not adjust the test.
