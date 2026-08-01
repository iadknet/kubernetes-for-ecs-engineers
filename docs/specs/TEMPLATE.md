# [Feature Name] — Technical Spec

**Status**: ⏳ PLANNED
**Last updated**: YYYY-MM-DD

Copy this file to `docs/specs/<kebab-case-feature-name>/SPEC.md` and fill it in.
Supporting files live alongside it in the same directory. Delete every
`**Instructions**` and `**Example**` block as you fill the section in — a spec
that still contains template instructions is not finished.

**Module and platform specs:** For documents that span cluster setup, tooling,
or several components rather than a single feature, follow the same section
order below and mark subsections **Not applicable** where they do not apply.

**Status markers**: ✅ COMPLETED · 🚧 IN PROGRESS · ⏳ PLANNED

---

## Overview

**Instructions**: Write 2-3 sentences describing what this delivers and why it
is needed now. Keep it simple and clear. Name the module it belongs to (M0–M7,
CAP) when it maps to one.

**Example**: "This spec covers deploying the flashcards service to the KIND
cluster as a multi-replica Deployment fronted by a Service. It is the M1
hands-on exercise and establishes the workload that later modules harden."

---

## Business Requirements

**Instructions**: What this must accomplish in outcome terms, independent of
how it is built. For a training module these are the learning objectives plus
the production-realism bar the work is held to. Group related requirements
under subheadings. One clear sentence each. Focus on "what", not "how".

**Example**:

```text
### Learning Objectives
- I can explain how a Deployment differs from an ECS Service and what the
  Deployment controller does that ECS does for you.
- I can describe what a K8s Service provides that an ALB target group does.

### Production Bar
- The workload declares resource requests and limits.
- The workload defines readiness and liveness probes.
- Rollout is zero-downtime and demonstrably reversible.
```

---

## Technical Requirements

**Instructions**: Describe how the feature will be built. Include the
Kubernetes objects, Go components, and observability signals involved. Use
concrete manifests, paths, and commands where they clarify.

### Kubernetes Resources

**Instructions** (mark **Not applicable** for Go-only or tooling-only specs):
List every object this creates or modifies. For each, include kind, name,
namespace, and the fields that carry real intent — replicas, resources, probes,
selectors, service type, ports.

**Example**:

```yaml
# Deployment/flashcards in namespace flashcards
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: flashcards
          resources:
            requests: { cpu: 50m, memory: 64Mi }
            limits:   { cpu: 500m, memory: 256Mi }
          readinessProbe:
            httpGet: { path: /healthz, port: 8080 }
```

### Go Components

**Instructions** (mark **Not applicable** for manifest-only specs): List the
packages, types, and functions added or changed, and the behavior each owns.
Note anything that talks to the Kubernetes API and which client it uses.

**Example**:

```text
- `internal/health/health.go` — /healthz and /readyz handlers; readiness
  flips false on SIGTERM so the endpoint drains before the Pod dies.
- `cmd/flashcards/main.go` — wire graceful shutdown with a 15s timeout.
```

### Observability

**Instructions** (mark **Not applicable** only when nothing long-running is
added or changed): State what signals the finished thing emits and how they are
seen. Anything long-running — a service, endpoint, or controller — ships with
structured logs and metrics. Name the metric names, log fields, and where they
surface. See the `golang-observability` skill for the Go-side patterns.

**Example**:

```text
- Metrics: `flashcards_http_requests_total{method,path,status}` and
  `flashcards_http_request_duration_seconds` on :8080/metrics.
- Logs: slog JSON to stdout with `request_id`, `method`, `path`, `status`.
- Seen via: `kubectl port-forward` + curl in M1; scraped by Prometheus and
  charted in Grafana from M4 on.
```

---

## Implementation Phases

**Instructions**: Break the work into phases. Each phase gets a clear objective,
a checkbox task list, deliverables, and a status marker. Order phases by real
dependency — if two phases can run in either order, they are probably one phase.

Every phase follows test-first ordering: add or update the failing test, make it
pass, then harden. Every phase ends with a documentation sync task; the spec
cannot be marked complete until affected docs are updated and listed here.

Update status markers as work progresses.

**Format**:

```text
### Phase 1: [Phase Name] — ⏳ PLANNED

**Objective**: [What this phase achieves]

**Tasks**:

- [ ] Add or update failing tests for [behavior] — `path/to/thing_test.go`
- [ ] Implement [behavior] until the tests pass
- [ ] Refactor or harden [behavior] while keeping tests green
- [ ] Sync affected documentation with the implemented changes

**Deliverables**:

- File or manifest created
- Behavior implemented and verified
- Documentation updated, listed by path
```

---

## Test-Driven Development Requirements

**Instructions**: Define the expected TDD workflow before implementation begins.
Each behavior change is driven by a failing test first, then the smallest
implementation that passes it, then any refactor that keeps the suite green.
See the `golang-testing` skill for Go test structure and conventions.

