# Target Roles

This file grounds the training curriculum in real job postings the user is
applying to. Training exercises, priorities, and "what to learn next"
decisions in this project should trace back to a concrete requirement below
rather than generic Kubernetes trivia. Update this file as new target roles
are added or postings change.

## Company: Teleport (goteleport.com)

Remote-first infrastructure identity/access company, product written in Go,
built on Kubernetes. Two open roles are currently in scope:

### 1. Senior Site Reliability Engineer - US
<https://jobs.ashbyhq.com/goteleport/1ceb2d28-1745-4d0d-b21e-3fa88be9b996>

Building Teleport Cloud (SaaS) — production/SaaS infrastructure from scratch.

Requirements:
- 5+ years progressive Software Engineering and/or SRE/DevOps experience
- Strong Linux systems, networking, containers, troubleshooting
- Solid **Go** and **Kubernetes development** experience (not just operating
  clusters — writing Go code that interacts with Kubernetes)
- Scripting/automation/tooling experience, including AI-agent-assisted
  operational workflows
- AWS preferred, GCP acceptable
- Observability: **Prometheus, Grafana, Loki**
- On-call, 24/7/365 rotation
- Traditional ops: patching, scaling, backup/restore, disaster recovery
- Reasoning about correctness/system invariants (formal or property-based
  methods a plus)

What they'd actually do: re-engineer core Teleport for global scale/routing
latency, build monitoring/observability, eliminate toil via automation,
investigate customer-facing outages/incidents.

**Interview gate: a take-home Go + Kubernetes coding challenge** — see
"SRE Kubernetes Challenge" below. This is the single most concrete target
for hands-on exercises in this project.

### 2. IT Security and Automation Engineer
<https://jobs.ashbyhq.com/goteleport/62176999-281a-4975-8464-e8217582caf4>

More IT/infra-automation flavored than the SRE role, but still requires
diving into K8s and Go + Temporal.

Requirements:
- Go programming; Temporal experience a bonus
- Linux systems engineering, **Kubernetes**
- IaC (**Terraform**) and shell scripting
- IT security & administration experience
- Comfort with traditional IT helpdesk/tooling work (Okta, Rippling,
  Zendesk, Jamf Pro, Google Chrome Enterprise, Panther)
- Nice to have: data warehousing (Redshift, Fivetran, DBT)

Projects mentioned: next-gen IT workflow automation using **Temporal + Go
running on Kubernetes**; hardened macOS endpoint security automation.

**Interview gate: a take-home challenge using Terraform and shell
scripts (or Go)** — the general "Security and Automation" challenge (see
below) is the closest public analog.

## SRE Kubernetes Challenge — the concrete curriculum target

Source: <https://github.com/gravitational/careers/blob/main/challenges/sre/challenge.md>

Build a **Go server that interacts with a Kubernetes cluster**, deployable
to a local cluster (KIND suggested, must work on macOS and Linux). This
challenge is leveled 1-5, and the leveling itself is a useful skill
checklist — each level is a strict superset of the one before:

- **Level 1** — HTTP API to *read* a Deployment's replica count. Dockerfile.
  Deploy to K8s manually per docs. Basic happy/unhappy-path tests.
- **Level 2** — Add HTTP API to *set* replica count. Add integration tests
  that run against a real local cluster (not just mocks).
- **Level 3** — Add HTTP API to list all Deployments in the cluster. Add an
  HTTP health check that verifies actual Kubernetes API connectivity (not
  just "process is up"). Package as a **Helm chart** (Deployment,
  ServiceAccount, Service at minimum). Helm upgrades must not cause
  service unavailability (zero-downtime rollout).
- **Level 4** — Replace naive per-request API calls with a **watch-based
  cache** (client-go informer or controller-runtime) so reads don't hit the
  Kubernetes API server every time. Secure the HTTP API with **mTLS**.
  `make`-driven build/deploy/test workflow. Configurable Helm chart.
- **Level 5** — Same functionality over **gRPC** instead of HTTP. Implement
  an actual **CRD + controller** that reconciles desired state (stored in
  the CRD) against the live Deployment — i.e., write a real Kubernetes
  operator, not just a client. Production-grade Helm packaging: Deployment,
  Role, RoleBinding, ServiceAccount, Service.

Skills this challenge is designed to test, and therefore skills worth
deliberately practicing in this project:
- `client-go` and/or `controller-runtime` (informers/watches, not polling)
- Writing a CRD and a reconciliation-loop controller (the core Kubernetes
  operator pattern — genuinely new vs. ECS, see the
  `k8s-for-ecs-engineers` skill's "no ECS equivalent" section)
- Helm chart authoring, including safe/zero-downtime upgrades
- mTLS between services
- gRPC API design and protobuf
- Local cluster tooling (KIND), Dockerfile authoring, `make`-based workflows
- Integration testing against a real (local) cluster, not just unit-testing
  with mocks
- Design-doc-first workflow: they explicitly want a short written design
  doc (API structure, pod lifecycle, TLS config, dev workflow) reviewed
  *before* code — practice writing these, not just the code

Explicit non-goals per their own guidance (don't over-build): no need for a
shared cache, multi-region/multi-AZ deployment, or gold-plated config
systems for this specific exercise — hardcode and TODO instead. This is a
useful calibration signal: production-realism (per this project's other
guidance) is about correctness and honesty regarding gaps, not maximal
scope on every exercise.

## Security and Automation Challenge (relevant to the IT Security role)

Source: <https://github.com/gravitational/careers/blob/main/challenges/security-automation/challenge.md>

Build workflow automation that configures an Auth0 tenant, adds a web app,
and registers users — via **Terraform**, triggered by **GitHub Actions**
(Level 3) or a **Temporal** Go workflow + CLI (Level 4). Less
Kubernetes-specific, but reinforces: IaC with Terraform, secure credential
storage/scoping, OIDC-based app auth, and Temporal as a workflow engine —
all called out in the IT Security job posting itself.

## How to use this file

- When proposing the next training exercise, prefer one that maps to an
  unchecked item above over a generic tutorial topic.
- When the user's Kubernetes work in this project resembles part of the SRE
  challenge (a Go client of the K8s API, a Helm chart, a controller), treat
  it as practice for that actual interview challenge and hold it to the
  challenge's own bar (tests for happy/unhappy paths, design-doc-first,
  avoid scope creep) rather than a lower "just get it running" bar.
- Both roles want K8s *development* (writing Go/controllers against the
  API), not just K8s *operations* (kubectl, YAML). Weight exercises
  accordingly — this is a different emphasis than most "learn Kubernetes"
  material, which is operations-heavy.
