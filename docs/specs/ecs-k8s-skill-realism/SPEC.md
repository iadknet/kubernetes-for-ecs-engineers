# ECS/Kubernetes Skill Realism — Technical Spec

**Status**: ✅ COMPLETED
**Last updated**: 2026-08-01

## Overview

Refine the repository's ECS-anchored Kubernetes teaching skill so its examples
are production-shaped, factually precise, and concise. This is curriculum-wide
guidance rather than work for a single module.

## Business Requirements

- Teach Kubernetes through accurate ECS/Fargate comparisons without implying
  false one-to-one equivalence.
- Ground configuration examples in a stated operational scenario, preferably
  the `flashcards` workload, and explain field alignment and consequences.
- Keep the skill smaller and less repetitive than its current 330-line,
  3,715-word form.

## Technical Requirements

### Kubernetes Resources

Not applicable. The change affects teaching instructions only.

### Go Components

Not applicable.

### Observability

Not applicable.

## Implementation Phases

### Phase 1: Refine and validate the skill — ✅ COMPLETED

**Objective**: Correct reviewed inaccuracies and consolidate the teaching
workflow without adding auxiliary reference files.

**Tasks**:

- [x] Replace the paired-configuration guidance with a scenario, comparison,
      alignment, consequence, and omissions workflow.
- [x] Correct resource, probe, disruption, Secret, EKS identity, and EKS
      compute guidance.
- [x] Remove repeated prose and keep the resulting skill smaller than before.
- [x] Validate structure, frontmatter, whitespace, and final size.
- [x] Sync affected documentation with the implemented changes.

**Deliverables**:

- `.agents/skills/k8s-for-ecs-engineers/SKILL.md` — concise, corrected teaching
  guidance
- `docs/specs/ecs-k8s-skill-realism/SPEC.md` — implementation record

## Test-Driven Development Requirements

### TDD Plan

- Structural validation: skill-creator `quick_validate.py`, or an equivalent
  frontmatter check if its optional YAML dependency is unavailable.
- Text validation: `git diff --check` and searches for the reviewed inaccurate
  claims.
- Size regression: compare line and word counts before and after the rewrite.

### TDD Exceptions

- A failing automated test cannot practically precede a prose-only rewrite;
  verification uses structural, content, and size checks after editing.

## Technical Implementation Details

### Key Files

- `.agents/skills/k8s-for-ecs-engineers/SKILL.md` — the only skill artifact
  changed

### Implementation Patterns

Keep stable teaching rules in the skill. Require current primary-source
verification for provider- or version-sensitive claims rather than embedding a
large static example library.

### Important Notes

- AWS configuration remains explanatory; runnable work remains KIND-only.
- The skill must distinguish voluntary evictions, rollout behavior, and
  involuntary failures.

## Success Criteria

- [x] The skill requires scenario-based paired configuration when it adds
      teaching value.
- [x] Reviewed factual issues are corrected without losing the ECS anchor.
- [x] The final skill has fewer than 330 lines and 3,715 words: 200 lines and
      1,830 words.
- [x] Structural and whitespace validation pass.

## Troubleshooting Guide

### Bundled validator lacks PyYAML

**Problem**: `quick_validate.py` exits before validation with
`ModuleNotFoundError: No module named 'yaml'`.
**Solution**: Run an equivalent Ruby YAML/frontmatter validation plus
`git diff --check`; both pass.
**Reference**: `.codex/skills/.system/skill-creator/scripts/quick_validate.py`

## Future Enhancements

Not applicable.

## Dependencies

### External Dependencies

- Current official Kubernetes and AWS documentation for version-sensitive
  claims

### Internal Dependencies

- `README.md` — authoritative no-AWS substitution table
- `docs/target-roles.md` — production and interview bar

## Risks and Mitigation

### Learning Risks

- **Risk**: Concision removes a caveat that prevents a misleading analogy.
- **Mitigation**: Preserve relationship categories and operational
  consequences while deleting repeated explanation.

## Notes for AI Agents

Keep checkboxes and status synchronized with verified work. Update this spec
before implementation if scope changes.
