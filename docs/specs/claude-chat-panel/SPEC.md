# Claude Chat Panel — Technical Spec

**Status**: ✅ COMPLETE
**Last updated**: 2026-08-01

---

## Overview

A chat side panel in the drill view lets you ask Claude about the current card
without switching to Claude Code. The panel talks to a chat *provider* behind a
small interface; the only provider built here wraps the `claude` CLI in
headless mode (`claude -p`), running on the local Claude subscription — no API
key, no per-token billing. That provider makes the feature local-only, and it
is gated off by default so the deployed artifact is unchanged; it belongs to
no module.

The interface exists so a future provider — API-key driven, containerizable,
possibly a different vendor entirely — is a new implementation plus config,
with no handler or UI change. Building that provider is out of scope.

The Claude Agent SDK is not an option for the CLI provider: Anthropic permits
subscription auth for the CLI itself but requires API keys for Agent SDK
integrations. Wrapping the CLI as a subprocess is the supported path.

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
- The web layer and UI depend only on the `chat.Provider` interface — no CLI
  flag names, model names, or claude-specific event shapes above the provider.
  Swapping providers is a new implementation selected by `CHAT_PROVIDER`.
- Fail fast on misconfiguration: an unknown `CHAT_PROVIDER`, or the claude-cli
  provider with no `claude` binary on `PATH` (or at `CLAUDE_BIN`), is a startup
  error, not a broken panel at runtime. Each provider owns its own startup
  validation.
- No secrets touch the app: the claude-cli provider's auth is the CLI's own
  login state. The app never reads, stores, or forwards a credential.
- Claude gets read-only tools (`Read`, `Grep`, `Glob`) — it cannot edit or
  execute anything. File reads are not sandboxed to the repo; acceptable
  because the loopback guard limits callers to the local user.
- Chat endpoints answer the local panel only. The server binds all interfaces
  (`:PORT`); the chat handlers must not let anyone else on the network spend the
  subscription. A loopback check on `RemoteAddr` is necessary but **not
  sufficient** — the user's own browser is on loopback, so any website can post
  to `127.0.0.1` and be indistinguishable from the panel. The handlers must also
  require a JSON content type (which a forgeable cross-origin request cannot
  send), reject a foreign `Origin` or `Sec-Fetch-Site`, and require a loopback
  `Host` so a domain rebound to `127.0.0.1` is caught.

---

## Technical Requirements

### Kubernetes Resources

**Not applicable.** The feature is compile-time present but runtime-disabled in
the cluster; no manifest sets `CHAT_ENABLED`.

### Go Components

