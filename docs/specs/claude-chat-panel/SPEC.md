# Claude Chat Panel — Technical Spec

**Status**: ⏳ PLANNED
**Last updated**: 2026-08-01

---

## Overview

A chat side panel in the drill view lets you ask Claude about the current card
without switching to Claude Code. The panel is backed by the `claude` CLI in
headless mode (`claude -p`), running on the local Claude subscription — no API
key, no per-token billing. It is a local-only developer feature of the
flashcards workload, gated off by default so the deployed artifact is unchanged;
it belongs to no module.

The Claude Agent SDK is not an option here: Anthropic permits subscription auth
for the CLI itself but requires API keys for Agent SDK integrations. Wrapping
the CLI as a subprocess is the supported path.

---

## Business Requirements

### Learning Objectives

- I can ask "explain this card" mid-drill and get an answer grounded in the
  card's content and the repo's `k8s-for-ecs-engineers` teaching rules (ECS
  analogies, production realism), because the subprocess runs inside this repo
  and inherits `AGENTS.md` and the repo skills.
- Follow-up questions keep their context: a chat session spans cards, so "how
  does this relate to the previous card?" works.

### Production Bar

- Off by default. Without `CHAT_ENABLED=true` the binary serves no chat route,
  ships no panel markup, and behaves byte-for-byte like today. The Dockerfile
  and every manifest need no change.
- Fail fast on misconfiguration: `CHAT_ENABLED=true` with no `claude` binary on
  `PATH` (or at `CLAUDE_BIN`) is a startup error, not a broken panel at runtime.
- No secrets touch the app: auth is the CLI's own login state. The app never
  reads, stores, or forwards a credential.
- Claude gets read-only tools (`Read`, `Grep`, `Glob`) — the chat can cite the
  curriculum and decks but cannot edit or execute anything.
- Chat endpoints answer loopback clients only. The server binds all interfaces
  (`:PORT`); the chat handlers must not let anyone else on the network spend the
  subscription.

---

## Technical Requirements

### Kubernetes Resources

**Not applicable.** The feature is compile-time present but runtime-disabled in
the cluster; no manifest sets `CHAT_ENABLED`.

### Go Components

```text
- `cmd/flashcards/main.go` — read CHAT_ENABLED (default false), CLAUDE_BIN
  (default "claude"), and CHAT_MODEL (default "sonnet" — the panel's default
  model selection) via env helpers, envBool being new; when enabled, resolve
  the binary with exec.LookPath at startup and fail run() if missing; document
  the vars in the package comment; pass a web.Config into web.New.
- `internal/chat/chat.go` — Runner: spawns the CLI per turn with
  `-p <prompt> --output-format stream-json --verbose
  --include-partial-messages --append-system-prompt <tutoring instructions>
  --tools "Read,Grep,Glob" --model <model> [--effort <effort>]`, cwd = the
  server's working directory, kills the subprocess when the request context is
  cancelled, parses the NDJSON event stream into text deltas + the session id,
  and passes `--resume <session id>` on every turn after the first. Model and
  effort are per-turn parameters: `--model` overrides a resumed session's
  model, so switching mid-session needs no reset; `--effort` is omitted when
  unset, leaving the model default.
- `internal/web/web.go` — web.New gains a Config{Chat *chat.Runner, ChatModel
  string}; Routes registers POST /chat and POST /chat/reset only when Chat is
  non-nil; both handlers reject non-loopback RemoteAddr with 403. POST /chat
  takes the user message, the current card id, and optional model/effort
  fields validated against fixed allowlists (models: sonnet, opus, haiku —
  the CLI offers no programmatic listing, so the list is hard-coded; efforts:
  low, medium, high, xhigh, max, or empty for model default); unknown values
  → 400. The handler prepends a card-context block (deck, front, back,
  requires terms) to the prompt and relays text deltas to the client as an
  SSE stream on the POST response.
- `internal/web/templates/drill.html` + `static/` — panel markup with model
  and effort selectors above the chat box (model preselected from CHAT_MODEL,
  effort defaulting to "model default"), sent with each message; plus a small
  vanilla-JS client (fetch + ReadableStream; no htmx SSE extension — it is a
  CDN dependency and the app rule is no CDN), rendered only when chat is
  enabled.
```

Session model: this is a single-user app, so the Runner holds one current
session id in memory. `/chat/reset` clears it; a server restart does too.

### Observability

- Logs: one slog line per turn — `session_id`, `card`, `dur`, `exit_code`; one
  per subprocess failure with stderr tail. Surfaced on stdout like all existing
  logs.
- Metrics: none — the app exposes no metrics endpoint yet (arrives M4), and a
  disabled-in-cluster feature does not justify starting one.

---

## Implementation Phases

### Phase 1: Flag, validation, and gating — ⏳ PLANNED

**Objective**: The feature can be switched on and off, and misconfiguration
fails at boot.

**Tasks**:

- [ ] Add failing tests: envBool parsing; Routes without chat serves 404 on
      /chat and drill HTML contains no panel markup — `internal/web/web_test.go`
- [ ] Add envBool, CHAT_ENABLED/CLAUDE_BIN/CHAT_MODEL wiring, and the LookPath
      startup check in `cmd/flashcards/main.go`; thread web.Config through
      web.New
- [ ] Sync `flashcards/README.md` and the main.go package comment with the new
      env vars

**Deliverables**:

- `CHAT_ENABLED=true` without a binary exits non-zero with a clear error
- Default build is unchanged (no route, no markup)
- `flashcards/README.md` updated

