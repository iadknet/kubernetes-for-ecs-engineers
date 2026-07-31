# Progress

Living tracker for the training program. Update the status and date as each
module is completed. Statuses: `not started` · `in progress` · `done`.

| # | Module | Status | Completed | Notes |
|---|--------|--------|-----------|-------|
| M0 | Local production-shaped cluster | in progress | — | Start here → `modules/00-setup/README.md` |
| M1 | Core workloads (Pods/Deployments/Services) | not started | — | |
| M2 | Config, Secrets, Namespaces, RBAC | not started | — | Secrets backed by self-hosted Vault + ESO |
| M3 | Production concerns (limits, probes, HPA, PDB, Helm, Ingress) | not started | — | |
| M4 | Observability (Prometheus/Grafana/Loki) | not started | — | |
| M5 | K8s development in Go (client-go, informers) | not started | — | |
| M6 | CRDs + controllers (operator pattern) | not started | — | |
| M7 | Cluster authentication (certs, SA tokens, OIDC) | not started | — | Deliberate deep dive on what M2 uses as a tool |
| CAP | Capstone: Teleport SRE take-home (L1→L5) | not started | — | |

## Log

- **2026-07-31** — Built **[flashcards/](../flashcards/)**: 255 ECS-anchored
  cards across 12 decks (M0–M7 + capstone + acronyms, kubectl↔ECS command
  mapping, and EKS-specific knowledge), served by a small Go + htmx app with
  FSRS spaced repetition. Runs locally and as a ~19 MB distroless container.
  It doubles as the program's **example workload** — from M1 on we deploy and
  harden this instead of a stock nginx image, so each module has something real
  to apply itself to. Not yet on the cluster: manifests land with M1.
- **2026-07-31** — Constraint added: **no AWS account available**. AWS stays the
  teaching anchor and interview vocabulary, but no exercise may depend on it.
  Added a substitution table to the README (Vault + ESO for Secrets Manager,
  ingress-nginx for ALB, Prometheus/Grafana/Loki for CloudWatch, `kind load` for
  ECR). M2 now explicitly builds on self-hosted Vault.
- **2026-07-30** — Program scaffolded. Chosen setup: Docker Desktop + KIND on
  Apple Silicon macOS. Approach: guided build-up to capstone. M0 authored and
  started.
