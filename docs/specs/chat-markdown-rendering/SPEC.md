# Chat Markdown Rendering — Technical Spec

**Status**: ✅ COMPLETE
**Last updated**: 2026-08-02

---

## Overview

The drill view's chat panel already receives markdown — Claude answers with
fenced code blocks, lists and emphasis — but `chat.js:48` assigns each delta to
`.textContent`, so a `kubectl` example arrives as prose with visible backticks.
The only thing making it legible is `white-space: pre-wrap`
(`flashcards/internal/web/templates/drill.html:44`).

This spec renders those answers through the goldmark instance the app already
uses for cards (`flashcards/internal/web/web.go:64`), so a tutor answer is
styled exactly like the card beside it. Rendering happens **once, server-side,
when the turn completes**: the `done` event carries the rendered HTML and the
client swaps it in. Deltas keep streaming as plain text, so the answer still
appears as it is written.

Nothing changes about what the model is asked for or what it returns. This is a
rendering change on an existing stream.

---

## Business Requirements

### Learning Objectives

- A command, manifest or Go snippet in a tutor answer is visually a code block,
  distinguishable from the prose around it at a glance, so it can be copied and
  run without picking backticks out of a sentence.
- A tutor answer and a card answer look the same, because they are the same
  markdown through the same renderer. The panel is not a second visual dialect
  to learn.

### Production Bar

- Streaming survives. The answer still appears progressively; the render is not
  paid for with a spinner and a wall of text at the end.
- Model output is treated as untrusted input, and that property is held by a
  test rather than by the current default of a dependency.
- A failed turn still shows the partial answer and the error, and does not
  discard text the learner was already reading.
- No new browser dependency. The renderer that ships is the one already in the
  binary.

---

## Technical Requirements

### Kubernetes Resources

**Not applicable** — Go handler, one static JS file, and CSS.

### Go Components

```text
- `internal/web/chat.go:176` — accumulate the deltas passed to the emit
  callback into a strings.Builder alongside relaying them, and send the
  rendered result as the `done` event payload at `chat.go:204`. The
  accumulation is the whole server-side change; `writeEvent` already
  JSON-encodes payloads, so HTML needs no new framing.
- `internal/web/chat.go` — render via the existing `s.markdown()`
  (`internal/web/web.go:309`), not a new renderer. See **Sanitization** for why
  the shared instance is safe and what must be true for it to stay so.
- `internal/web/web.go:320-326` — the comment above the G203 suppression says
  deck content is trusted compile-time input. That stops being the whole truth
  the moment chat renders through it. Rewrite it to state the actual property
  the code relies on: goldmark's HTML renderer is not in Unsafe mode, so raw
  HTML in *any* input is omitted rather than passed through.
- `internal/web/static/chat.js` — on `done`, swap the answer node's innerHTML
  and add the `md` class; then run MermaidJS over any diagram in it. On
  `error`, leave the plain-text node alone.
- `internal/web/templates/drill.html` — a `.chat-msg.md` rule undoing
  `pre-wrap` and taming block margins inside a bubble.
```

### Sanitization

The panel's answer is model output. It is not deck content, so the
trusted-compile-time-input argument at `web.go:320` does not cover it, and the
question of whether the shared renderer is safe has to be answered rather than
assumed.

It is. goldmark's HTML renderer defaults to `Unsafe: false`, and the app never
sets `html.WithUnsafe()`. Verified against this app's exact configuration —
`goldmark.New(goldmark.WithExtensions(extension.GFM, mermaidExtender()))`:

| Input | Output |
| --- | --- |
| `<script>alert(1)</script>` | `<!-- raw HTML omitted -->` |
| `hello <img src=x onerror=alert(1)>` | `<p>hello <!-- raw HTML omitted --></p>` |
| `[click](javascript:alert(1))` | `<a href="">click</a>` |
| `![x](data:text/html;base64,…)` | `<img src="" alt="x">` |

So no sanitizer and no second goldmark instance is needed — but that safety is
a **default**, not a declaration, and a default can be switched off by someone
working on cards who never thinks about the chat panel. Adding
`html.WithUnsafe()` to `s.md` for a card-authoring reason would silently make
model output executable. `TestChatRenderedAnswerOmitsRawHTML` is what turns
that from a silent change into a failing build; it is the load-bearing test in
this spec.