**Required TDD Tasks**:

- Identify the test files, cases, fixtures, and smoke checks needed per phase.
- Add or update tests before changing production code for the behavior at hand.
- Run the new tests and record the expected failure before implementing, where
  practical.
- Implement the smallest change that satisfies the failing tests.
- Run focused tests after each change, and `make check` before marking complete.
- Document any exception where a failing test cannot practically come first,
  including the reason and the verification used instead.

For manifests and charts, the analog to a unit test is
`kubectl apply --dry-run=server`, `helm template`, or `kubeconform`. Use it
rather than skipping verification.

**Format**:

```text
### TDD Plan

- Phase 1 tests: `internal/health/health_test.go` validates readiness flips
  false on SIGTERM.
- Phase 2 tests: `internal/web/web_test.go` validates /metrics is served.
- Manifest checks: `kubectl apply -k manifests/ --dry-run=server`
- Regression: `make check`

### TDD Exceptions

- None.
```

---

## Technical Implementation Details

**Instructions**: Capture the patterns and gotchas an implementer needs. Fill
this in as the code is written, not before.

**Format**:

```text
### Key Files

- `manifests/deployment.yaml` — workload spec, probes, resources
- `internal/health/health.go` — readiness/liveness handlers

### Implementation Patterns

Describe the pattern in prose, with a short code or YAML excerpt where it
carries more than words do.

### Important Notes

- Critical gotchas and why they bite
- ECS/Fargate contrast worth remembering
- Known limitations of this approach
```

---

## Success Criteria

**Instructions**: Observable statements with a clear pass/fail, as checkboxes.
Not a restatement of the task list — this is how you know the goal was reached.
Include the exact command that demonstrates each where one exists.

**Example**:

```text
- [ ] `kubectl get deploy flashcards` shows 3/3 ready
- [ ] `kubectl rollout restart deploy/flashcards` completes with no 5xx
      observed by a concurrent curl loop
- [ ] Every container declares requests, limits, and both probes
- [ ] `make check` passes
```

---

## Troubleshooting Guide

**Instructions**: Document problems actually hit during this work, with their
fix. Add entries as they are encountered — do not speculate ahead of time.
Mark **Not applicable** while empty.

**Format**:

```text
### ImagePullBackOff on a locally built image

**Problem**: Pods stay in ImagePullBackOff despite the image existing locally.
**Cause**: KIND nodes have their own containerd store; the host's Docker
images are not visible to them.
**Solution**: `kind load docker-image flashcards:dev --name <cluster>`, and set
`imagePullPolicy: IfNotPresent`.
**Reference**: `modules/00-setup/README.md`
```

---

## Future Enhancements

**Instructions**: Out-of-scope ideas worth remembering, kept brief. These are
explicitly not built by this spec. Mark **Not applicable** when there are none.

**Example**:

```text
- Package as a Helm chart (lands in M3)
- Add PodDisruptionBudget and topology spread constraints
```

---

## Dependencies

**Instructions**: What must exist for this to work.

**Format**:

```text
### External Dependencies

- KIND cluster from `modules/00-setup/kind-cluster.yaml` — the target cluster
- ingress-nginx — Ingress admission (M3 onward)

### Internal Dependencies

- `flashcards/` image built via `make image` — the workload under deployment
- `docs/curriculum.md` M1 section — the objectives this spec implements
```

---

## Risks and Mitigation

**Instructions**: Identify problems specific to this work and how to prevent or
handle them. Skip generic risks that restate obvious tradeoffs.

**Format**:

```text
### Technical Risks

- **Risk**: Probes tuned too aggressively cause restart loops under load.
- **Mitigation**: Set initialDelaySeconds from measured startup time; verify
  with `kubectl describe pod` restart counts after a load run.

### Learning Risks

- **Risk**: Copying manifests without understanding the selector/label
  relationship, which breaks silently later.
- **Mitigation**: Delete the Service selector and observe the failure before
  fixing it.
```

---

## Notes for AI Agents

When implementing against this spec:

1. Tick each checkbox as that item actually completes — not in a batch at the
   end, never for work that was skipped or deferred. A checked box means the
   artifact exists and its verification passed.
2. Update the phase status markers and the top **Status** / **Last updated**
   fields in the same change as the work they describe.
3. Enforce TDD: write or update tests before production code for each behavior
   change, record the focused test commands and results, and document any
   exception under **TDD Exceptions**.
4. Add **Technical Implementation Details** and **Troubleshooting** entries as
   code is written and bugs are found — not up front.
5. If the work reveals the spec is wrong, fix the spec first, then implement.
   Never let the code and the spec disagree silently.
6. Delete superseded text rather than accumulating it; git holds the history.
7. Do not mark this spec complete until the documentation sync task is done and
   the affected docs are listed in the phase deliverables.
8. Cite code as `path/to/file.go:42`.
