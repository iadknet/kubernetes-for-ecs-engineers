package deck_test

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	flashcards "github.com/iadk/k8s-flashcards"
	"github.com/iadk/k8s-flashcards/internal/deck"
)

// Card ids in the prerequisite fixture below.
const (
	base      = "term-base"
	alpha     = "term-a"
	beta      = "term-b"
	concept   = "concept"
	unrelated = "unrelated"
)

// Card ids in the checkpoint fixtures, shared with deck_test.go.
const (
	moduleCard     = "m0-one"
	checkpointCard = "m0-checkpoint"
)

// prereqLib is a small dependency graph:
//
//	concept -> term-a -> term-base
//	        -> term-b
//	unrelated (no edges)
func prereqLib(t *testing.T) *deck.Library {
	t.Helper()

	const body = `
deck: Prereqs
cards:
  - id: term-base
    term: Base
    q: q
    a: a
  - id: term-a
    term: Alpha
    q: q
    a: a
    requires: [term-base]
  - id: term-b
    term: Beta
    q: q
    a: a
  - id: concept
    q: q
    a: a
    requires: [term-a, term-b]
  - id: unrelated
    q: q
    a: a
`

	lib, err := deck.Load(fstest.MapFS{"decks/p.yaml": {Data: []byte(body)}}, "decks")
	if err != nil {
		t.Fatal(err)
	}

	return lib
}

// loadDeck loads a single-deck fixture written inline.
func loadDeck(body string) (*deck.Library, error) {
	return deck.Load(fstest.MapFS{"decks/g.yaml": {Data: []byte(body)}}, "decks")
}

func ids(cards []deck.Card) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.ID)
	}

	return out
}

