# ECS Transfer Deck Rollout — Technical Spec

**Status**: ⏳ PLANNED
**Last updated**: 2026-09-02

## Overview

Apply the accepted **Coming from ECS** transfer pattern across every card of the
reachable decks — M0 (`01-foundations.yaml`), M1 (`02-core-workloads.yaml`), and
the EKS lens (`11-eks-and-aws.yaml`) — and finish with a consistency pass. The
pattern, the `ecs_comparison:` schema, and the responsive rendering are already
built and validated on pilot cards; that work and its authoring rules live in
[`docs/specs/ecs-transfer-sections/SPEC.md`](../ecs-transfer-sections/SPEC.md).
This spec is content authoring against that pattern and adds no code.

## Business Requirements

### Learning Objectives

- Each useful ECS anchor identifies the nearest familiar concept, where the
  mapping stops, and the consequence during deployment or an incident.
- The AWS-specific deck distinguishes portable Kubernetes behavior from
  EKS-managed behavior on every card.
- The full set of ECS anchors is internally consistent: no repetition,
  contradiction, or overlong explanation across decks.

### Scope

- Audit and improve `ecs:` / `ecs_comparison:` content for all 63 scoped cards:
  24 in `01-foundations.yaml`, 23 in `02-core-workloads.yaml`, and 16 in
  `11-eks-and-aws.yaml`. The five pilot cards from the pattern spec are already
  terminal in the audit and are not re-audited.
- Record each card's disposition, rationale, and any primary source in
  `docs/specs/ecs-transfer-sections/AUDIT.tsv`, and populate the `lens` column
  for every deck-11 row.
- Preserve each card's question, tested answer, module, and glossary
  prerequisites. A factual defect outside the two ECS transfer fields is
  recorded as `blocked-factual-defect` and scoped separately, never absorbed
  here.

### Non-Goals

- Changing the `ecs_comparison:` schema, validation, or rendering — that is
  shipped and specified in the pattern spec.
- Requiring an ECS section on every card; neutral cards stay intentionally
  blank.
- Editing the M2–M7 module decks ahead of their modules, or the glossary,
  acronym, command-comparison, or capstone decks.

## Technical Requirements

The authoring contract is defined in the pattern spec's **Technical
Requirements** and is unchanged. In brief, each edited section carries only what
teaches the card:

1. the nearest ECS/Fargate operational hook;
2. the mapping classification or analogy boundary; and
3. the production or troubleshooting consequence.

An optional `ecs_comparison:` object is added only where at least two directives
align or one ECS object splits across multiple Kubernetes objects. Size ceilings
(one-sentence `scenario`/`consequence`/`omissions`, ≤25-line excerpts, two-to-
four `alignments` rows), the `direct`/`partial`/`split`/`no-equivalent` mapping
enum, and excerpt validity rules all hold as specified there.

### Kubernetes Resources

Not applicable. Excerpts are teaching artifacts, dry-run validated but never
applied.

### Go Components

Not applicable. No code changes; the schema, validation, and rendering shipped
with the pattern spec.

### Observability

Not applicable. No runtime signals change.

## Implementation Phases

### Phase 1: Complete the M0 and M1 decks — ⏳ PLANNED

**Objective**: Apply the accepted pattern across both reachable module decks
without manufacturing weak analogies.

**Tasks**:

- [ ] Audit all 24 cards in `01-foundations.yaml` for accuracy, boundary,
      consequence, and concision.
- [ ] Audit all 23 cards in `02-core-workloads.yaml` on the same terms.
- [ ] Add paired comparisons where the trigger applies, prioritizing rollouts,
      routing, and workload ownership.
- [ ] Add sections where ECS experience materially helps and leave neutral
      cards intentionally blank.
- [ ] Verify provider- and version-sensitive claims against primary sources and
      dry-run every new Kubernetes excerpt.
- [ ] Record each card's disposition, rationale, and any required primary source
      in `AUDIT.tsv`.
- [ ] Run deck validation and sync this spec and `AUDIT.tsv`.

**Deliverables**:

- `flashcards/decks/01-foundations.yaml` and
  `flashcards/decks/02-core-workloads.yaml` — improved ECS transfer sections
- `docs/specs/ecs-transfer-sections/AUDIT.tsv` — terminal dispositions for both
  decks

### Phase 2: Improve the EKS lens and complete the audit — ⏳ PLANNED

**Objective**: Make the AWS-specific deck distinguish portable Kubernetes from
EKS-managed behavior and finish with a consistency pass.

**Tasks**:

- [ ] Audit all 16 cards in `11-eks-and-aws.yaml` for the same comparison
      pattern and current AWS behavior.
- [ ] Mark every deck-11 row `portable` or `eks-specific` in the `lens` column.
- [ ] Review all changed sections together for repetition, contradictions, and
      overlong explanations.
- [ ] Run the full flashcards checks.
- [ ] Complete `AUDIT.tsv`, sync this spec, and mark only verified work
      complete.

**Deliverables**:

- `flashcards/decks/11-eks-and-aws.yaml` — improved EKS/Fargate transfer
  sections