```text
- `internal/chat/chat.go` — the provider-neutral surface, no claude imports:

  - `Provider` interface: `Send(ctx, Turn, emit func(delta string) error)
    error` streams one turn's text deltas; `Reset()` starts a fresh
    conversation; `Options() Options` reports capabilities.
  - `Turn` carries structured fields — `Message` (the user's question),
    `CardContext` (the card block: deck, front, back, requires terms —
    assembled by web, since it is flashcards domain knowledge),
    `SystemPrompt` (tutoring instructions), and the selected model/effort.
    Providers decide how to compose these onto their wire; the interface never
    carries a pre-flattened prompt.
  - `Options` carries `Models []string`, `DefaultModel string`, and
    `Efforts []string` (empty ⇒ the provider has no effort dial and the UI
    hides the selector). The web layer validates requests against these — no
    allowlists live above the provider.

  Conversation continuity is the provider's concern (the CLI provider holds a
  session id; an API provider would hold message history).
- `internal/chat/claudecli/claudecli.go` — the one Provider built here. Owns
  its configuration: the constructor reads CLAUDE_BIN (default "claude") and
  CHAT_MODEL (default "sonnet"), validates the binary with exec.LookPath and
  CHAT_MODEL against its model list, and fails construction on either.
  Composes the Turn as `--print <flags> <CardContext + Message>` — the prompt is
  the trailing positional argument, and one argv element, so no shell ever sees
  it — with `--output-format stream-json
  --verbose --include-partial-messages --append-system-prompt <SystemPrompt>
  --tools "Read,Grep,Glob" --model <model> [--effort <effort>]`, cwd = the
  server's working directory, kills the subprocess when the request context is
  cancelled, parses the NDJSON event stream into deltas + the session id, and
  passes `--resume <session id>` on every turn after the first. `--model`
  overrides a resumed session's model, so switching mid-session needs no
  reset; `--effort` is omitted when unset. Options reports models
  sonnet/opus/haiku (the CLI offers no programmatic listing, so the list is
  hard-coded here), default from CHAT_MODEL, efforts low/medium/high/xhigh/max.
- `cmd/flashcards/main.go` — reads only the provider-neutral vars: CHAT_ENABLED
  (default false, via a new envBool helper) and CHAT_PROVIDER (default
  "claude-cli", the only value; unknown → startup error). Constructs the
  selected provider when enabled and fails run() if construction fails.
  Provider-specific env vars (CLAUDE_BIN, CHAT_MODEL, and any a future
  provider adds) are read, validated, and documented by the provider package —
  the generic wiring never learns provider vocabulary. Pass a web.Config into
  web.New; document CHAT_ENABLED/CHAT_PROVIDER in the package comment.
- `internal/web/web.go` — web.New gains a Config{Chat chat.Provider}; Routes
  registers POST /chat and POST /chat/reset only when Chat is non-nil; both
  handlers reject non-loopback RemoteAddr with 403. POST /chat takes the user
  message, the current card id, and optional model/effort fields validated
  against the provider's Options (unknown values → 400), builds the Turn
  (CardContext from the card's deck/front/back/requires, the tutoring
  SystemPrompt, the message, model/effort), and relays deltas to the client
  as an SSE stream on the POST response.
- `internal/web/templates/drill.html` + `static/` — panel markup with model
  and effort selectors above the chat box, rendered from the provider's
  Options (default model preselected; effort selector omitted when the
  provider reports none), sent with each message; plus a small vanilla-JS
  client (fetch + ReadableStream; no htmx SSE extension — it is a CDN
  dependency and the app rule is no CDN), rendered only when chat is enabled.
```

Session model: this is a single-user app, so the provider holds one current
conversation in memory. `/chat/reset` clears it; a server restart does too.

### Observability

- Logs, split by ownership so the interface stays clean: web logs one line per
  turn with the generic fields — `card`, `model`, `effort`, `dur`, error if
  any. Each provider logs its own implementation detail from inside Send — for
  claude-cli that is `session_id`, `exit_code`, and a stderr tail on failure.
  All on stdout like existing logs.
- Metrics: none — the app exposes no metrics endpoint yet (arrives M4), and a
  disabled-in-cluster feature does not justify starting one.

---

## Implementation Phases

### Phase 1: Flag, validation, and gating — ✅ COMPLETE

**Objective**: The interface exists, the feature gates off cleanly, and an
unknown provider fails at boot. (Provider-specific validation — the missing
binary — lands with the provider in Phase 2.)

**Tasks**:

- [x] Add failing tests: envBool parsing; Routes without chat serves 404 on
      /chat and drill HTML contains no panel markup; CHAT_PROVIDER=bogus fails
      run() — `internal/web/web_test.go` and the cmd-level check
- [x] Define the Provider/Turn/Options types in `internal/chat`
- [x] Add envBool and CHAT_ENABLED/CHAT_PROVIDER wiring with provider selection
      in `cmd/flashcards/main.go`; thread web.Config through web.New
- [x] Sync `flashcards/README.md` and the main.go package comment with the new
      env vars

**Deliverables**:

- Default build is unchanged (no route, no markup)
- Unknown CHAT_PROVIDER exits non-zero with a clear error
- `flashcards/README.md` updated

### Phase 2: claude-cli provider — ✅ COMPLETE

**Objective**: `internal/chat/claudecli` implements Provider against a stub
binary, resuming the session on the next turn.

**Tasks**:

