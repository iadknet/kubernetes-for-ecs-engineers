# Clear Teaching Writing Skill — Technical Spec

**Status**: ✅ COMPLETE
**Last updated**: 2026-09-02

## Overview

Add a repository-local Agent Skill, `clear-teaching-writing`, that governs the
prose *style* of this project's teaching text — flashcard `q`/`a`, `ecs` and
`ecs_comparison` sections, and the teaching docs and explanations across the
repo. It is the companion to `k8s-for-ecs-engineers`: that skill decides what is
true and worth teaching; this one decides how the sentence reads. The immediate
driver is the ECS-transfer deck rollout, which will author or revise dozens of
cards; a shared style skill keeps that prose clear and free of the convoluted
phrasing that otherwise creeps in.

This is a small guidelines skill: the deliverable is one `SKILL.md` and its
symlink. There is no code, and no automated verification — a human accepts the
guidance.

## Business Requirements

### Skill Objectives

- An author revising or writing teaching text produces plain, direct prose: one
  idea per sentence, the answer first, a named actor, concrete over abstract.
- The skill states, as checkable anti-patterns, the specific convolutions to
  cut — em-dash asides, "not X but Y", rule-of-three padding, "which is why"
  chains, hedging, uniform sentence rhythm, monotone openers — each with a
  before → after fix.
- Flashcard text additionally honors the spaced-repetition contract: minimum
  information, atomic recall, the explanation shorter than the answer it
  supports.
- The skill defers cleanly: `k8s-for-ecs-engineers` owns teaching content,
  `spec-writing` owns specs, `golang-code-style` owns code and comments. This
  skill changes wording, never facts, mappings, or required caveats.

### Quality Bar

- The guidance sharpens convoluted text without flattening good prose or
  stripping technical precision, API names, or deliberate caveats. It frames the
  anti-tells as heuristics, not hard bans.
- The skill conforms to the repository's Agent Skill conventions in `AGENTS.md`:
  a canonical `.agents/skills/` package plus a `.claude/skills/` compatibility
  symlink, coding-agent agnostic.

## Technical Requirements

The deliverable is one skill package. No code, manifests, or runtime changes.

- `.agents/skills/clear-teaching-writing/SKILL.md` — canonical skill, single
  file, matching the shape and length of
  `.agents/skills/k8s-for-ecs-engineers/SKILL.md` (~150–200 lines). Frontmatter
  carries `name` and a `description` scoped to writing or revising this
  project's teaching text — cards, teaching docs, and explanations — and
  explicitly not code comments or commit messages, so it does not fire in
  `golang-code-style`'s domain.
- `.claude/skills/clear-teaching-writing` — relative symlink to the canonical
  directory, per `AGENTS.md`.

Required `SKILL.md` sections:

1. **Core rule** — one recalled idea per card, plain language, the explanation
   shorter than the answer it supports.
2. **Plain-language principles** — lead with the answer; one clause does one
   job; active voice with a named actor; concrete operational detail over
   abstraction; a working sentence-length target with deliberate variation.
3. **Spaced-repetition layer** — minimum-information and atomic formulation for
   card text; do not pack two facts into one sentence.
4. **Anti-tells checklist** — the core: each convolution named, why it reads
   badly, and a before → after rewrite drawn from realistic card prose.
5. **Boundaries** — what this skill does not touch (content accuracy and
   mappings → `k8s-for-ecs-engineers`; specs → `spec-writing`; code and comments
   → `golang-code-style`); this skill changes wording only.
6. **Self-review pass** — a short checklist an author runs before text lands.

### Kubernetes Resources

Not applicable.

### Go Components

Not applicable.

### Observability

Not applicable.

## Implementation Phases

### Phase 1: Author and wire the skill — ✅ COMPLETE

**Objective**: Ship the skill package.

**Tasks**:

- [x] Draft `.agents/skills/clear-teaching-writing/SKILL.md` covering the six
      required sections, distilling the sources in **Dependencies** and citing
      them.
- [x] Build the before → after examples in the anti-tells checklist from real
      convoluted phrasings, each fix preserving the technical content.