- `docs/specs/ecs-transfer-sections/AUDIT.tsv` — complete, with the lens column
  populated for deck 11
- A validated, internally consistent set of module-level ECS anchors

## Test-Driven Development Requirements

### TDD Plan

The comparison schema is covered by the parser and web tests shipped with the
pattern spec; this spec adds content, not behavior, so it relies on those plus
the following authoring-time gates run per phase:

- Structural checks: `make lint-decks` from `flashcards/` after each phase.
- Schema check: pipe every new Kubernetes excerpt through
  `kubectl apply --dry-run=server -f -` against the KIND cluster from M0. This
  is an authoring-time gate, not part of `make check`.
- Audit checks: from the repository root, both commands produce no output once
  the audit is complete:

  ```bash
  # Inventory is complete and duplicate-free.
  diff \
    <(rg --no-filename '^  - id: ' \
      flashcards/decks/{01-foundations,02-core-workloads,11-eks-and-aws}.yaml \
      | sed 's/^  - id: //' | sort) \
    <(tail -n +2 docs/specs/ecs-transfer-sections/AUDIT.tsv | cut -f1 | sort)

  # Every row has a terminal disposition, a rationale, and a deck-11 lens.
  awk -F'\t' 'NR>1 && ($3 !~ /^(keep|rewrite|add|paired-comparison|not-useful|blocked-factual-defect)$/ \
    || $4 == "" \
    || ($2 == "11-eks-and-aws" && $6 !~ /^(portable|eks-specific)$/))' \
    docs/specs/ecs-transfer-sections/AUDIT.tsv
  ```

- Regression checks: `make check` from `flashcards/` before completion.
- Human content check: each useful anchor answers yes to all five rubric
  questions — Is the familiar hook clear? Is the analogy boundary explicit? Is
  there an actionable deployment or incident consequence? Does the section avoid
  duplicating the tested answer? Is the section shorter than the answer it
  supports? Paired comparisons must additionally make the object split clear and
  not look production-complete.

### TDD Exceptions

- A failing automated test cannot establish teaching quality before a prose
  edit. The structural tests, dry-run gate, and the human rubric review provide
  verification instead.

## Technical Implementation Details

### Key Files

- `flashcards/decks/01-foundations.yaml`, `02-core-workloads.yaml`,
  `11-eks-and-aws.yaml` — the audited content
- `docs/specs/ecs-transfer-sections/AUDIT.tsv` — one disposition and supporting
  rationale per scoped card
- `docs/specs/ecs-transfer-sections/SPEC.md` — the authoritative pattern,
  schema, and rendering contract this spec applies

### Important Notes

- ECS configuration is explanatory and never a runnable exercise.
- A missing required field is a defect, not an omission. `omissions` names
  production concerns, never a required schema field.
- Verify EKS-, AWS-, version-, and tool-sensitive claims against current primary
  documentation before a deck-11 card lands.

## Success Criteria

- [ ] Every card in decks 01, 02, and 11 is recorded exactly once in
      `AUDIT.tsv`, and both audit commands produce no output.
- [ ] Newly added sections address a concrete transfer benefit or analogy trap;
      no coverage quota creates filler.
- [ ] Every paired comparison has schema-complete excerpts, two to four
      classified field alignments, an operational consequence, and omissions
      naming only production concerns.
- [ ] Every Kubernetes excerpt passes `kubectl apply --dry-run=server`.
- [ ] Every deck-11 row carries a `portable` or `eks-specific` lens value.
- [ ] `make lint-decks` and `make check` pass from `flashcards/`.

## Troubleshooting Guide

Not applicable.

## Future Enhancements

- Decks `03`–`08` (M2–M7) inherit this pattern when each module is reached, as
  part of that module's glossary-allowlist drain. Editing them now would
  front-run just-in-time module authoring.

## Dependencies

### External Dependencies

- KIND cluster from `modules/00-setup/` — target for `kubectl --dry-run=server`
  excerpt validation.

### Internal Dependencies

- `docs/specs/ecs-transfer-sections/SPEC.md` — the shipped pattern, schema, and
  rendering this spec applies.
- `docs/specs/ecs-transfer-sections/AUDIT.tsv` — the seeded 63-card inventory
  this spec drives to terminal dispositions.

## Risks and Mitigation

### Learning Risks

- **Risk**: A coverage instinct manufactures weak analogies on cards where ECS
  experience does not transfer.
- **Mitigation**: The five-question rubric and an explicit `not-useful`
  disposition; neutral cards stay blank on purpose.

## Notes for AI Agents

When implementing against this spec:

1. Tick each checkbox as that item actually completes — never in a batch, never
   for skipped work. A checked box means the artifact exists and its
   verification passed.
2. Update the phase status markers and the top **Status** / **Last updated**
   fields in the same change as the work they describe.
3. The schema and rendering are frozen. If a card seems to need a schema change,
   stop and revise the pattern spec first, then implement.
4. Verify provider-, version-, and tool-sensitive claims against current primary
   documentation and record the source in `AUDIT.tsv`.
5. Cite code as `path/to/file.go:42`.