- [x] Add failing tests against a stub script standing in for CLAUDE_BIN that
      emits canned stream-json: deltas surface in order; session id is captured;
      second turn passes --resume; --model is passed every turn and --effort
      only when set; context cancellation kills the process; non-zero exit
      returns an error carrying the stderr tail; Options reports the documented
      models and efforts; the constructor rejects a missing binary and a
      CHAT_MODEL outside its model list —
      `internal/chat/claudecli/claudecli_test.go`
- [x] Implement the provider until the tests pass
- [x] Sync docs: document CLAUDE_BIN and CHAT_MODEL in the claudecli package
      doc and `flashcards/README.md`

**Deliverables**:

- `internal/chat/claudecli` package with stub-driven tests; no network or real
  subscription use in `make check`
- `CHAT_ENABLED=true` without a claude binary exits non-zero with a clear error

### Phase 3: Endpoint and panel — ✅ COMPLETE

**Objective**: A question typed in the drill view streams an answer grounded in
the current card.

**Tasks**:

- [x] Add failing tests with a fake Provider: POST /chat streams SSE and the
      Turn's CardContext contains the card's front/back/requires; unknown card id,
      model, or effort → 400 (validated against the fake's Options); the
      request's model/effort reach the provider, with its DefaultModel used
      when omitted; the selectors render from Options and the effort selector
      is absent when Efforts is empty; non-loopback RemoteAddr → 403;
      /chat/reset resets the provider — `internal/web/web_test.go`
- [x] Implement the handlers, card-context block, tutoring system-prompt text,
      and the SSE relay (clear the write deadline per request — see
      Implementation Details)
- [x] Add the panel markup, model/effort selectors, and JS client to
      drill.html/static
- [x] Manual verification against the real CLI: ask about a glossary card and
      confirm the answer uses ECS framing; also ask the card's own question
      before flipping it and confirm the panel explains without answering the
      card directly
- [x] Sync `flashcards/README.md` (usage section) and this spec's status

**Deliverables**:

- Working panel end-to-end on a logged-in laptop
- `flashcards/README.md` updated

---

## Test-Driven Development Requirements

### TDD Plan

- Phase 1 tests: `internal/web/web_test.go` (gating) and
  `cmd/flashcards/main_test.go` (envBool; the unknown-provider failure verified
  by running `run()` with CHAT_PROVIDER=bogus).
- Phase 2 tests: `internal/chat/claudecli/claudecli_test.go` with a stub
  binary (a shell script written to t.TempDir()) — the seam that keeps
  `make check` free of network, auth, and quota use.
- Phase 3 tests: `internal/web/chat_test.go` with a fake chat.Provider — the
  same seam a future provider swap exercises. The handler tests live beside
  `internal/web/chat.go` rather than in `web_test.go`, which is already 1000
  lines; the gating test stayed in `web_test.go` because it is about `Routes`.
- Regression: `make check` before each phase is marked complete.

### TDD Exceptions

- The final "real CLI" verification in Phase 3 is manual: it depends on a
  logged-in subscription and burns quota, so it cannot run in `make check`.
  Observed on 2026-08-01 against CLI v2.1.220:
  - Asked `term-container`'s own question unflipped, on haiku. It answered
    "You already know what this is… same OCI image, same runtime, same
    isolation" and described what to reason from without stating the card's
    definition — the tutoring prompt holds.
  - Follow-up on `term-pod`, switched to opus with `effort=low`, asking "how
    does this relate to the previous card?". It opened "A container is the unit
    you *build*; the thing on this card is the unit Kubernetes actually
    *schedules*" — the session resumed. It volunteered that requests/limits are
    per container but scheduled on the sum, and that a failing liveness probe
    restarts one container while readiness gates the whole Pod's IP: the
    production-realism bar, unprompted.
  - Provider log showed one `session_id` across both turns; web's turn log
    showed `model=haiku` then `model=opus effort=low`.
- The JS client (`internal/web/static/chat.js`) has no automated test — this
  module has no JS toolchain and adding one for ~120 lines is not the trade the
  spec wants. It is covered by the manual verification above.

---

## Technical Implementation Details

### SSE vs server timeouts

`http.Server` in `cmd/flashcards/main.go` sets `WriteTimeout: 30s`, which
applies to the whole response and would cut off any chat turn longer than 30s.
The chat handler clears it per request via
`http.NewResponseController(w).SetWriteDeadline(time.Time{})`
(`internal/web/chat.go:100`) rather than loosening the global timeout for every
route.

### The logging middleware had to learn to unwrap

`ResponseController` reaches `Flush` and `SetWriteDeadline` by walking an
`Unwrap() http.ResponseWriter` chain. `statusRecorder` in
`cmd/flashcards/main.go` wraps every response and had no `Unwrap`, so both
lookups returned `http.ErrNotSupported`: the write deadline was never cleared
and the first delta failed the turn. Fixed at `cmd/flashcards/main.go:150`.

Neither the handler tests nor `make check` caught this — `httptest.Recorder`
implements `Flush` directly, so the wrapper is never in the path. The regression
test therefore drives a real connection through `logging` itself
(`cmd/flashcards/main_test.go:44`). **Any future wrapper around the handler
chain has to unwrap or the panel silently stops streaming.**

The emit path now tolerates `ErrNotSupported` from `Flush` rather than failing
the turn: a writer that cannot flush still delivers the whole answer at the end,
which is a degraded panel, not a broken one.

### Event-stream shape

Recorded from the real CLI (v2.1.220) and frozen as the test fixture in
`internal/chat/claudecli/claudecli_test.go`. Text arrives as
`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":…}}}`;
`thinking_delta` rides the same channel and is filtered out. The session id
appears on nearly every event, including `system/init`.

Decoding uses a `json.Decoder` over the pipe rather than a line scanner: NDJSON
lines carrying a whole assistant message or a thinking signature blow past
`bufio.Scanner`'s default token limit. A `TeeReader` into a bounded tail buffer
is what lets a decode failure quote what was actually read.

### Killing the subprocess

`exec.CommandContext` kills only its direct child. The cancellation test uses
`exec sleep` in the stub for that reason — a forked `sleep` would inherit the
stdout pipe and keep it open past the kill, which is exactly how a real
misbehaving child would hang a turn.

### Refusing what a website could forge

`localOnly` requires all four of:

| Check | Stops |
|---|---|
| `RemoteAddr` is loopback | Anyone else on the network |
| `Content-Type: application/json` | The actual CSRF: a no-cors fetch or a form post may only send a safelisted content type, so JSON implies a CORS preflight this server never answers |
| `Origin` absent or loopback; `Sec-Fetch-Site` absent, `same-origin`, or `none` | A browser that does reach the handler saying where it came from |
| `Host` is a loopback name or address | DNS rebinding — a domain pointed at `127.0.0.1` is same-origin with itself, so `Origin` matches `Host` and only `Host` gives it away |

Only the literal name `localhost` and loopback IPs are accepted as hosts; every
check fails closed on anything unparseable. Absent `Origin`/`Sec-Fetch-Site` is
allowed because a non-browser client such as `curl` cannot be tricked into
making a request on someone else's behalf — the threat is specifically a browser
acting on a page's instructions.

The content-type requirement is why `chat.js` sets the header on `/chat/reset`
too, even though that request has no body.

### The working directory is load-bearing, so `make run-chat` owns it

`make run` runs from `flashcards/`, and the CLI resolves context relative to the
subprocess's working directory — which measurably is not enough (see
Troubleshooting). `run-chat` therefore builds and then runs the binary from the
repo root, pinning `DATA_DIR` back to `flashcards/data` so review state does not
move. It is a separate target rather than a flag on `run`: `run` should keep
matching what the cluster gets, and chat needs a logged-in CLI that a fresh
clone of this repo may not have.

