# Authoring diagrams in cards

A card whose answer is a *sequence* — the kubelet pull loop, reconciliation,
graceful shutdown, the API request path — may carry a diagram so the order and
direction of control are recalled visually instead of reassembled from prose.

A diagram is a fenced block tagged `mermaid`, inside `a:`:

````yaml
    a: |
      The kubelet on that machine notices the assignment, calls the container
      runtime over CRI, and reports back.

      ```mermaid
      flowchart LR
        K[kubelet] -->|watches for its assignments| API[API server]
        K -->|starts the work over CRI| R[container runtime]
      ```
````

These rules apply to `mermaid` blocks only. The untagged kubectl and YAML
examples elsewhere in the decks are deliberately out of scope — card text is
already full of identifiers rules 1 and 2 would reject (`AWS_PROFILE`,
`http_requests_total`), and widening the lint would force an allowlist.

## The rules

1. **Space-separate every arrow** — `A --> B`, never `A-->B`.
2. **No `_` next to an alphanumeric.** Use spaces or hyphens in ids and labels.
3. **Prefer a bracket label for a glossary term** — `K[kubelet]` over a bare
   `kubelet` node id. Not enforced.
4. **`flowchart` only**, any direction. One diagram vocabulary is one thing to
   learn to read. If a card genuinely needs `sequenceDiagram` or
   `stateDiagram`, loosen the lint then — not in advance.
5. **The diagram supplements the prose; it never replaces it.** The answer must
   still read correctly with the diagram deleted. The diagram is the recall
   aid, not the content. Not enforced.

Rules 1, 2 and 4 are enforced by `TestDiagramStyle`
(`internal/deck/diagram_test.go`), which runs under `make lint-decks`.

## Choosing a direction

The card column is 48rem. A `flowchart LR` chain of four or more nodes is wider
than that, and MermaidJS answers by scaling the whole diagram down until the
labels are too small to read — it does not wrap or overflow, so nothing looks
broken. Use `flowchart TD` for any chain of four or more, and keep `LR` for
three-node flows and for fan-in shapes, which are naturally wide but shallow.

## Why rules 1 and 2 exist

They are not cosmetic. The vocabulary gate scans a card as raw text, with no
markdown awareness, so **diagram labels are gated exactly like prose** — and
its word-boundary class (`internal/deck/glossary_test.go`, `termUse`) counts
`-` and `_` as word characters. A glossary term touching either is invisible to
the gate:

| In the diagram | Term `kubelet` seen by the gate |
| --- | --- |
| `kubelet-->CRI` | **no** |
| `kubelet_status` | **no** |
| `K[kubelet] --> C` | yes |
| `kubelet --> C` | yes |
| `-->\|kubelet\|` | yes |
| `API[kube-apiserver]` | yes, for the term `kube-apiserver` |

So an idiomatic tight arrow does not break the render — it silently exempts the
card from the vocabulary gate, which is the failure nobody would notice.

Note what is **not** a problem: a hyphen *inside* a term (`kube-apiserver`,
`kube-proxy`) is fine, because `[` and `]` are boundaries and the term itself
contains that hyphen. Only a `-` or `_` separating a term from something else
hides it. The lint bans *arrow operators* and underscores against alphanumerics
— not hyphens generally.

## `requires:` edges

Because labels are scanned, adding a diagram can pull new glossary terms into a
card and force new `requires:` edges. That is correct: the card now genuinely
uses that vocabulary. Add the edges in the same change as the diagram.

Better still, keep the diagram inside the vocabulary the card has already
earned. A diagram is a recall aid for what the answer says; it should not be
where a term is introduced.
