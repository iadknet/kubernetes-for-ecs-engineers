# ECS Transfer Sections — Technical Spec

**Status**: 🚧 IN PROGRESS
**Last updated**: 2026-09-02

## Overview

Improve the flashcards' **Coming from ECS** sections so they transfer an
experienced ECS/Fargate engineer's operational instincts without forcing false
one-to-one mappings. Where configuration structure carries the lesson, the
section presents matching ECS JSON and Kubernetes YAML side by side. Scope is
the decks whose modules are reachable now — M0, M1, and the EKS lens — plus the
schema and rendering support the pattern needs. M2–M7 inherit the pattern as
each module is reached.

## Business Requirements

### Learning Objectives

- Each useful ECS anchor identifies the nearest familiar concept, where the
  mapping stops, and the consequence during deployment or an incident.
- Partial, split, and no-equivalent mappings are stated explicitly;
  provider-specific behavior is identified separately from the mapping class.
- Configuration-heavy comparisons show minimal matching excerpts and align the
  fields that produce the same—or importantly different—behavior.
- The sections remain concise post-answer context rather than becoming a second
  answer to memorize.

### Scope

- Audit and improve `ecs:` content in `01-foundations.yaml` (M0),
  `02-core-workloads.yaml` (M1), and `11-eks-and-aws.yaml` — 63 cards.
- Add structured paired-configuration content to cards where at least two
  directives align meaningfully or one ECS object maps across Kubernetes
  objects.
- Add an `ecs:` section to a card only when prior ECS/Fargate knowledge creates
  useful transfer or a likely misconception.
- Preserve each card's existing question, tested answer, module, and glossary
  prerequisites. If the audit exposes a defect outside `ecs:` or
  `ecs_comparison:`, record it as `blocked-factual-defect` in the audit and open
  separately scoped work. Do not widen this spec to absorb it.
- Decks `03`–`08` (M2–M7) inherit the content rules below when each module is
  reached, as part of that module's glossary-allowlist drain
  (`AGENTS.md`, "Vocabulary is gated"), which rewrites those same cards anyway.
  Editing them now would front-run just-in-time module authoring and guarantee a
  second pass.

### Non-Goals

- Requiring an ECS section on every card.
- Editing the M2–M7 module decks ahead of their modules.
- Rewriting the glossary, acronym, command-comparison, or capstone decks.
- Correcting factual defects outside the two ECS transfer fields.
- Adding ECS material to the tested answer merely to increase coverage.
- Dumping complete task definitions or manifests into a card.

## Technical Requirements

The existing optional `ecs:` field remains the mechanism for short prose
anchors. Each edited section should contain only the parts that materially
teach the card:

1. the nearest ECS/Fargate operational hook;
2. the mapping classification or analogy boundary; and
3. the production or troubleshooting consequence.

An optional `ecs_comparison:` object carries paired configuration. Use it only
when at least two directives align or when an ECS object splits across multiple
Kubernetes objects. It contains:

- a one-sentence operational scenario;
- labeled ECS JSON and Kubernetes YAML excerpts using the same workload name,
  ports, resources, health paths, and replica assumptions;
- field-alignment rows, each classified as `direct`, `partial`, `split`, or
  `no-equivalent`, with a caveat;
- the operational consequence; and
- deliberate omissions so an excerpt is never mistaken for production-ready
  configuration.

**Size ceilings.** The section is post-answer context, not a second answer.
`scenario`, `consequence`, and `omissions` are one sentence each; each excerpt
is at most 25 lines; `alignments` holds two to four rows. A comparison needing
more is teaching more than one thing and belongs on more than one card.

**Mapping classification.** `mapping` classifies the directive pair named in
that row, not the enclosing concept. An object-level split gets its own row
rather than downgrading an exact field match to `partial`. Provider specificity
belongs in `caveat` or the surrounding prose, not in the enum.

**Excerpt validity.** Kubernetes excerpts are complete objects: every field the
schema requires is present. `omissions` names production concerns — probes,
resources, rollout policy, traffic policy — and never a required schema field.
Multi-object excerpts are `---`-separated documents, so `kubernetes_yaml` parses
as a multi-document stream. Load-time validation enforces shape as well as
syntax: `ecs_json` is a non-empty JSON object, and every `kubernetes_yaml`
document is a non-empty mapping with no duplicate keys. Schema conformance
itself needs an API server and so stays in the dry-run gate below.
Field paths and semantics are verified against current official
documentation, and every Kubernetes excerpt is confirmed with
`kubectl apply --dry-run=server -f -` against the local KIND cluster before its
card lands. This honors the `k8s-for-ecs-engineers` rule to validate examples
with the project's available dry-run tools; it stays out of `make check`, which
is deliberately cluster-free.