### Phase 2: CLI runner — ⏳ PLANNED

**Objective**: `internal/chat` can run a turn against a stub binary and resume
the session on the next turn.

**Tasks**:

- [ ] Add failing tests against a stub script standing in for CLAUDE_BIN that
      emits canned stream-json: deltas surface in order; session id is captured;
      second turn passes --resume; --model is passed every turn and --effort
      only when set; context cancellation kills the process; non-zero exit
      returns an error carrying the stderr tail — `internal/chat/chat_test.go`
- [ ] Implement the Runner until the tests pass
- [ ] Sync docs: none expected beyond package doc comments

**Deliverables**:

- `internal/chat` package with stub-driven tests; no network or real
  subscription use in `make check`

### Phase 3: Endpoint and panel — ⏳ PLANNED

**Objective**: A question typed in the drill view streams an answer grounded in
the current card.

**Tasks**:

- [ ] Add failing tests with a fake runner: POST /chat streams SSE and the
      prompt contains the card's front/back/requires; unknown card id, model,
      or effort → 400; the request's model/effort reach the runner, with
      CHAT_MODEL as the default when omitted; non-loopback RemoteAddr → 403;
      /chat/reset clears the session — `internal/web/web_test.go`
- [ ] Implement the handlers, card-context block, tutoring system-prompt text,
      and the SSE relay (clear the write deadline per request — see
      Implementation Details)
- [ ] Add the panel markup, model/effort selectors, and JS client to
      drill.html/static
- [ ] Manual verification against the real CLI: ask about a glossary card and
      confirm the answer uses ECS framing
- [ ] Sync `flashcards/README.md` (usage section) and this spec's status

**Deliverables**:

- Working panel end-to-end on a logged-in laptop
- `flashcards/README.md` updated

---

## Test-Driven Development Requirements

### TDD Plan

- Phase 1 tests: `internal/web/web_test.go` (gating), `cmd` covered by the
  existing pattern of env helpers being exercised at startup — the LookPath
  failure is verified by running `run()` with a bogus CLAUDE_BIN.
- Phase 2 tests: `internal/chat/chat_test.go` with a stub binary (a shell
  script written to t.TempDir()) — the seam that keeps `make check` free of
  network, auth, and quota use.
- Phase 3 tests: `internal/web/web_test.go` with a fake runner interface.
- Regression: `make check` before each phase is marked complete.

### TDD Exceptions

- The final "real CLI" verification in Phase 3 is manual: it depends on a
  logged-in subscription and burns quota, so it cannot run in `make check`.
  Recorded as a checkbox with the observed transcript summarized in this spec.

---

## Technical Implementation Details

*Fill in as code is written.* One known pattern up front:

### SSE vs server timeouts

`http.Server` in `cmd/flashcards/main.go` sets `WriteTimeout: 30s`, which
applies to the whole response and would cut off any chat turn longer than 30s.
The chat handler must clear it per request via
`http.NewResponseController(w).SetWriteDeadline(time.Time{})` rather than
loosening the global timeout for every route.

---

## Success Criteria

- [ ] With `CHAT_ENABLED` unset: `curl -i localhost:8080/chat` → 404, and the
      drill page HTML contains no chat markup
- [ ] `CHAT_ENABLED=true CLAUDE_BIN=/nonexistent ./flashcards` exits non-zero
      naming the missing binary
- [ ] With the stub binary, `go test ./internal/chat ./internal/web` proves
      streaming, resume, cancellation, and the loopback guard
- [ ] On a logged-in laptop: a question about the current card streams an
      answer that uses the card's content and an ECS analogy, and a follow-up
      question shows the session resumed (same session id logged)
- [ ] Switching the model selector between two turns keeps the same session id
      while the turn log shows the new model
- [ ] `make check` passes
- [ ] Docker image builds unchanged and serves the app with no chat surface

---

## Troubleshooting Guide

**Not applicable** — populated as problems are hit.

---

## Future Enhancements

- Persist chat transcripts alongside review state so a session survives restart
- A "explain why my answer was wrong" quick action that sends the graded answer
  automatically

---

## Dependencies

### External Dependencies

- Claude Code CLI installed and logged in (subscription auth) on the machine
  where `CHAT_ENABLED=true` — headless mode, stream-json output, `--resume`,
  `--append-system-prompt`, `--model`, and `--effort` per its CLI reference
  (`--effort` needs a recent release, v2.1.205+)
- This repo checked out: the subprocess inherits `AGENTS.md` and
  `.claude/skills/k8s-for-ecs-engineers` by running inside it

### Internal Dependencies

- `internal/deck.Library` — card lookup for the context block
- `internal/web` drill view — the panel's host page

---

## Risks and Mitigation

### Technical Risks

- **Risk**: The CLI's stream-json event shape changes across Claude Code
  releases and silently breaks parsing.
- **Mitigation**: Parse only the fields we consume (type, text delta, session
  id); on unparseable output, fail the turn with the raw tail in the error so
  the log shows what changed. The stub-binary fixtures document the shape we
  depend on.

- **Risk**: Chat endpoints reachable from the network let others spend the
  subscription and read repo files.
- **Mitigation**: Loopback-only guard on the chat handlers, tested; feature off
  by default everywhere else.

### Learning Risks

- **Risk**: Asking the panel instead of attempting recall turns drills into
  reading.
- **Mitigation**: The panel lives on the drill page but never reveals or grades
  the card itself; the tutoring system prompt instructs Claude to explain, not
  to answer the card before the user has flipped it.

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