### Panel layout, and why its CSS is gated with it

The panel sits in a second column beside the card, not below it: `drill.html`
wraps the card in `.drill-layout` > `.drill-card` and renders the panel as its
sibling. The panel is `position: sticky` and scrolls its own log, so a long
conversation never drags the card out of view — the card is what the drill is
about. Below 64rem the grid collapses to one column and the panel returns to
sitting under the card.

All of it — including the `.wrap` widening from 48rem to 76rem — lives in a
`<style>` block inside `{{with .Chat}}`, not in the shared stylesheet in
`layout.html`. Two reasons: putting it in the shared sheet left `.chat-form`
rules on every page of a chat-disabled build, which made "the page contains no
chat markup" untestable; and the wider page would then apply to a build that has
nothing to put in the extra width.

The `.drill-layout` wrapper is rendered unconditionally but is styled only by
that gated block, so with chat off it is an inert `div` and the page renders
exactly as it did before the feature existed.

htmx is unaffected: it swaps `#card` inside the left column and oob-swaps
`#drill-stats`, both of which stay where they were relative to each other.

---

## Success Criteria

- [x] With `CHAT_ENABLED` unset: `curl -i localhost:8080/chat` → 404, and the
      drill page HTML contains no chat markup
- [x] `CHAT_ENABLED=true CLAUDE_BIN=/nonexistent ./flashcards` exits non-zero
      naming the missing binary, and `CHAT_PROVIDER=bogus` exits non-zero
      naming the unknown provider
