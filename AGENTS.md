# Agent Notes

This repository is a guided, hands-on Kubernetes training program for an
engineer with deep AWS ECS/Fargate experience and no prior Kubernetes. It
contains the curriculum, per-module exercises, and `flashcards/` — a small Go
service that is both the study tool and the example workload deployed and
hardened across the modules.

## Working Order

Read these before creating specs or making implementation changes:

1. `README.md` — program overview, the no-cloud-account constraint, the
   substitution table, and the module roadmap, which doubles as the progress
   tracker.
2. `docs/curriculum.md` — module-by-module objectives, exercises, and
   "done when" criteria.
3. `docs/target-roles.md` — the real job postings and take-home challenges that
   set the quality bar.
4. `docs/specs/TEMPLATE.md` — the technical spec template all work starts from.

## Spec-First Workflow

All non-trivial work starts as a written spec, not as code.

1. Create `docs/specs/<kebab-case-feature-name>/SPEC.md` by copying
   `docs/specs/TEMPLATE.md`. Supporting files live alongside it in the same
   directory.
2. Author it with the `spec-writing` skill, which defines what "lean" means.
3. Refine it with the human. Use the `spec-review` skill when a second pass is
   wanted; it reports P1–P3 findings and a readiness verdict.
4. Implement only once the spec is agreed.
5. Tick task and success-criteria checkboxes as each item actually completes,
   and update the spec's **Status** and **Last updated** in the same change as
   the work. An inaccurate checkbox is a defect.

If the work reveals the spec is wrong, update the spec first, then implement.
The code and the spec must not disagree silently.

## Agent Skills

- `.agents/skills/` is the authoritative source for repository-local Agent
  Skills. Keep shared skill content and resources coding-agent agnostic.
- `.claude/skills/<name>` entries are compatibility symlinks to the canonical
  directories. Edit `.agents/skills/<name>` rather than the symlinked path.
- Add a compatibility link when adding a canonical skill so Claude Code and
  agents that support `.agents/skills/` discover the same package.

## Project Direction

- **ECS is the teaching anchor.** The `k8s-for-ecs-engineers` skill holds the
  teaching rules — defer to it rather than restating them here.
- **Production realism by default.** Nothing is "done" because it ran. Surface
  the real gaps — resource limits, probes, RBAC, secret backing, disruption
  budgets, observability — even when an exercise's happy path doesn't need them.
- **No AWS account is available.** Nothing hands-on may depend on AWS;
  everything runs locally on KIND. The substitution table in `README.md` is
  authoritative for what replaces what.
- **Prefer established tools over hand-rolled ones**, and deployable artifacts
  (containers, manifests, charts) over local scripts.
- **From M1 on, `flashcards/` is the workload** that exercises deploy and harden.
- **The real interview submission is the human's own work.** The capstone is a
  rehearsal.

## Go Conventions

- `flashcards/` is the only Go module. Its `Makefile` and `.golangci.yml` are
  authoritative for build, test, and lint.
- Run `make check` in `flashcards/` before considering Go work done — the same
  aggregate `.githooks/pre-push` runs, so passing locally means the push passes.
  Single targets like `make test` are for iterating. `make help` lists them.
- The vendored `golang-*` skills cover style, naming, testing, error handling,
  and security conventions.

## Document Conventions

- Keep this file tool-agnostic. It is the single source of agent instructions;
  tool-specific entry points (such as `CLAUDE.md`) should only import it, never
  duplicate it.
- **No hand-maintained history or duplicated status.** No change logs, decision
  logs, or per-document status sections. Specs carry the *why*, git carries the
  *when*, and the README roadmap is the one place module status lives. A stale
  record is worse than no record.
- When a decision changes how the program works, edit the document that states
  how it works. Do not append a dated note about the change.
- Summarizing a README rule here is fine — this file must stand alone. Keep
  copies from drifting by naming one document authoritative, as above.
- Update the README roadmap's status marker as each module's **Done when** bar
  is met.
- Cite code as `path/to/file.go:42`.
