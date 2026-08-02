# Flashcards

A spaced-repetition drill for Kubernetes concepts, vocabulary, and acronyms —
**327 cards** across 13 decks, every one of them anchored to its AWS ECS/Fargate
equivalent (or explicitly flagged as having none). The first deck is a
**glossary tier**: 72 cards teaching one term apiece, which every other card
declares a dependency on.

It is also the training program's **example workload**. Rather than deploying a
stock nginx image in M1–M4, you deploy this, and harden it module by module as
the curriculum reaches each concept. See [the backlog](#the-hardening-backlog).

## Run it

```bash
make run          # http://localhost:8080
make run-chat     # ...with the Claude chat panel (see below)
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

A card is **locked** until every term it requires has reached FSRS `Review`
state — retained, not merely seen. A filtered drill pulls in the terms it needs,
so `?module=M0` teaches M0's vocabulary as part of M0 and the scope line names
how many terms it added. The dashboard's *locked* column is the backlog behind
vocabulary you haven't learned yet; it drains as the glossary lands. Cram mode
ignores locking entirely, on purpose — it is the night-before escape hatch.

Only 40 unseen cards are introduced per day by default (`NEW_PER_DAY`), because
starting 327 new cards at once is how a review queue becomes unusable by Friday.

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | Listen port |
| `DATA_DIR` | `./data` | Where `review.json` is persisted |
| `DECKS_DIR` | *(unset)* | Read decks from disk instead of the embedded copy |
| `NEW_PER_DAY` | `40` | Cap on newly introduced cards per day |
| `CHAT_ENABLED` | `false` | Serve the drill view's [chat panel](#asking-claude-about-a-card) |
| `CHAT_PROVIDER` | `claude-cli` | Which chat backend to build |

`DECKS_DIR` exists for a specific exercise: in M2 you mount the decks from a
**ConfigMap** and watch config decouple from the image.

Each chat provider reads its own configuration, so the table above is every
variable the *command* knows. The `claude-cli` provider adds two:

| Env var | Default | Purpose |
|---|---|---|
| `CLAUDE_BIN` | `claude` | Path to the Claude Code CLI, resolved on `PATH` |
| `CHAT_MODEL` | `sonnet` | Model used when a request names none |

Both are validated at startup: a missing binary or a model the CLI does not
offer exits non-zero rather than leaving a panel that breaks on first use.

## Asking Claude about a card

```bash
make run-chat                     # http://localhost:8080, panel on the drill page
make run-chat CHAT_MODEL=opus     # or pick the model up front
```

Ask "why would I use this instead of an ECS service?" without leaving the drill;
the conversation spans cards, so follow-ups like "how does that relate to the
previous card?" work.

Answers come from the **Claude Code CLI on your machine** (`claude --print`),
inheriting `AGENTS.md` and the `k8s-for-ecs-engineers` skill — which is why they
come back in ECS framing. Auth is the CLI's own login state, so the app never
sees a credential. The subprocess gets `Read`, `Grep`, and `Glob` and nothing
else: it cannot edit or run anything.

`run-chat` is a separate target from `run` for two reasons. It needs a
logged-in `claude` CLI, which `make run` should not require; and it starts the
server from the **repo root**, because the subprocess inherits the server's
working directory and only at the root does the CLI resolve the repo's skills
and expand `CLAUDE.md`'s `@AGENTS.md` import. Run it from `flashcards/` and the
answers quietly lose all of that.

Two deliberate limits:

- **The panel explains around a card, never through it.** Ask the card's own
  question before flipping and it tells you what to reason from, not the answer.
  A drill whose answers are one question away is just reading.
- **Local only, and that takes more than a loopback check.** The server binds
  every interface so a port-forward works, but the chat routes answer this
  machine alone. A loopback check by itself would not be enough: your browser is
  on loopback too, so any page you visit could POST to `127.0.0.1:8080` and look
  like the panel. The routes also require a JSON content type — which a
  forgeable cross-origin request cannot send — reject a foreign `Origin` or
  `Sec-Fetch-Site`, and require a loopback `Host` so a domain rebound to
  `127.0.0.1` is caught.

It is **off in the cluster and belongs to no module**: no manifest sets
`CHAT_ENABLED`, and with it unset the binary registers no chat route and renders
no panel markup, CSS, or script. The provider interface is the interesting part
for the curriculum — `internal/web` depends on `internal/chat` and never on
`internal/chat/claudecli`, so an API-key-driven provider that *could* run in a
container is a new implementation plus a `CHAT_PROVIDER` value.

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
    requires: [term-service, term-deployment]   # optional, glossary card ids

  - id: term-service                # a glossary card: it teaches one term
    term: Service                   # the term; makes this the card that owns it
    aliases: [Services]             # other spellings that count as the same term
    q: |
      Service
    a: |
      A stable name and virtual IP in front of a changing set of Pods.
```

Three rules worth knowing:

- **`id` is the primary key for review history.** Rename one and that card's
  scheduling resets to new. Fix a typo in the text freely; leave the id alone.
- **A block scalar's first line sets its indentation.** An answer that *starts*
  with an indented code block breaks the YAML parse. Use a fenced ``` block
  instead — `make test` catches this, since the test suite parses every
  real deck and renders every real card through the templates.
- **Use a glossary term, require it.** `make lint-decks` fails any card whose
  `q` or `a` uses a term it neither requires nor defines. Decks not yet cleaned
  up are listed in the allowlist in `internal/deck/glossary_test.go`, and
  draining a deck's entry is part of reaching that module.

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
    ├── chat/                 provider-neutral chat interface
    │   └── claudecli/        the one provider: the Claude Code CLI, headless
    ├── deck/                 parse + validate + filter
    ├── review/               FSRS scheduling + atomic JSON store
    └── web/                  handlers, html/template, vendored htmx
```
