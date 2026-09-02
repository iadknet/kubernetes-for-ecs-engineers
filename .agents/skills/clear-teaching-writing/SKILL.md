---
name: clear-teaching-writing
description: Use when writing or revising this project's teaching prose — flashcard `q`/`a`, `ecs` and `ecs_comparison` sections, and teaching docs and explanations across the repo — to keep it plain, direct, and free of convoluted or patterned phrasing. Companion to `k8s-for-ecs-engineers`, which owns teaching content; this skill changes wording only, never facts, ECS↔K8s mappings, or required caveats. Not for code comments or commit messages — `golang-code-style` owns those.
---

# Clear Teaching Writing

This project's teaching text should read like a good engineer explaining a
concept out loud: one idea at a time, the answer first, plain words. This skill
keeps that prose clear. It is the companion to `k8s-for-ecs-engineers`, which
decides what is true and worth teaching; this skill decides how the sentence
reads.

It applies to flashcard `q`/`a`, the `ecs` and `ecs_comparison` sections, and
the teaching docs and explanations across the repo. It changes wording only. If
a rewrite would alter a fact, an ECS↔K8s mapping, or a deliberate caveat, stop —
that belongs to `k8s-for-ecs-engineers`.

Sources: [Google Developer Documentation Style
Guide](https://developers.google.com/style) for plain language and sentence
length; [SuperMemo's Twenty
rules](https://www.supermemo.com/en/blog/twenty-rules-of-formulating-knowledge)
for the spaced-repetition layer; practitioner research on LLM writing "tells"
for the anti-tells checklist. `nbj-write-clearly` is prior art built on the same
Google guide, but it targets developer docs, not ECS-anchored flashcards — read
it, do not adopt it.

## Core rule

One card teaches one recalled idea, in plain language, and any explanation is
shorter than the answer it supports.

A card that makes you recall two things is two cards. If the `a` field needs a
second paragraph to justify the first, the justification is doing too much work —
cut it to the one sentence a learner actually needs. The enumeration card
`m0-control-plane-components` says it plainly in its own answer: "this one is the
enumeration, nothing more."

## Plain-language principles

- **Lead with the answer.** The first sentence of `a` is the thing being
  recalled. Reasons, caveats, and diagrams come after. "Only the API server."
  then the why — not three clauses of setup before the payload.
- **One clause does one job.** If a sentence has three commas and two dashes, it
  is holding several facts hostage. Split it.
- **Active voice with a named actor.** Say who acts. "The kubelet pulls its
  assignments," not "assignments are pulled." Kubernetes teaching is full of
  actors — controllers, the scheduler, kube-proxy — name them.
- **Concrete over abstract.** Prefer the real port, the real object, the real
  command over a feature description. Reuse the `flashcards` workload and its
  real fields rather than an abstract "the application."
- **Sentence length: aim for about 25 words max, each doing one thing — then
  vary it deliberately.** A run of same-length sentences reads like a metronome.
  Follow a long sentence with a short one. The short sentence is where the point
  lands.

## Spaced-repetition layer

Flashcard text carries an extra constraint on top of plain language: the
minimum-information principle. Formulate the smallest item that still makes
sense.

- **Atomic recall.** One question, one retrievable answer. `ClusterIP,
  NodePort, LoadBalancer — what does each add?` works because each is a superset
  of the last; a card asking to compare five unrelated fields does not.
- **Never pack two facts into one sentence.** "readiness controls Service
  traffic and liveness restarts the container" is two recall targets welded
  together. If the card tests both, use two clauses a learner can answer
  separately — or two cards.
- **Understand before memorize.** If the phrasing only parses once you already
  know the answer, rewrite it. The card teaches; it does not quiz trivia.

## Anti-tells checklist

These are the convolutions that creep into teaching prose drafted by capable
models. They are **heuristics, not bans** — one deliberate em dash or triad is
good writing. The target is the reflexive, patterned overuse. Each `before` is
realistic ECS→K8s card prose; each `after` keeps every fact and caveat.

**Em-dash asides (overuse).** Nested dashes bury the main clause.
- before: A readiness probe — unlike liveness, which restarts the container —
  controls Service traffic — so a failing Pod leaves rotation — without a
  restart.
- after: A readiness probe controls Service traffic. When it fails, the Pod
  leaves rotation but is not restarted. Liveness is the probe that restarts.

**"It's not X, it's Y."** The negation is filler; state the positive fact.
- before: A Kubernetes Service is not a replica manager, it's a network
  endpoint.
- after: A Kubernetes Service is a network endpoint — a stable virtual IP that
  load-balances to selected Pods. Replica count belongs to the Deployment.

**Rule-of-three padding.** Decorative triads add cadence, not information.
- before: `kubectl apply` writes desired state cleanly, declaratively, and
  idempotently, and controllers take it from there.
- after: `kubectl apply` writes desired state; controllers reconcile from there.
  Applying the same manifest twice changes nothing.

**"Which is why" chains.** One causal link is fine; a chain hides the logic.
- before: The kubelet pulls its work, which is why nothing pushes to it, which
  is why a wedged agent looks healthy from outside.
- after: The kubelet pulls its work; nothing pushes to it. So a wedged agent
  looks healthy from outside until you check what is actually running on it.

**Hedging and throat-clearing.** Qualifiers dilute a fact the card states as
true.
- before: It's worth noting that, generally speaking, a Service does not in most
  cases manage replicas.
- after: A Service does not manage replicas.

**Low sentence-length variance.** A metronome of 14–22-word sentences is
exhausting even when each is correct.
- before: A Deployment owns ReplicaSets and holds them at the desired count. A
  ReplicaSet owns Pods and holds them at the desired count. A rolling update
  builds a new ReplicaSet and moves the replicas across.
- after: A Deployment owns ReplicaSets; a ReplicaSet owns Pods. Each holds its
  children at the desired count. A rolling update creates a new ReplicaSet and
  shifts replicas from old to new — which is why rollback is cheap.

**Monotone openers.** Three sentences starting `The`/`This`/`It`/`In` in a row
read as a list, not an explanation.
- before: The Service selects Pods by label. The Service load-balances across
  them. The Service plays no part in scaling.
- after: A Service selects Pods by label and load-balances across them. Scaling
  is not its job — that is the Deployment's.

**Style-words.** `delve`, `tapestry`, `realm`, `leverage`, `seamless`,
`robust`, `crucial` signal filler. Name the thing instead.
- before: Let's delve into the rich tapestry of Kubernetes networking
  primitives.
- after: Kubernetes splits networking across separate objects: Service, Ingress,
  and NetworkPolicy.

## Boundaries

This skill changes wording only. Each neighboring concern has an owner:

- **Content accuracy and ECS↔K8s mappings** → `k8s-for-ecs-engineers`. Whether a
  mapping is direct, partial, split, or absent, and whether a caveat is
  required, is its call, not a wording choice.
- **Specs** → `spec-writing`. Spec prose lives under a different contract.
- **Code and code comments** → `golang-code-style`. Comments and commit messages
  are out of scope here.

Preserve technical terms, API and field names, UI labels, modal verbs (`must`,
`may`, `can`), and deliberate caveats verbatim. Clarity is not the removal of
necessary precision. If a wording fix would drop a "partial mapping" note or
soften a "base64 is not encryption" warning, stop and leave it to the content
skill.

## Self-review pass

Before teaching text lands, read it once against this list:

- [ ] The first sentence of the answer is the thing being recalled.
- [ ] One recalled idea per card; any explanation is shorter than the answer.
- [ ] No sentence packs two separate facts a learner must recall apart.
- [ ] Active voice with a named actor where an actor exists.
- [ ] Sentence lengths vary; no metronome, no monotone openers.
- [ ] No reflexive em-dash asides, "not X but Y", decorative triads, "which is
      why" chains, hedging, or style-words.
- [ ] Every technical term, field name, and caveat survived the edit unchanged.