func TestWithPrerequisites(t *testing.T) {
	t.Parallel()

	lib := prereqLib(t)
	pick := func(want ...string) []deck.Card {
		var out []deck.Card

		for _, id := range want {
			c, ok := lib.Get(id)
			if !ok {
				t.Fatalf("no such card %q", id)
			}

			out = append(out, c)
		}

		return out
	}

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "pulls in the transitive closure, in dependency order, before the input",
			in:   []string{concept},
			want: []string{base, alpha, beta, concept},
		},
		{
			name: "a set that already contains its prerequisites is unchanged",
			in:   []string{base, alpha, beta, concept},
			want: []string{base, alpha, beta, concept},
		},
		{
			name: "only the missing prerequisites are prepended",
			in:   []string{alpha, concept},
			want: []string{base, beta, alpha, concept},
		},
		{
			name: "a card with no prerequisites is untouched",
			in:   []string{unrelated},
			want: []string{unrelated},
		},
		{
			name: "the closure is not the whole library",
			in:   []string{alpha},
			want: []string{base, alpha},
		},
		{
			name: "an empty selection stays empty",
			in:   nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ids(lib.WithPrerequisites(pick(tt.in...)))
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("WithPrerequisites(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// Input order is what the scheduler introduces cards in, so an expansion that
// reshuffles the authored order would silently rewrite the study sequence.
func TestWithPrerequisitesKeepsInputOrder(t *testing.T) {
	t.Parallel()

	lib := prereqLib(t)

	in := []deck.Card{}

	for _, id := range []string{unrelated, concept, beta} {
		c, _ := lib.Get(id)
		in = append(in, c)
	}

	got := ids(lib.WithPrerequisites(in))
	want := []string{base, alpha, unrelated, concept, beta}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A module drill must never pull the module's exam into the study queue, and an
// exam must never drag the whole module into an unrelated scope. Checkpoint
// cards are therefore neither expansion targets nor expansion sources.
func TestWithPrerequisitesIgnoresCheckpointCards(t *testing.T) {
	t.Parallel()

	const body = `
deck: Checkpointed
module: M0
cards:
  - id: term-base
    term: Base
    q: q
    a: a
  - id: m0-one
    q: q
    a: a
    requires: [term-base]
  - id: m0-checkpoint
    checkpoint: M0
    q: q
    a: a
`

	lib, err := deck.Load(fstest.MapFS{"decks/c.yaml": {Data: []byte(body)}}, "decks")
	if err != nil {
		t.Fatal(err)
	}

	pick := func(want ...string) []deck.Card {
		var out []deck.Card

		for _, id := range want {
			c, ok := lib.Get(id)
			if !ok {
				t.Fatalf("no such card %q", id)
			}

			out = append(out, c)
		}

		return out
	}

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "a checkpoint card is not added as a prerequisite",
			in:   []string{moduleCard},
			want: []string{"term-base", moduleCard},
		},
		{
			name: "a checkpoint card in the input pulls in nothing",
			in:   []string{checkpointCard},
			want: []string{checkpointCard},
		},
		{
			name: "a scope containing the exam still expands its ordinary cards",
			in:   []string{moduleCard, checkpointCard},
			want: []string{"term-base", moduleCard, checkpointCard},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ids(lib.WithPrerequisites(pick(tt.in...)))
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("WithPrerequisites(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestGlossaryIndexesTermsAndAliases(t *testing.T) {
	t.Parallel()

	const body = `
deck: G
cards:
  - id: term-svc
    term: Service
    aliases: [Services]
    q: q
    a: a
  - id: plain
    q: q
    a: a
`

	lib, err := loadDeck(body)
	if err != nil {
		t.Fatal(err)
	}

	g := lib.Glossary()
	if len(g) != 2 {
		t.Fatalf("glossary has %d entries, want 2 (term + alias)", len(g))
	}

	for _, key := range []string{"Service", "Services"} {
		if c, ok := g[key]; !ok || c.ID != "term-svc" {
			t.Errorf("glossary[%q] = %v, %v; want term-svc", key, c.ID, ok)
		}
	}
}

// --- Distractors -------------------------------------------------------------

// distractorDeck is a glossary with one obvious neighbourhood — the three
// control-plane components — plus outsiders, and one concept card whose
// `requires:` edges make a second, cross-tag neighbourhood.
//
// The deck-level tag is deliberately present: every real glossary card carries
// it, so a selection that treated any shared tag as a neighbourhood signal would
// see the whole glossary as one neighbourhood and the preference would be inert.
const distractorDeck = `
deck: Glossary
tags: [glossary]
cards:
  - id: term-apiserver
    term: apiserver
    tags: [control-plane]
    q: apiserver
    a: The cluster API front door.
  - id: term-scheduler
    term: scheduler
    tags: [control-plane]
    q: scheduler
    a: Assigns unassigned workloads to machines.
  - id: term-controller-manager
    term: controller-manager
    tags: [control-plane]
    q: controller-manager
    a: Runs the reconciliation loops.
  - id: term-etcd
    term: etcd
    tags: [storage]
    q: etcd
    a: The cluster datastore.
  - id: term-kubelet
    term: kubelet
    tags: [machine]
    q: kubelet
    a: The per-machine agent.
  - id: term-ingress
    term: ingress
    tags: [network]
    q: ingress
    a: HTTP routing from outside the cluster.
  - id: term-probe
    term: probe
    tags: [health]
    q: probe
    a: A periodic health check.
  - id: pairs-etcd-and-kubelet
    q: |
      Which two pieces hold state?
    a: |
      One of them does.
    requires: [term-etcd, term-kubelet]
`

func distractorLib(t *testing.T) *deck.Library {
	t.Helper()

	lib, err := loadDeck(distractorDeck)
	if err != nil {
		t.Fatal(err)
	}

	return lib
}

func TestDistractors(t *testing.T) {
	t.Parallel()

	lib := distractorLib(t)

	get := func(id string) deck.Card {
		t.Helper()

		c, ok := lib.Get(id)
		if !ok {
			t.Fatalf("no such card %q", id)
		}

		return c
	}

	tests := []struct {
		name string
		card string
		want []string // unordered; empty means "only check the count"
	}{
		{
			name: "prefers the tag neighbourhood over the rest of the glossary",
			card: "term-apiserver",
			want: []string{"term-scheduler", "term-controller-manager"},
		},
		{
			name: "prefers terms a single card requires together",
			card: "term-etcd",
			want: []string{"term-kubelet"},
		},
		{
			name: "a term with no neighbourhood still gets three",
			card: "term-probe",
		},
		{
			name: "a term whose only sibling is itself draws from the rest",
			card: "term-ingress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ids(lib.Distractors(get(tt.card), "2026-07-31"))
			if len(got) != 3 {
				t.Fatalf("Distractors(%s) = %v, want 3", tt.card, got)
			}

			for _, id := range got {
				if id == tt.card {
					t.Errorf("Distractors(%s) included the card's own answer", tt.card)
				}

				if c, _ := lib.Get(id); c.Term == "" {
					t.Errorf("Distractors(%s) picked non-glossary card %q", tt.card, id)
				}
			}

			for _, id := range tt.want {
				if !slices.Contains(got, id) {
					t.Errorf("Distractors(%s) = %v, want it to contain %q", tt.card, got, id)
				}
			}
		})
	}
}

// The override exists because an automatic pick can be accidentally also-correct.
// It must therefore win outright, not merely seed the automatic selection.
func TestDistractorsOverrideWinsOutright(t *testing.T) {
	t.Parallel()

	const body = `
deck: Glossary
cards:
  - id: term-apiserver
    term: apiserver
    tags: [control-plane]
    q: apiserver
    a: The cluster API front door.
    distractors: [term-etcd, term-kubelet, term-probe]
  - id: term-scheduler
    term: scheduler
    tags: [control-plane]
    q: scheduler
    a: Assigns work.
  - id: term-etcd
    term: etcd
    q: etcd
    a: The datastore.
  - id: term-kubelet
    term: kubelet
    q: kubelet
    a: The agent.
  - id: term-probe
    term: probe
    q: probe
    a: A health check.
`

	lib, err := loadDeck(body)
	if err != nil {
		t.Fatal(err)
	}

	c, _ := lib.Get("term-apiserver")

	got := strings.Join(ids(lib.Distractors(c, "2026-07-31")), ",")
	if want := "term-etcd,term-kubelet,term-probe"; got != want {
		t.Errorf("Distractors = %q, want the override %q", got, want)
	}
}

// A dangling or non-glossary distractor is fatal at load time: the alternative
// is a drill that silently offers fewer than four options, or none.
func TestDistractorsAreValidatedAtLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, body, want string
	}{
		{
			name: "unknown id",
			body: `
deck: G
cards:
  - id: term-a
    term: Alpha
    q: q
    a: a
    distractors: [term-nope]
`,
			want: `term-nope`,
		},
		{
			name: "not a glossary card",
			body: `
deck: G
cards:
  - id: term-a
    term: Alpha
    q: q
    a: a
    distractors: [plain]
  - id: plain
    q: q
    a: a
`,
			want: `plain`,
		},
		{
			// More distractors than a question has room for means the extras are
			// silently dropped. Say so at load rather than picking three at random.
			name: "more distractors than a question offers",
			body: `
deck: G
cards:
  - id: term-a
    term: Alpha
    q: q
    a: a
    distractors: [term-b, term-c, term-d, term-e]
  - id: term-b
    term: Bravo
    q: q
    a: a
  - id: term-c
    term: Charlie
    q: q
    a: a
  - id: term-d
    term: Delta
    q: q
    a: a
  - id: term-e
    term: Echo
    q: q
    a: a
`,
			want: `term-a`,
		},
		{
			name: "the card itself",
			body: `
deck: G
cards:
  - id: term-a
    term: Alpha
    q: q
    a: a
    distractors: [term-a]
`,
			want: `itself`,
		},
		{
			name: "distractors without a term",
			body: `
deck: G
cards:
  - id: plain
    q: q
    a: a
    distractors: [term-a]
  - id: term-a
    term: Alpha
    q: q
    a: a
`,
			want: `no 'term'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := loadDeck(tt.body)
			if err == nil {
				t.Fatal("expected a load error")
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should mention %q", err, tt.want)
			}
		})
	}
}

// The options have to be stable for as long as the card is on screen, and
// re-rolled often enough that the position of the right answer is not itself
// learnable. Seeding from the card id and the day gives both, with no clock to
// inject and no global rand state to reset.
func TestOptionsAreStablePerDay(t *testing.T) {
	t.Parallel()

	lib := distractorLib(t)

	c, _ := lib.Get("term-apiserver")

	first := strings.Join(ids(lib.Options(c, "2026-07-31")), ",")
	if again := strings.Join(ids(lib.Options(c, "2026-07-31")), ","); first != again {
		t.Errorf("same card and day gave %q then %q", first, again)
	}

	var rerolled bool

	for _, d := range []string{"2026-08-01", "2026-08-02", "2026-08-03", "2026-08-04"} {
		if strings.Join(ids(lib.Options(c, d)), ",") != first {
			rerolled = true
			break
		}
	}

	if !rerolled {
		t.Errorf("options %q never changed across four other days: the day is not in the seed", first)
	}
}

func TestOptionsContainTheAnswer(t *testing.T) {
	t.Parallel()

	lib := distractorLib(t)

	for _, id := range []string{"term-apiserver", "term-probe", "term-etcd"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			c, _ := lib.Get(id)

			got := ids(lib.Options(c, "2026-07-31"))
			if len(got) != 4 {
				t.Fatalf("Options(%s) = %v, want 4", id, got)
			}

			if !slices.Contains(got, id) {
				t.Errorf("Options(%s) = %v, missing the correct answer", id, got)
			}
		})
	}
}

// A concept card is not drilled by recognition, so it has no options at all.
func TestOptionsAreGlossaryOnly(t *testing.T) {
	t.Parallel()

	lib := distractorLib(t)

	c, _ := lib.Get("pairs-etcd-and-kubelet")
	if got := lib.Options(c, "2026-07-31"); got != nil {
		t.Errorf("Options(concept card) = %v, want nil", ids(got))
	}

	if got := lib.Distractors(c, "2026-07-31"); got != nil {
		t.Errorf("Distractors(concept card) = %v, want nil", ids(got))
	}
}

// Four options need four glossary cards. A glossary too small to fill them must
// degrade to what it has rather than repeat a term or panic.
func TestOptionsDegradeInATinyGlossary(t *testing.T) {
	t.Parallel()

	const body = `
deck: G
cards:
  - id: term-a
    term: Alpha
    q: q
    a: a
  - id: term-b
    term: Beta
    q: q
    a: b
`

	lib, err := loadDeck(body)
	if err != nil {
		t.Fatal(err)
	}

	c, _ := lib.Get("term-a")

	got := ids(lib.Options(c, "2026-07-31"))
	if len(got) != 2 {
		t.Fatalf("Options = %v, want the 2 cards that exist", got)
	}
}

// --- The glossary invariant --------------------------------------------------

// allowedUnrequiredTerms lists the decks not yet held to the invariant. Phase 2
// of the vocabulary spec cleans the glossary and M0; every other deck is
// drained as its module is reached, which is when its cards get rewritten
// anyway. The list only shrinks.
var allowedUnrequiredTerms = map[string]bool{
	"02-core-workloads":   true,
	"03-config-rbac":      true,
	"04-production":       true,
	"05-observability":    true,
	"06-client-go":        true,
	"07-crds-controllers": true,
	"08-cluster-auth":     true,
	"09-acronyms":         true,
	"10-kubectl-vs-ecs":   true,
	"11-eks-and-aws":      true,
	"12-capstone":         true,
}

// termUse matches a glossary term as a whole word, case sensitively.
//
// Case sensitivity keeps "KIND" the tool apart from "kind" the object field,
// and the word boundary is what stops a naive substring scan flagging
// "Service" inside "ServiceAccount". Hyphens count as word characters, so
// "proxy" does not match inside "kube-proxy".
func termUse(term string) *regexp.Regexp {
	const edge = `[^0-9A-Za-z_-]`
	return regexp.MustCompile(`(^|` + edge + `)` + regexp.QuoteMeta(term) + `($|` + edge + `)`)
}

// usesUnrequiredTerms returns the glossary terms a card uses without either
// requiring or defining them.
func usesUnrequiredTerms(c deck.Card, glossary map[string]deck.Card) []string {
	required := map[string]bool{}
	for _, id := range c.Requires {
		required[id] = true
	}

	text := c.Q + "\n" + c.A

	var found []string

	for term, owner := range glossary {
		switch {
		case owner.ID == c.ID: // the card defining the term
			continue
		case required[owner.ID]:
			continue
		case termUse(term).MatchString(text):
			found = append(found, term)
		}
	}

	sort.Strings(found)

	return found
}

// A card that uses a term it does not require is a card that arrives before its
// vocabulary is learnable. This is the defect the glossary tier exists to fix,
// and enforcing it here is what stops it reappearing.
func TestGlossaryTermsAreRequiredWhereUsed(t *testing.T) {
	t.Parallel()

	lib, err := deck.Load(flashcards.Decks, "decks")
	if err != nil {
		t.Fatal(err)
	}

	glossary := lib.Glossary()
	if len(glossary) == 0 {
		t.Fatal("no glossary terms found: the term cards are the point of this check")
	}

	files := map[string]bool{}

	for _, c := range lib.Cards {
		files[c.File] = true

		if allowedUnrequiredTerms[c.File] {
			continue
		}

		if terms := usesUnrequiredTerms(c, glossary); len(terms) > 0 {
			t.Errorf("card %q (%s) uses glossary terms it does not require: %s",
				c.ID, c.File, strings.Join(terms, ", "))
		}
	}

	// A stale allowlist entry is a silently disabled check.
	for name := range allowedUnrequiredTerms {
		if !files[name] {
			t.Errorf("allowlist names deck %q, which no longer exists", name)
		}
	}
}

func TestGlossaryLintFixtures(t *testing.T) {
	t.Parallel()

	const body = `
deck: Fixtures
cards:
  - id: term-service
    term: Service
    q: |
      Service
    a: |
      A stable virtual IP in front of a set of Pods.
  - id: requires-it
    q: |
      How does a Service find its endpoints?
    a: |
      By label selector.
    requires: [term-service]
  - id: uses-it-unrequired
    q: |
      What does a Service do?
    a: |
      Load balances.
  - id: longer-word
    q: |
      What is a ServiceAccount?
    a: |
      An in-cluster identity.
`

	lib, err := deck.Load(fstest.MapFS{"decks/f.yaml": {Data: []byte(body)}}, "decks")
	if err != nil {
		t.Fatal(err)
	}

	glossary := lib.Glossary()

	tests := []struct {
		id   string
		want string
	}{
		{"term-service", ""}, // the card defining the term
		{"requires-it", ""},  // uses it and requires it
		{"uses-it-unrequired", "Service"},
		{"longer-word", ""}, // Service inside ServiceAccount is not a use
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()

			c, ok := lib.Get(tt.id)
			if !ok {
				t.Fatalf("no card %q", tt.id)
			}

			got := strings.Join(usesUnrequiredTerms(c, glossary), ", ")
			if got != tt.want {
				t.Errorf("unrequired terms = %q, want %q", got, tt.want)
			}
		})
	}
}