A separate chat-only goldmark instance was considered as the alternative
boundary and rejected: its config would have to be kept in sync with `s.md` by
hand, and config drift between two renderers is a worse and quieter failure
than one renderer with a test on it.

### Rendering Integration

Five things the swap does not solve by itself.

1. **`pre-wrap` must stop applying once the node is HTML.** `.chat-msg` sets
   `white-space: pre-wrap` (`drill.html:44`), which is what makes the streaming
   plain text readable and what must not survive the swap — goldmark's output
   carries the source's newlines inside its `<p>` elements, so a rendered
   answer under `pre-wrap` gets every prose line break twice. The `md` class
   added at swap time sets `white-space: normal`. Scoping it to a class rather
   than changing `.chat-msg` keeps `pre-wrap` where it is still needed: the
   streaming state, and the `you` bubble, which stays plain text.

2. **Block margins need taming inside a bubble.** `.chat-log` spaces messages
   with `gap: .85rem`; default `<p>`, `<ul>` and `<pre>` margins then add a
   second, larger gap at the top and bottom of every answer. Zero the first
   child's `margin-top` and the last child's `margin-bottom`. Nothing else
   about `code`/`pre` needs styling — `layout.html:34-38` already covers both,
   including `overflow-x: auto`, which is what keeps a wide command inside the
   panel's `minmax(0, 1fr)` column.

3. **MermaidJS must be re-run over the answer.** The renderer emits
   `<pre class="mermaid">` for a mermaid fence, and the only thing that draws
   those is the `htmx:afterSwap` listener at `layout.html:129`. Chat does not
   use htmx, so without an explicit call an answer containing a diagram shows
   raw Mermaid source with the code-block framing deliberately removed by the
   `pre.mermaid` rule (`layout.html:41`) — worse-looking than today. `chat.js`
   calls `mermaid.run()` over the answer node after the swap.

   Deck diagrams are human-written and gated by `TestDiagramStyle`; a model
   improvising one at temperature has no such guard, so an unparseable diagram
   must degrade to its visible source. That takes **three** things, and the
   first two are each insufficient — see point 4, which records what each one
   actually does.

4. **A bad diagram takes three settings to degrade well.** Established by
   observation against MermaidJS 11.16.0, after the first two each turned out
   not to do what their names suggest:

   | Setting | What it actually does |
   | --- | --- |
   | `run({ suppressErrors: true })` | Stops `run()` throwing, so later nodes still process. Draws the error graphic anyway. |
   | `initialize({ suppressErrorRendering: true })` | Stops the error graphic. Leaves the node emptied — a silent blank gap. |
   | Restoring `textContent` in `chat.js` | What actually puts the source back. |

   All three ship. `chat.js` stashes each block's source before the run and, in
   the promise's `finally`, restores it on any node that has no `<svg>`,
   flagging it `data-failed` so the `pre.mermaid` rule
   (`layout.html:41`) — which strips code-block framing from diagrams — hands
   it back for a block that is source again.

   `suppressErrorRendering` is set in the one global `initialize` call, so it
   applies to deck diagrams too. That is a fix rather than a side effect: the
   card-diagrams spec already claims a failed diagram "degrades to visible
   Mermaid source text", which was not true before this change. Verified that
   all seven M0 diagrams still render.

5. **The error path does not render.** A turn that fails mid-stream has partial
   markdown, and half a fenced block renders as something misleading rather
   than as what arrived. `error` keeps today's behaviour exactly: plain text,
   `err` class, message appended. Only `done` swaps.

### Observability

**Not applicable** — no new long-running component. The existing
`slog.Info("chat turn", …)` at `chat.go:202` is unchanged; render failures
inside `s.markdown()` already fall back to escaped plain text
(`web.go:316-318`) rather than failing the turn.

---

## Implementation Phases

### Phase 1: Render on completion — ✅ COMPLETE

**Objective**: The `done` event carries rendered HTML, the panel swaps it in,
and model output is provably not executable.