**Vocabulary gate.** `ecs:` and `ecs_comparison:` stay outside the glossary
gate, which scans `q` and `a` only (`flashcards/internal/deck/glossary_test.go:723`).
The gate governs what a card asks you to recall, and these fields are
post-answer context, so comparison content may name terms the card does not
`require:`.

This is the canonical deck shape:

```yaml
ecs_comparison:
  scenario: >-
    flashcards runs three replicas behind one stable service endpoint.
  ecs_json: |
    {
      "serviceName": "flashcards",
      "taskDefinition": "arn:aws:ecs:region:account:task-definition/flashcards:7",
      "desiredCount": 3,
      "loadBalancers": [{
        "targetGroupArn": "arn:aws:elasticloadbalancing:region:account:targetgroup/flashcards/example",
        "containerName": "flashcards",
        "containerPort": 8080
      }]
    }
  kubernetes_yaml: |
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: flashcards
    spec:
      replicas: 3
      selector:
        matchLabels: {app: flashcards}
      template:
        metadata:
          labels: {app: flashcards}
        spec:
          containers:
            - name: flashcards
              image: flashcards:dev
              ports: [{containerPort: 8080}]
    ---
    apiVersion: v1
    kind: Service
    metadata:
      name: flashcards
    spec:
      selector: {app: flashcards}
      ports: [{port: 8080, targetPort: 8080}]
  alignments:
    - ecs: "service.desiredCount"
      kubernetes: "Deployment.spec.replicas"
      mapping: direct
      caveat: >-
        Both are the desired replica count; neither carries the traffic path.
    - ecs: "service.loadBalancers[] target-group wiring"
      kubernetes: "Service.spec.selector and Service.spec.ports"
      mapping: split
      caveat: >-
        One ECS block names container, port, and target group at once;
        Kubernetes splits pod choice from port mapping in a separate object.
    - ecs: "service.taskDefinition ARN"
      kubernetes: "Deployment.spec.selector.matchLabels"
      mapping: no-equivalent
      caveat: >-
        ECS binds a service to its tasks by ARN. A Deployment owns Pods only by
        label match, and a selector that disagrees with its own template labels
        is rejected at admission.
  consequence: >-
    Scaling the Deployment does not create or change its stable endpoint.
  omissions: >-
    Health probes, rollout strategy, resource requests and limits, and
    production traffic policy.
```

`scenario`, `ecs_json`, `kubernetes_yaml`, `consequence`, and `omissions` are
required non-empty strings. `alignments` requires two to four rows; every row
requires non-empty `ecs`, `kubernetes`, `mapping`, and `caveat` values.
`mapping` accepts exactly `direct`, `partial`, `split`, or `no-equivalent`.

The field is optional and independent of `ecs:`. A card may use either field or
both. When both exist, the renderer places `ecs:` prose first and the structured
comparison second under one **Coming from ECS** section. Scenario, alignment
cells, consequence, and omissions use the existing safe Markdown renderer;
configuration remains plain text escaped by `html/template`.
The template supplies the fixed excerpt labels **ECS/Fargate JSON** and
**Kubernetes YAML**; labels are not author-controlled schema fields.

The two excerpts render in equal-width columns above a `48rem` viewport media
query and stack ECS first at or below it. The alignment table and closing notes
render below them. Concrete details should use the flashcards service where they
make the comparison easier to act on. EKS-, AWS-, version-, and tool-sensitive
claims must be checked against current primary documentation.

Three approaches were considered:

- Blanket `ecs:` coverage would produce repetitive filler and contradict the
  minimum-information principle for spaced-repetition material.
- Separate ECS-comparison cards would make the analogy part of active recall,
  but would enlarge the curriculum and test a second objective on many topics.
- Improving the existing optional post-answer section preserves the current
  study model while allowing richer transfer where it matters. This is the
  selected approach.

