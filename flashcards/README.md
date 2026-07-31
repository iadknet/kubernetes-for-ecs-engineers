# Flashcards

A spaced-repetition drill for Kubernetes concepts, vocabulary, and acronyms —
**255 cards** across 12 decks, every one of them anchored to its AWS ECS/Fargate
equivalent (or explicitly flagged as having none).

It is also the training program's **example workload**. Rather than deploying a
stock nginx image in M1–M4, you deploy this, and harden it module by module as
the curriculum reaches each concept. See [the backlog](#the-hardening-backlog).

## Run it

```bash
make run          # http://localhost:8080
make test
make image        # ~19 MB distroless image
make docker-run   # run the container, review state in ./data
```

Nothing to install but Go — no npm, no build step, no CDN. htmx is vendored at
`internal/web/static/`, so it works offline and inside a locked-down cluster.

## How the drilling works

Cards are scheduled with [FSRS](https://github.com/open-spaced-repetition/go-fsrs),
the algorithm Anki uses. You grade yourself **Again / Hard / Good / Easy** (keys
`1`–`4`; space reveals the answer) and the scheduler decides when each card comes
back — minutes for *Again*, days or weeks for *Easy*.

- `/` — dashboard: what's due per deck, streak, tag filters
- `/drill` — the drill loop; `?module=M3`, `?deck=07`, `?tag=rbac` narrow it
- `/drill?cram=1` — ignore scheduling and drill everything (the night before an
  interview)
- `/browse` — every card as a study sheet, no scheduling

Only 20 unseen cards are introduced per day by default (`NEW_PER_DAY`), because
starting 255 new cards at once is how a review queue becomes unusable by Friday.

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | Listen port |
| `DATA_DIR` | `./data` | Where `review.json` is persisted |
| `DECKS_DIR` | *(unset)* | Read decks from disk instead of the embedded copy |
| `NEW_PER_DAY` | `20` | Cap on newly introduced cards per day |

`DECKS_DIR` exists for a specific exercise: in M2 you mount the decks from a
**ConfigMap** and watch config decouple from the image.

## Writing cards

Decks are `decks/*.yaml`. The schema is deliberately small:

```yaml
deck: Core workloads — Pods, Deployments, Services   # required
module: M1                                            # optional, enables --module
tags: [workloads]                                     # optional, applied to every card

cards:
  - id: m1-service-collision      # required, must be globally unique and stable
    q: |                          # required, markdown
      Why is "Service" the most confusing word coming from ECS?
    a: |                          # required, markdown
      It names a *networking* object, not a replica manager.
    ecs: |                        # optional — omit when there is no ECS analog
      ECS Service ≈ K8s **Deployment**. K8s **Service** ≈ the ECS Service's
      load-balancer wiring.
    tags: [service, gotcha]
```

Two rules worth knowing:

- **`id` is the primary key for review history.** Rename one and that card's
  scheduling resets to new. Fix a typo in the text freely; leave the id alone.
- **A block scalar's first line sets its indentation.** An answer that *starts*
  with an indented code block breaks the YAML parse. Use a fenced ``` block
  instead — `make test` catches this, since the test suite parses every
  real deck and renders every real card through the templates.

## What's deliberately missing

The app is intentionally unhardened right now. It has exactly the production
shape that M0 material has earned, and no more. Two things it *does* already
get right, because retrofitting them teaches the wrong lesson:

- **`/healthz` and `/readyz` are separate.** Liveness checks only "is this
  process wedged"; readiness checks decks loaded *and* review state writable. A
  liveness probe that checked a dependency would turn one outage into a
  cluster-wide restart storm.
- **SIGTERM drains gracefully**, which is what makes rolling updates
  non-disruptive later.

Review state is a plain JSON file, and that will fail instructively: deploy this
on Kubernetes with no volume and your history vanishes the first time the Pod
reschedules. That's the ECS-familiar "tasks are ephemeral" lesson delivered
firsthand, and the reason M3 adds a PVC.

## The hardening backlog

Each module adds a layer to this same app:

| Module | What gets added |
|---|---|
| **M1** | Deployment + Service, `make kind-load`, port-forward, rolling update, rollback |
| **M2** | Decks from a ConfigMap (`DECKS_DIR`), own namespace, least-privilege ServiceAccount |
| **M3** | requests/limits, wire up the probes, PDB, HPA, PVC for review state, Helm chart, Ingress |
| **M4** | `/metrics` via `client_golang`, Grafana dashboard, one alert rule, logs in Loki |
| **M5** | The same binary starts talking to the Kubernetes API with client-go + informers |

## Layout

```
flashcards/
├── decks/*.yaml              the content — source of truth
├── embed.go                  go:embed of decks/ (must live at module root)
├── cmd/flashcards/main.go    config, timeouts, graceful shutdown
└── internal/
    ├── deck/                 parse + validate + filter
    ├── review/               FSRS scheduling + atomic JSON store
    └── web/                  handlers, html/template, vendored htmx
```