- [x] Add the `.claude/skills/clear-teaching-writing` compatibility symlink.
- [x] Add a one-line companion cross-reference to `clear-teaching-writing` in
      `.agents/skills/k8s-for-ecs-engineers/SKILL.md` so the content skill points
      to the style skill.

**Deliverables**:

- `.agents/skills/clear-teaching-writing/SKILL.md` — the skill
- `.claude/skills/clear-teaching-writing` — compatibility symlink
- `.agents/skills/k8s-for-ecs-engineers/SKILL.md` — companion cross-reference
- `docs/specs/clear-teaching-writing/SPEC.md` — this spec, kept in sync

## Test-Driven Development Requirements

Not applicable. The deliverable is prose guidance, not code or a manifest; there
is nothing to unit test or dry-run. The gate is a human reading the finished
`SKILL.md` and accepting the guidance.

## Technical Implementation Details

### Key Files

- `.agents/skills/k8s-for-ecs-engineers/SKILL.md` — the companion whose shape,
  length, and tone this skill matches, and whose content boundary it respects.
- `AGENTS.md` — the Agent Skills conventions (canonical dir + symlink) this skill
  follows.

### Important Notes

- Style only. If applying the skill would change a fact, mapping, or caveat,
  stop — that belongs to `k8s-for-ecs-engineers`, not here.
- Preserve technical terms, API names, UI labels, modal verbs, and deliberate
  caveats verbatim; clarity is not the removal of necessary precision.
- The anti-tells are heuristics, not hard bans. An em dash or a triad used
  deliberately is fine; the target is the reflexive, patterned overuse.

## Success Criteria

- [x] `.agents/skills/clear-teaching-writing/SKILL.md` exists with all six
      required sections and cited sources.
- [x] `.claude/skills/clear-teaching-writing` resolves to the canonical
      directory.
- [x] `k8s-for-ecs-engineers/SKILL.md` carries the companion cross-reference.
- [x] The author accepts the finished guidance as clear, correctly scoped, and
      within its boundaries.

## Troubleshooting Guide

Not applicable.

## Future Enhancements

- Split the anti-tells into a `references/` file if the checklist outgrows a
  single readable `SKILL.md`.
- Apply the skill in a pass over the M0/M1 decks as the ECS-transfer deck
  rollout reaches them.

## Dependencies

### External Dependencies

Source material the skill distills and cites (no runtime dependency):

- [Google Developer Documentation Style Guide](https://developers.google.com/style)
  — plain-language principles and sentence-length guidance.
- [SuperMemo — Twenty rules of formulating knowledge](https://www.supermemo.com/en/blog/twenty-rules-of-formulating-knowledge)
  and the minimum-information principle — the spaced-repetition layer; already
  cited by the ECS-transfer pattern spec.
- Practitioner research on LLM writing "tells" (em-dash overuse, "it's not X,
  it's Y", rule-of-three, low sentence-length variance, monotone openers) — the
  concrete anti-pattern list. Reference `nbj-write-clearly` as prior-art
  clear-writing skill, but do not adopt it: it targets developer docs, not
  flashcards, and is not ECS-anchored.

### Internal Dependencies

- `k8s-for-ecs-engineers` — the content skill this pairs with.
- The flashcards decks under `flashcards/decks/` — the primary text this skill
  is applied to.

## Risks and Mitigation

### Quality Risks

- **Risk**: The skill flattens good, deliberately varied prose into a bland
  house style, or strips necessary technical precision.
- **Mitigation**: Frame the anti-tells as heuristics and mandate preserving
  terms and caveats; the author reviews the guidance before it ships.

- **Risk**: Scope creep into content correctness, duplicating or contradicting
  `k8s-for-ecs-engineers`.
- **Mitigation**: The **Boundaries** section names the owning skill for each
  concern; this skill changes wording only.

## Notes for AI Agents

When implementing against this spec:

1. Tick each checkbox as that item actually completes — never in a batch, never
   for skipped work.
2. Update the phase status marker and the top **Status** / **Last updated**
   fields in the same change as the work.
3. Match the canonical skill convention in `AGENTS.md`: edit the
   `.agents/skills/` package and add the `.claude/skills/` symlink rather than a
   copy.
4. Keep the skill coding-agent agnostic; do not hard-code one tool's paths or
   commands.
5. Cite code and files as `path/to/file:42`.