**Tasks**:

- [x] Add failing `TestChatDoneCarriesRenderedHTML` in
      `internal/web/chat_test.go`: a `fakeProvider` whose deltas spell a fenced
      `go` block across a chunk boundary, asserting the `done` event payload
      contains `<pre><code class="language-go">`. Splitting the fence across
      deltas is the point — it is what fails if a later change tries to render
      per-delta instead of on completion
- [x] Add failing `TestChatRenderedAnswerOmitsRawHTML`: deltas containing
      `<script>alert(1)</script>` and `[x](javascript:alert(1))` yield a `done`
      payload with no `<script` and no `javascript:`. This is the test that
      makes sharing `s.md` safe; see **Sanitization**
- [x] Add failing `TestChatFailedTurnSendsNoRenderedAnswer`: a provider that
      errors after emitting deltas still produces `event: error` and no
      `event: done`
- [x] Accumulate deltas in `handleChat` and send `s.markdown()` of the result
      as the `done` payload
- [x] Rewrite the trust comment at `internal/web/web.go:320-326` to state the
      not-Unsafe property rather than the deck-content one, naming chat as the
      second caller
- [x] Swap `innerHTML` and add the `md` class on `done` in `chat.js`; leave the
      `error` path as plain text
- [x] Call `mermaid.run({ nodes: …, suppressErrors: true })` over the swapped
      answer, guarded on `window.mermaid` being present
- [x] Make an unparseable diagram degrade to its source, which took two more
      things than expected: `suppressErrorRendering: true` in `layout.html`'s
      `initialize`, and restoring each block's stashed `textContent` in
      `chat.js` for any node left without an `<svg>`. See point 4 of
      **Rendering Integration** for what each of the three settings does
- [x] Add the `.chat-msg.md` CSS to `drill.html`: `white-space: normal`, and
      first-child/last-child margin resets
- [x] Add a `pre.mermaid[data-failed]` rule to `layout.html` restoring the
      code-block framing to a diagram that came back as source
- [x] Verify in the browser via `make run`: ask for a `kubectl` example and
      confirm the block is styled, the prose is single-spaced, and the text
      still streamed before it rendered. Check both OS themes
- [x] Ask for a diagram and confirm it draws; hand the panel a deliberately
      malformed one and confirm it degrades to visible source rather than an
      error graphic
- [x] Confirm the global `suppressErrorRendering` did not change deck diagrams:
      all 7 `pre.mermaid` blocks on `/browse?module=M0` still render an `<svg>`
      and none is flagged `data-failed`
- [x] Sync affected documentation with the implemented changes

**Deliverables**:

- `internal/web/chat.go` — delta accumulation and a rendered `done` payload
- `internal/web/web.go` — corrected trust comment
- `internal/web/static/chat.js` — completion swap, Mermaid re-run, and the
  source restore for a diagram that would not parse
- `internal/web/templates/drill.html` — `.chat-msg.md` styling
- `internal/web/templates/layout.html` — `suppressErrorRendering` and the
  `pre.mermaid[data-failed]` rule
- `internal/web/chat_test.go` — the three tests above

`docs/specs/claude-chat-panel/SPEC.md` needs no edit: it specifies the SSE
frames, the panel markup and the provider interface, and never states how the
answer text is displayed. Checked before writing this spec, so the sync task
above is not an invitation to go find something to change.

---

## Test-Driven Development Requirements

### TDD Plan

- Phase 1 tests: `TestChatDoneCarriesRenderedHTML`,
  `TestChatRenderedAnswerOmitsRawHTML` and
  `TestChatFailedTurnSendsNoRenderedAnswer` in `internal/web/chat_test.go`, all
  driven through the existing `fakeProvider` (`chat_test.go:42`) and `ask`
  helper (`chat_test.go:154`). No new fixtures are needed.
- `TestChatStreamsAnAnswerGroundedInTheCard` (`chat_test.go:163`) already
  asserts `event: delta` frames carry the raw chunks and that `event: done`
  arrives. It must stay green unchanged — that is the regression guard on
  streaming surviving this change.
- Regression: `make check`