- [x] With the stub binary, `go test ./internal/chat/... ./internal/web` proves
      streaming, resume, cancellation, and the loopback guard
- [x] `internal/web` imports `internal/chat` but not
      `internal/chat/claudecli` (`go list -deps ./internal/web | grep
      claudecli` is empty) — the swap seam holds
- [x] On a logged-in laptop: a question about the current card streams an
      answer that uses the card's content and an ECS analogy, and a follow-up
      question shows the session resumed (same session id in the claudecli
      provider's log)
- [x] Switching the model selector between two turns keeps the same session id
      in the provider's log while web's turn log shows the new model
- [x] `make check` passes
- [x] Docker image builds unchanged and serves the app with no chat surface

---

## Troubleshooting Guide

**The panel prints one word then "the chat provider failed".** Check the server
log for `feature not supported`. Something in the handler chain wraps
`http.ResponseWriter` without an `Unwrap() http.ResponseWriter` method — see
*The logging middleware had to learn to unwrap* above.

**A turn dies at exactly 30s.** The write deadline was not cleared, same root
cause as above: `SetWriteDeadline` fails silently when the controller cannot
reach the real writer.

**`unreadable event stream (output tail: …)`.** The CLI's stream-json shape
changed. The tail in the error is the raw output; compare it with the fixture in
`claudecli_test.go` and update both together.

**Answers ignore the repo's ECS framing, or read like a generic model.** The
server's working directory is the subprocess's working directory, and it has to
be the **repository root**. Verified 2026-08-01: started in `flashcards/`, the
CLI resolves *no* skills and sees `CLAUDE.md` with its `@AGENTS.md` import
unexpanded; started at the root it has both. Use `make run-chat`, which runs
from the root for exactly this reason — `make run` would not.

This fails silently and looks like a weaker model rather than a
misconfiguration, so it is worth checking first.

---

## Future Enhancements

- An API-key-driven Provider (any vendor) that can run in the container —
  new spec; it will also need to replace the loopback guard with real auth
  and revisit "no secrets touch the app"
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
- This repo checked out, **with the server started at its root**: that is what
  makes the subprocess inherit `AGENTS.md` and
  `.claude/skills/k8s-for-ecs-engineers`. A subdirectory is not enough — see the
  Troubleshooting Guide. `make run-chat` exists to get this right.

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

- **Risk**: A loopback guard reads as a security boundary and is not one. Any
  page the user visits can make their browser POST to `127.0.0.1:8080`; the
  request arrives on loopback, and while the attacker cannot read the streamed
  answer, the subscription is spent and the provider's file-reading tools have
  run. Found in review, not in testing — the original guard passed every test it
  had.
- **Mitigation**: `localOnly` in `internal/web/chat.go:88` layers three more
  checks over the address one; see *Refusing what a website could forge* below.
  Tested with the forged shapes themselves, asserting the provider ran zero
  turns.

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
