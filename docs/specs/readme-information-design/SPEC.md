# README Information Design — Technical Spec

**Status**: ✅ COMPLETED
**Last updated**: 2026-09-03

## Overview

Make the repository README a concise entry point. Preserve the author-written
opening exactly, keep the local substitution table and module roadmap
authoritative, and link to detailed documents instead of repeating them.

## Business Requirements

- A new reader can quickly understand what the project is, how to start, and
  where to find detail.
- README lines 1–40 remain unchanged.
- The README remains the source of truth for AWS-to-local substitutions and
  module progress.
- Detailed flashcard behavior, curriculum content, and contributor rules stay
  in their existing documents.

## Technical Requirements

- Replace README content after the opening with these sections: Start here,
  How the program works, Local substitutions, Roadmap, Project map, Working
  agreement, and Security and license.
- Prefer short paragraphs, compact tables, and links over prose that duplicates
  `flashcards/README.md`, `docs/curriculum.md`, or `AGENTS.md`.

### Kubernetes Resources

Not applicable.

### Go Components

Not applicable.

### Observability

Not applicable.

## Implementation Phases

### Phase 1: Restructure and trim the README — ✅ COMPLETED

**Objective**: Turn the README into a clear project entry point without losing
its authoritative tables.

**Tasks**:

- [x] Preserve the opening at README lines 1–40.
- [x] Remove duplicated implementation and teaching detail.
- [x] Reorganize the remaining content around reader tasks.
- [x] Verify links and Markdown structure.

**Deliverables**:

- `README.md` — concise repository entry point
- `docs/specs/readme-information-design/SPEC.md` — this spec

## Test-Driven Development Requirements

Not applicable. This is a prose-only change. Verification consists of checking
the preserved opening, local links, headings, and table structure.

## Technical Implementation Details

### Key Files

- `README.md` — edited document
- `flashcards/README.md` — detailed app usage and behavior
- `docs/curriculum.md` — detailed module objectives and completion criteria
- `AGENTS.md` — contributor and agent workflow

### Implementation Patterns

Keep one level of summary in the README, then link to the owning document.

### Important Notes

- The opening through the second demo image is author-written and must remain
  byte-for-byte unchanged.
- The substitution and roadmap tables stay in the README because other project
  documents treat them as authoritative.

## Success Criteria

- [x] README lines 1–40 are unchanged.
- [x] The local substitution table and roadmap remain present.
- [x] Detailed flashcard behavior is linked rather than duplicated.
- [x] Every local Markdown link resolves.

## Troubleshooting Guide

Not applicable.

## Future Enhancements

Not applicable.

## Dependencies

### External Dependencies

None.

### Internal Dependencies

- `docs/curriculum.md` — detailed curriculum source
- `flashcards/README.md` — detailed application source
- `docs/target-roles.md` — hiring-target context
- `AGENTS.md` — workflow source

## Risks and Mitigation

- **Risk**: Trimming removes rules another document expects the README to own.
  **Mitigation**: Retain the substitution table and roadmap in full.
- **Risk**: Exact card counts become stale. **Mitigation**: Omit counts from the
  repository overview and leave application detail to `flashcards/README.md`.

## Notes for AI Agents

Preserve the opening and both authoritative tables when making future README
edits. Put curriculum, application, and workflow detail in their owning docs.