### TDD Exceptions

- The `pre-wrap` interaction, the margin resets and the Mermaid re-run are
  verified in the browser rather than by a Go test. A test could assert the CSS
  rule and the `mermaid.run` call exist as strings; it could not assert the
  answer reads correctly, which is the actual requirement. Both are explicit
  browser-check tasks in Phase 1.

---

## Technical Implementation Details

### Key Files

- `flashcards/internal/web/chat.go:183` — the accumulation, inside the `Send`
  emit callback
- `flashcards/internal/web/chat.go:215` — the rendered `done` payload
- `flashcards/internal/web/chat.go:349` — `writeEvent`; its JSON encoding
  already handles HTML containing blank lines, which is why no framing changed
- `flashcards/internal/web/web.go:64` — the shared goldmark instance. The
  comment above `markdown()` is what stands between it and `WithUnsafe()`
- `flashcards/internal/web/web.go:309` — `markdown()`, reachable from
  `handleChat` directly because both are `package web`
- `flashcards/internal/web/static/chat.js:37` — `render()`, the swap and the
  Mermaid handling
- `flashcards/internal/web/static/chat.js:90` — the `done` branch of
  `handleFrame`, which previously ignored the event entirely
- `flashcards/internal/web/templates/drill.html:50` — `.chat-msg.md`, which
  overrides `.chat-msg`'s `pre-wrap`
- `flashcards/internal/web/templates/layout.html:34-38` — the `code`/`pre`
  styling the rendered answer inherits for free
- `flashcards/internal/web/templates/layout.html:44` —
  `pre.mermaid[data-failed]`, the framing for a restored diagram source
- `flashcards/internal/web/templates/layout.html:133` —
  `suppressErrorRendering`, which affects deck diagrams too
- `flashcards/internal/web/templates/layout.html:140` — the `htmx:afterSwap`
  Mermaid hook that chat cannot rely on

---

## Success Criteria

- [x] `make check` passes
- [x] Asking for a `kubectl` example in the panel renders a styled code block,
      observed in the browser via `make run` in both OS themes
- [x] The answer text still streams in progressively before it renders — the
      panel is not blank until the turn ends
- [x] Rendered prose is single-spaced: no doubled line breaks from `pre-wrap`
      surviving the swap
- [x] `TestChatRenderedAnswerOmitsRawHTML` fails if `html.WithUnsafe()` is
      added to the goldmark instance at `web.go:64` — verified by adding
      `goldmark.WithRendererOptions(html.WithUnsafe())` there, watching the test
      fail, and reverting
- [x] `TestChatStreamsAnAnswerGroundedInTheCard` passes unchanged
- [x] A failed turn shows the partial answer plus the error, with no swap
- [x] A mermaid fence in an answer draws as a diagram; a malformed one leaves
      its source visible rather than an error graphic

---

## Troubleshooting Guide

### A malformed diagram shows a bomb icon, or a blank gap

**Problem**: An unparseable `mermaid` block in a chat answer was replaced by
mermaid's "Syntax error in text" bomb graphic. Setting `suppressErrors: true`
on the `run()` call did not stop it; adding `suppressErrorRendering: true`
stopped the bomb but left an empty gap where the block had been.
**Cause**: The two options do different jobs, and neither restores content.
`suppressErrors` only stops `run()` throwing, so the remaining nodes still
process; the error graphic is drawn by the renderer before the throw.
`suppressErrorRendering` stops that draw, but mermaid has already emptied the
node by then.
**Solution**: Both options, plus restoring the source. `chat.js` stashes each
block's `textContent` before the run and puts it back, in the promise's
`finally`, on any node that ended up without an `<svg>`.
**Reference**: `flashcards/internal/web/static/chat.js:37`

### The chat panel could not be exercised without spending a subscription