For the configuration presentation, ordinary Markdown blocks were rejected
because the current renderer can only stack them, while GFM tables cannot
cleanly contain readable multi-line fenced JSON and YAML. A small structured
field plus a responsive template is selected: it produces literal side-by-side
configuration without enabling raw HTML in deck Markdown.

The choice follows the repository's existing teaching contract and
[SuperMemo's minimum-information guidance](https://www.supermemo.com/en/blog/twenty-rules-of-formulating-knowledge):
keep the recalled item atomic and put concise explanatory context outside the
tested answer.

### Kubernetes Resources

Not applicable. This is curriculum content only; the excerpts are teaching
artifacts, dry-run validated but never applied.

### Go Components

- `internal/deck/deck.go` — add `ECSComparison` and `ECSAlignment` types plus an
  optional `*ECSComparison` field on `Card`; validate the complete contract
  above, parsing `kubernetes_yaml` as a multi-document stream, while preserving
  cards that omit it.
- `internal/web/web.go` — add comparison view types and prepare comparison prose
  with the existing safe Markdown renderer while leaving configuration text as
  escaped strings for `html/template`.
- `internal/web/templates/` — render the paired excerpts, alignment table,
  consequence, and omissions in every card surface that currently renders the
  ECS section.
- `internal/web/templates/layout.html` — use a responsive two-column grid that
  stacks on narrow screens.

### Observability

No new runtime signals are needed. Invalid comparison data fails during the
existing deck-load path, and rendering regressions are covered by web tests;
the service's existing request logging is unchanged.

## Implementation Phases

### Phase 1: Establish the voice and paired format — 🚧 IN PROGRESS

**Objective**: Validate concise prose anchors and both paired-comparison
triggers — an object split and a dense field alignment — before applying the
pattern deck-wide.

**Tasks**:

- [x] Rewrite the representative M0 `ecs:` anchors with the three-part pattern:
      `m0-kubelet-is-the-thing-that-acts` (direct), `m0-kubeconfig-context`
      (partial), and `m0-reconciliation-loop` (no-equivalent).
- [x] Create `docs/specs/ecs-transfer-sections/AUDIT.tsv` with the header below
      and all 63 scoped card IDs; completed pilot rows carry their terminal
      dispositions and every unaudited row is `pending`.
- [x] Add failing parser tests in `flashcards/internal/deck/deck_test.go` and
      web tests in `flashcards/internal/web/web_test.go` for the structured
      comparison contract, validation failures, escaping, and all rendering
      surfaces.
- [x] Implement the optional comparison schema and responsive rendering.
- [x] Add the object-split pilot to `m1-service-collision`, where the ECS
      Service outcome splits across a Deployment and a Kubernetes Service.
- [x] Add the dense-field pilot to `m1-rolling-update-knobs`, aligning ECS
      `deploymentConfiguration` against `strategy.rollingUpdate`.
- [x] Dry-run both pilots' Kubernetes excerpts against the KIND cluster.
- [ ] Review both pilots with the learner against the rubric and revise the
      pattern if needed.
- [ ] Run deck validation and sync this spec and `AUDIT.tsv` with the accepted
      pilots.

**Deliverables**:

- `flashcards/decks/01-foundations.yaml` — reviewed prose pilot
- `flashcards/decks/02-core-workloads.yaml` — both reviewed paired pilots
- `flashcards/internal/deck/` and `flashcards/internal/web/` — validated,
  responsive comparison support
- `docs/specs/ecs-transfer-sections/SPEC.md` — accepted content rules
- `docs/specs/ecs-transfer-sections/AUDIT.tsv` — seeded card inventory

### Phase 2: Complete the M0 and M1 decks — ⏳ PLANNED

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
- [ ] Run deck validation and sync `SPEC.md` and `AUDIT.tsv`.

**Deliverables**:

- `flashcards/decks/01-foundations.yaml` and
  `flashcards/decks/02-core-workloads.yaml` — improved ECS transfer sections
- `docs/specs/ecs-transfer-sections/AUDIT.tsv` — terminal dispositions for both
  decks

### Phase 3: Improve the EKS lens and complete the audit — ⏳ PLANNED

**Objective**: Make the AWS-specific deck distinguish portable Kubernetes from
EKS-managed behavior and finish with a consistency pass.

**Tasks**:

- [ ] Audit all 16 cards in `11-eks-and-aws.yaml` for the same comparison
      pattern and current AWS behavior.
- [ ] Mark every deck-11 row `portable` or `eks-specific` in the `lens` column.
- [ ] Review all changed sections together for repetition, contradictions, and
      overlong explanations.
- [ ] Run the full flashcards checks.
- [ ] Complete `AUDIT.tsv`, sync `SPEC.md`, and mark only verified work
      complete.

**Deliverables**:

- `flashcards/decks/11-eks-and-aws.yaml` — improved EKS/Fargate transfer
  sections
- `docs/specs/ecs-transfer-sections/AUDIT.tsv` — complete, with the lens column
  populated for deck 11
- A validated, internally consistent set of module-level ECS anchors

## Test-Driven Development Requirements

### TDD Plan

- Structural checks: `make lint-decks` from `flashcards/` after each phase.
- Parser tests: table-driven cases in `flashcards/internal/deck/deck_test.go`
  accept a card without `ecs_comparison`, a complete comparison, and a
  multi-document `kubernetes_yaml`; reject each empty required field, malformed
  JSON/YAML, an excerpt that parses but is not object-shaped, a duplicate YAML
  key, an excerpt past the 25-line ceiling, fewer than two or more than four
  alignment rows, incomplete rows, and unknown mapping classifications. Focused
  command:
  `go test ./internal/deck -run ECSComparison`.
- Web tests: add a dedicated comparison fixture in
  `flashcards/internal/web/web_test.go`. Tests exercise free-recall and
  recognition drill results, browse, and checkpoint reveal and assert the
  paired labels, escaped configuration, Markdown-rendered prose, alignment
  rows, consequence, and omissions. They also prove raw HTML in comparison
  prose is omitted. Focused command:
  `go test ./internal/web -run ECSComparison`.
- Schema check: pipe every Kubernetes excerpt through
  `kubectl apply --dry-run=server -f -` against the KIND cluster from M0. This
  is an authoring-time gate, not part of `make check`.
- Responsive check: run `make run`, open `http://localhost:8080/browse?module=M1`
  and locate a pilot comparison at 1280x800 and 390x844, then inspect the
  computed layout. At 1280px the comparison grid has two equal tracks; at 390px
  it has one track, ECS precedes Kubernetes, and
  `document.documentElement.scrollWidth === document.documentElement.clientWidth`.
- Regression checks: `make check` from `flashcards/` before completion.
- Human content check: the learner accepts the M0 and M1 pilots only when each
  useful anchor answers yes to all five questions: Is the familiar hook clear?
  Is the analogy boundary explicit? Is there an actionable deployment or
  incident consequence? Does the section avoid duplicating the tested answer?
  Is the section shorter than the answer it supports? The paired pilots must
  additionally make the object split clear and not look production-complete.
- Review check for each edited section: familiar hook, analogy boundary, and
  operational consequence are present when relevant; no unsupported claim or
  duplicated tested answer remains.
- Audit checks: `AUDIT.tsv` has the header
  `card_id<TAB>deck<TAB>disposition<TAB>rationale<TAB>source<TAB>lens` and
  contains every card ID from decks 01, 02, and 11 exactly once. `pending` is
  allowed while its phase is open. From the repository root, both commands
  produce no output when the audit is complete:

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

  `source` holds a primary-source link whenever the claim is provider-,
  version-, or tool-sensitive.

### TDD Exceptions

- A failing automated test cannot establish teaching quality before a prose
  edit. Existing structural tests plus the explicit human review gate provide
  verification.
- The existing Go test stack cannot verify CSS layout. The two fixed viewport
  checks above are the required manual browser smoke test; structural HTML,
  ordering, and escaping remain automated.
- Schema validation of the Kubernetes excerpts needs a live API server, so it
  runs as the authoring-time dry-run above rather than inside `make check`.
  Parser tests prove syntax; `AUDIT.tsv` records the primary source used to
  verify field paths and semantics.

## Technical Implementation Details

### Key Files

- `flashcards/internal/deck/deck.go` — comparison schema and validation
- `flashcards/internal/web/web.go` — comparison view preparation
- `flashcards/internal/web/templates/fragments.html` — drill-card rendering
- `flashcards/internal/web/templates/browse.html` — browse-card rendering
- `flashcards/internal/web/templates/checkpoint-fragments.html` — checkpoint
  rendering
- `flashcards/internal/web/templates/layout.html` — responsive comparison CSS
- `docs/specs/ecs-transfer-sections/AUDIT.tsv` — one disposition and supporting
  rationale per scoped card

### Implementation Patterns

Keep raw JSON and YAML as plain strings and let `html/template` escape them.
Only prose fields pass through the existing non-Unsafe Goldmark renderer. Do
not enable raw HTML in Markdown to obtain the two-column layout.

### Important Notes

- ECS configuration is explanatory and never a runnable exercise.
- Kubernetes excerpts are schema-complete and dry-run validated, but they are
  teaching artifacts: they name their deliberate production omissions and are
  never applied to a cluster as part of an exercise.
- A missing required field is a defect, not an omission. `omissions` describes
  what would make the example production-ready, never what would make it valid.
- Side-by-side means visual columns on wide screens, not two unrelated code
  blocks placed sequentially.

## Success Criteria

- [ ] The learner accepts the M0 prose pilot and both M1 paired-configuration
      pilots as materially more useful under the five-question rubric.
- [ ] Every card in decks 01, 02, and 11 is recorded exactly once in
      `AUDIT.tsv`, and both audit commands produce no output.
- [ ] Newly added sections address a concrete transfer benefit or analogy trap;
      no coverage quota creates filler.
- [ ] Every paired comparison has schema-complete excerpts, two to four
      classified field alignments, an operational consequence, and omissions
      naming only production concerns.
- [ ] Every Kubernetes excerpt passes `kubectl apply --dry-run=server`.
- [ ] Paired excerpts render side by side at 1280x800 and ECS-first at 390x844,
      without horizontal overflow or unsafe Markdown HTML.
- [ ] Every deck-11 row carries a `portable` or `eks-specific` lens value.
- [ ] `make lint-decks` and `make check` pass from `flashcards/`.

## Troubleshooting Guide

Not applicable until implementation encounters a concrete issue.

## Future Enhancements

- M2–M7 ECS transfer sections (decks `03`–`08`) — deferred to each module's
  glossary-allowlist drain, under the content rules in **Scope**.

## Dependencies

### External Dependencies

- The KIND cluster from `modules/00-setup/` — dry-run validation of Kubernetes
  excerpts
- Current official Kubernetes documentation for portable behavior
- Current official AWS documentation for ECS and EKS behavior
- SuperMemo's minimum-information guidance for keeping recalled answers atomic

### Internal Dependencies

- `.agents/skills/k8s-for-ecs-engineers/SKILL.md` — authoritative comparison
  and production-review rules
- `README.md` — authoritative AWS-to-KIND substitution table
- `docs/curriculum.md` — module objectives and done-when criteria
- `docs/specs/ecs-k8s-skill-realism/SPEC.md` — completed teaching-contract work

## Risks and Mitigation

### Technical Risks

- **Risk**: Validation of the optional structure breaks existing cards that use
  only `ecs:` or neither ECS field.
- **Mitigation**: Keep `ecs_comparison` optional and test both legacy shapes
  before adding invalid-block cases.
- **Risk**: One card surface renders a different subset or escapes content
  differently.
- **Mitigation**: Exercise drill, browse, and checkpoint rendering from the same
  comparison fixture and keep configuration outside `template.HTML`.

### Learning Risks

- **Risk**: Longer sections turn card review into passive rereading.
- **Mitigation**: Enforce the size ceilings and the rubric's length question;
  limit the section to transfer, boundary, and consequence the answer does not
  already carry.
- **Risk**: An ECS analogy becomes memorable but technically misleading.
- **Mitigation**: Label partial, split, no-equivalent, and EKS-specific mappings
  explicitly and verify unstable claims with primary sources.
- **Risk**: An excerpt that silently drops a required field teaches a false
  mapping — the reader concludes the omitted binding does not exist.
- **Mitigation**: Require schema-complete objects, dry-run every excerpt, and
  keep required fields out of `omissions`.
- **Risk**: Adding a section to every card creates repetitive noise.
- **Mitigation**: Judge relevance card by card; blank is a valid result of the
  audit.

## Notes for AI Agents

Keep checkboxes, phase markers, status, and last-updated date synchronized with
verified work. Update this spec before implementation if the accepted pilot
changes the content pattern. Never add an analogy solely to satisfy a coverage
metric. When a later module is reached, apply these content rules to its deck as
part of that module's allowlist drain rather than reopening this spec.