**Problem**: Every browser check needs a real answer, but the only provider is
the Claude CLI against a live subscription — slow, non-deterministic, and it
cannot be made to emit a deliberately malformed diagram on demand.
**Cause**: The panel's verification is end-to-end by nature; the Go tests cover
the server's half of the contract but not the swap, the CSS, or Mermaid.
**Solution**: `CLAUDE_BIN` (`internal/chat/claudecli/claudecli.go:15`) accepts
any executable, and the provider only needs NDJSON `text_delta` events on
stdout. A ~40-line script emitting canned markdown — chunked mid-token and
paced with a sleep so the streaming-then-render transition is observable —
drives the whole path deterministically and offline. Pick the answer off a
substring of the question, which arrives as the last argv element, to get a
good diagram and a malformed one from the same stub.

---

## Future Enhancements

- Render progressively during the stream — throttled re-render of the
  accumulated buffer, or a client-side incremental parser — if answers ever
  grow long enough that the unstyled window is annoying. The tutor prompt
  (`chat.go:38`) caps them at "a few sentences or a short list", so it is not
  today.
- Copy-to-clipboard on a rendered code block.

---

## Dependencies

### External Dependencies

- `github.com/yuin/goldmark` v1.8.5 — already a direct dependency; no version
  change and no new module
- MermaidJS 11.16.0, already vendored at
  `flashcards/internal/web/static/mermaid.min.js`

### Internal Dependencies

- `docs/specs/claude-chat-panel/SPEC.md` — the panel this extends: the SSE
  frame format, `localOnly`, and the provider interface are all its work and
  are unchanged here
- `docs/specs/card-diagrams/SPEC.md` — the Mermaid wiring whose client-render
  and `NoScript` decisions are why chat must call `mermaid.run()` itself

---

## Risks and Mitigation

### Technical Risks

- **Risk**: `html.WithUnsafe()` is added to the shared goldmark instance for a
  card-authoring reason — an author wants inline HTML in a deck — and silently
  makes model output executable in the panel.
- **Mitigation**: `TestChatRenderedAnswerOmitsRawHTML` fails on that change.
  The corrected comment at `web.go:320` names chat as a caller so the
  consequence is visible at the point of edit, not only in CI.

- **Risk**: A later change renders per-delta instead of on completion, because
  live rendering looks like the obvious improvement, and half-parsed fences
  start flickering mid-stream.
- **Mitigation**: `TestChatDoneCarriesRenderedHTML` splits its fence across
  delta boundaries, so a per-delta renderer fails it rather than merely looking
  different.

- **Risk**: The `md` class is folded into `.chat-msg` as a tidy-up, restoring
  `pre-wrap` to rendered HTML and double-spacing every answer.
- **Mitigation**: Single-spacing is its own success criterion with a browser
  check, and **Rendering Integration** records that the class split exists to
  keep `pre-wrap` on the streaming and `you` states.

### Learning Risks

- **Risk**: A rendered, authoritative-looking answer reads as more reliable
  than the same text did in plain prose, and gets trusted over the card.
- **Mitigation**: None beyond the tutor prompt's existing rule against
  answering the card. Worth noticing rather than engineering against: the
  panel is a study aid, and the deck stays the source of truth.

---

## Notes for AI Agents

Follow the workflow in `docs/specs/TEMPLATE.md` under **Notes for AI Agents**.
Specific to this spec:

1. Do not add a markdown library — server or browser. The renderer is
   `s.markdown()` and the decision not to add marked/markdown-it plus a
   sanitizer is settled in **Sanitization** and the Overview.
2. Do not give chat its own goldmark instance. That was considered and
   rejected; the shared instance plus `TestChatRenderedAnswerOmitsRawHTML` is
   the chosen boundary.
3. `event: delta` keeps carrying plain text. Anything that changes the delta
   payload breaks streaming for a rendering gain this spec explicitly defers to
   **Future Enhancements**.
4. The bad-diagram degradation is three cooperating pieces —
   `suppressErrors`, `suppressErrorRendering`, and the `textContent` restore in
   `chat.js`. Removing any one brings back either a bomb graphic or a silent
   blank gap. Point 4 of **Rendering Integration** records which does what; read
   it before simplifying, because the option names do not describe their
   behaviour.
5. `suppressErrorRendering` is global and deck diagrams share it. That is
   intended. If deck-diagram authoring ever wants the error graphic back, scope
   it — do not simply flip the global, which silently re-breaks the chat path.
