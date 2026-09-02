package deck_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	flashcards "github.com/iadk/k8s-flashcards"
	"github.com/iadk/k8s-flashcards/internal/deck"
)

// The embedded decks are the actual product here, so validating them is a real
// test rather than a formality: a typo'd id silently resets review history.
func TestEmbeddedDecksAreValid(t *testing.T) {
	t.Parallel()

	lib, err := deck.Load(flashcards.Decks, "decks")
	if err != nil {
		t.Fatalf("embedded decks failed to load: %v", err)
	}

	if len(lib.Cards) < 150 {
		t.Errorf("expected at least 150 cards, got %d", len(lib.Cards))
	}

	if len(lib.Decks) < 10 {
		t.Errorf("expected at least 10 decks, got %d", len(lib.Decks))
	}

	for _, c := range lib.Cards {
		if c.Deck == "" || c.File == "" {
			t.Errorf("card %q missing derived deck metadata", c.ID)
		}

		if len(c.Tags) == 0 {
			t.Errorf("card %q has no tags", c.ID)
		}
	}
}

// Every module in the curriculum should be drillable by name.
func TestEmbeddedDecksCoverCurriculumModules(t *testing.T) {
	t.Parallel()

	lib, err := deck.Load(flashcards.Decks, "decks")
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range []string{"M0", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "CAP"} {
		if got := lib.Select(deck.Filter{Module: m}); len(got) == 0 {
			t.Errorf("no cards for module %s", m)
		}
	}
}

func loadOne(t *testing.T, body string) (*deck.Library, error) {
	t.Helper()
	return deck.Load(fstest.MapFS{"decks/test.yaml": {Data: []byte(body)}}, "decks")
}

func TestParseRejectsMalformedDecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, yaml, wantErr string
	}{
		{
			name:    "no deck title",
			yaml:    "cards:\n  - id: a\n    q: q\n    a: a\n",
			wantErr: "missing top-level",
		},
		{
			name:    "no cards",
			yaml:    "deck: Empty\ncards: []\n",
			wantErr: "no cards",
		},
		{
			name:    "card without id",
			yaml:    "deck: D\ncards:\n  - q: q\n    a: a\n",
			wantErr: "has no id",
		},
		{
			name:    "card without answer",
			yaml:    "deck: D\ncards:\n  - id: a\n    q: q\n",
			wantErr: "needs both",
		},
		{
			name:    "not yaml at all",
			yaml:    "deck: [unclosed\n",
			wantErr: "parsing",
		},
		{
			name:    "two cards claim the same term",
			yaml:    "deck: D\ncards:\n  - id: a\n    term: Pod\n    q: q\n    a: a\n  - id: b\n    term: Pod\n    q: q\n    a: a\n",
			wantErr: `duplicate glossary term "Pod"`,
		},
		{
			name:    "an alias collides with another card's term",
			yaml:    "deck: D\ncards:\n  - id: a\n    term: Pod\n    q: q\n    a: a\n  - id: b\n    term: Deployment\n    aliases: [Pod]\n    q: q\n    a: a\n",
			wantErr: `duplicate glossary term "Pod"`,
		},
		{
			name:    "two cards share an alias",
			yaml:    "deck: D\ncards:\n  - id: a\n    term: Pod\n    aliases: [workload]\n    q: q\n    a: a\n  - id: b\n    term: Deployment\n    aliases: [workload]\n    q: q\n    a: a\n",
			wantErr: `duplicate glossary term "workload"`,
		},
		{
			name:    "an alias repeats its own term",
			yaml:    "deck: D\ncards:\n  - id: a\n    term: Pod\n    aliases: [Pod]\n    q: q\n    a: a\n",
			wantErr: `duplicate glossary term "Pod"`,
		},
		{
			name:    "aliases without a term",
			yaml:    "deck: D\ncards:\n  - id: a\n    aliases: [Pod]\n    q: q\n    a: a\n",
			wantErr: "has 'aliases' but no 'term'",
		},
		{
			name:    "a card is both a term and a checkpoint",
			yaml:    "deck: D\nmodule: M0\ncards:\n  - id: a\n    term: Pod\n    checkpoint: M0\n    q: q\n    a: a\n",
			wantErr: "cannot be both a glossary card and a checkpoint card",
		},
		{
			name:    "a checkpoint names a module with no cards",
			yaml:    "deck: D\nmodule: M0\ncards:\n  - id: a\n    checkpoint: M9\n    q: q\n    a: a\n",
			wantErr: `checkpoint card "a" names module "M9", which has no cards`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := loadOne(t, tt.yaml)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestDuplicateIDsAcrossDecksAreRejected(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"decks/a.yaml": {Data: []byte("deck: A\ncards:\n  - id: dupe\n    q: q\n    a: a\n")},
		"decks/b.yaml": {Data: []byte("deck: B\ncards:\n  - id: dupe\n    q: q\n    a: a\n")},
	}

	_, err := deck.Load(fsys, "decks")
	if err == nil || !strings.Contains(err.Error(), "duplicate card id") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestGlossaryTermsAndAliasesAreParsed(t *testing.T) {
	t.Parallel()

	lib, err := loadOne(t, "deck: D\ncards:\n  - id: term-a\n    term: kube-apiserver\n    aliases: [API server, apiserver]\n    q: q\n    a: a\n")
	if err != nil {
		t.Fatal(err)
	}

	c, _ := lib.Get("term-a")
	if c.Term != "kube-apiserver" {
		t.Errorf("Term = %q, want kube-apiserver", c.Term)
	}

	if len(c.Aliases) != 2 || c.Aliases[0] != "API server" {
		t.Errorf("Aliases = %v", c.Aliases)
	}
}

// The comparison fixture is composed from four parts so a case can swap one
// whole excerpt without a substitution that has to match every line of it.
const (
	validComparisonScenario = `    ecs_comparison:
      scenario: flashcards runs three replicas behind one stable endpoint.
`

	validComparisonJSON = `      ecs_json: |
        {"serviceName":"flashcards","desiredCount":3}
`

	validComparisonYAML = `      kubernetes_yaml: |
        apiVersion: apps/v1
        kind: Deployment
        metadata: {name: flashcards}
        spec:
          replicas: 3
          selector: {matchLabels: {app: flashcards}}
          template:
            metadata: {labels: {app: flashcards}}
            spec:
              containers: [{name: flashcards, image: flashcards:dev}]
        ---
        apiVersion: v1
        kind: Service
        metadata: {name: flashcards}
        spec:
          selector: {app: flashcards}
          ports: [{port: 8080, targetPort: 8080}]
`

	validComparisonAlignments = `      alignments:
        - ecs: service.desiredCount
          kubernetes: Deployment.spec.replicas
          mapping: direct
          caveat: Both set the desired replica count.
        - ecs: service load-balancer wiring
          kubernetes: Service.spec.selector and ports
          mapping: split
          caveat: Kubernetes puts the endpoint in a separate object.
      consequence: Scaling the Deployment does not change its stable endpoint.
      omissions: Health probes, resources, and rollout policy.
`

	validECSComparison = validComparisonScenario +
		validComparisonJSON +
		validComparisonYAML +
		validComparisonAlignments
)

// comparisonExcerptLineCeiling mirrors deck.maxComparisonExcerptLines, which
// these black-box tests cannot see. The oversized fixtures below are one line
// past it, so the two only stay in step if the ceiling is what the tests say.
const comparisonExcerptLineCeiling = 25

// excerptBlock renders body as a `key: |` block scalar indented to sit inside
// the comparison fixture.
func excerptBlock(key string, body []string) string {
	var b strings.Builder

	b.WriteString("      ")
	b.WriteString(key)
	b.WriteString(": |\n")

	for _, line := range body {
		b.WriteString("        ")
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// oversizedJSON and oversizedYAML are well-formed excerpts exactly one line
// past the ceiling, so size is the only thing they can be rejected for.
func oversizedJSON() string {
	body := []string{"{"}

	for i := range comparisonExcerptLineCeiling - 1 {
		separator := ","
		if i == comparisonExcerptLineCeiling-2 {
			separator = ""
		}

		body = append(body, fmt.Sprintf(`  "field%d": %d%s`, i, i, separator))
	}

	return excerptBlock("ecs_json", append(body, "}"))
}

func oversizedYAML() string {
	body := []string{"apiVersion: v1", "kind: ConfigMap", "metadata: {name: flashcards}", "data:"}
	for i := range comparisonExcerptLineCeiling - 3 {
		body = append(body, fmt.Sprintf(`  key%d: "%d"`, i, i))
	}

	return excerptBlock("kubernetes_yaml", body)
}

func TestECSComparisonParsesValidShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		comparison string
		want       bool
	}{
		{name: "comparison omitted"},
		{name: "complete multi-document comparison", comparison: validECSComparison, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := "deck: D\ncards:\n  - id: a\n    q: q\n    a: a\n" + tt.comparison

			lib, err := loadOne(t, body)
			if err != nil {
				t.Fatalf("loading comparison: %v", err)
			}

			card, ok := lib.Get("a")
			if !ok {
				t.Fatal("card was not loaded")
			}

			if (card.ECSComparison != nil) != tt.want {
				t.Fatalf("ECSComparison present = %t, want %t", card.ECSComparison != nil, tt.want)
			}

			if tt.want && len(card.ECSComparison.Alignments) != 2 {
				t.Errorf("alignments = %d, want 2", len(card.ECSComparison.Alignments))
			}
		})
	}
}

// rejectedComparison is one mutation of validECSComparison the loader must
// refuse, and the substring of the load error that says why.
type rejectedComparison struct {
	name       string
	comparison string
	wantErr    string
}

// comparisonSub swaps one part of the valid fixture, failing loudly when a
// fixture edit stops matching. Without that check a stale substitution silently
// yields the valid fixture, and the case reports the unhelpful "expected an
// error, got nil" instead of naming the real cause.
func comparisonSub(t *testing.T, old, replacement string) string {
	t.Helper()

	out := strings.Replace(validECSComparison, old, replacement, 1)
	if out == validECSComparison {
		t.Fatalf("fixture no longer contains %q", old)
	}

	return out
}

func runRejectedComparisons(t *testing.T, tests []rejectedComparison) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := "deck: D\ncards:\n  - id: a\n    q: q\n    a: a\n" + tt.comparison

			_, err := loadOne(t, body)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

// An excerpt is authored by hand and pasted from two different consoles, so the
// loader has to reject more than a parse failure: something that parses but is
// not object-shaped, silently loses a duplicated key, or has grown past the
// size a post-answer aside can carry.
func TestECSComparisonRejectsInvalidExcerpts(t *testing.T) {
	t.Parallel()

	const validJSONBody = `{"serviceName":"flashcards","desiredCount":3}`

	runRejectedComparisons(t, []rejectedComparison{
		{
			name:       "empty ecs json",
			comparison: comparisonSub(t, validComparisonJSON, "      ecs_json: ''\n"),
			wantErr:    "ecs_json must not be empty",
		},
		{
			name:       "malformed ecs json",
			comparison: comparisonSub(t, validJSONBody, "{not-json"),
			wantErr:    "ecs_json is not a json object",
		},
		{
			// json.Valid accepts a bare scalar, so syntax alone would let this pass.
			name:       "ecs json is not an object",
			comparison: comparisonSub(t, validJSONBody, "3"),
			wantErr:    "ecs_json is not a json object",
		},
		{
			name:       "empty ecs json object",
			comparison: comparisonSub(t, validJSONBody, "{}"),
			wantErr:    "ecs_json is an empty json object",
		},
		{
			name:       "ecs json past the line ceiling",
			comparison: comparisonSub(t, validComparisonJSON, oversizedJSON()),
			wantErr:    fmt.Sprintf("ecs_json exceeds %d lines", comparisonExcerptLineCeiling),
		},
		{
			name:       "empty kubernetes yaml",
			comparison: comparisonSub(t, validComparisonYAML, "      kubernetes_yaml: ''\n"),
			wantErr:    "kubernetes_yaml must not be empty",
		},
		{
			name:       "malformed kubernetes yaml",
			comparison: comparisonSub(t, "apiVersion: apps/v1", "apiVersion: [unclosed"),
			wantErr:    "kubernetes_yaml is not valid yaml",
		},
		{
			// A duplicate key drops a field rather than failing to parse, which is
			// why the excerpt is decoded by a parser that treats it as an error.
			name:       "duplicate key in kubernetes yaml",
			comparison: comparisonSub(t, "        kind: Deployment\n", "        kind: Deployment\n        kind: StatefulSet\n"),
			wantErr:    "kubernetes_yaml is not valid yaml",
		},
		{
			name:       "kubernetes yaml document is not a mapping",
			comparison: comparisonSub(t, validComparisonYAML, excerptBlock("kubernetes_yaml", []string{"- apps/v1", "- Deployment"})),
			wantErr:    "kubernetes_yaml document #1 is not a mapping",
		},
		{
			name:       "empty kubernetes yaml document",
			comparison: comparisonSub(t, "        ---\n", "        ---\n        ---\n"),
			wantErr:    "kubernetes_yaml document #2 is empty",
		},
		{
			name:       "kubernetes yaml document has no fields",
			comparison: comparisonSub(t, "        ---\n", "        ---\n        {}\n        ---\n"),
			wantErr:    "kubernetes_yaml document #2 is empty",
		},
		{
			name:       "kubernetes yaml holds only comments",
			comparison: comparisonSub(t, validComparisonYAML, excerptBlock("kubernetes_yaml", []string{"# nothing here"})),
			wantErr:    "kubernetes_yaml has no documents",
		},
		{
			name:       "kubernetes yaml past the line ceiling",
			comparison: comparisonSub(t, validComparisonYAML, oversizedYAML()),
			wantErr:    fmt.Sprintf("kubernetes_yaml exceeds %d lines", comparisonExcerptLineCeiling),
		},
	})
}

// The prose and alignment rows are what keep a comparison a post-answer aside
// rather than a second answer, so each is required and the row count is bounded.
func TestECSComparisonRejectsInvalidProseAndAlignments(t *testing.T) {
	t.Parallel()

	extraAlignments := strings.Repeat(`        - ecs: extra
          kubernetes: extra
          mapping: partial
          caveat: Extra alignment.
`, 3)

	secondAlignment := `        - ecs: service load-balancer wiring
          kubernetes: Service.spec.selector and ports
          mapping: split
          caveat: Kubernetes puts the endpoint in a separate object.
`
	oneAlignment := strings.Replace(validComparisonAlignments, secondAlignment, "", 1)

	runRejectedComparisons(t, []rejectedComparison{
		{
			name:       "empty scenario",
			comparison: comparisonSub(t, "scenario: flashcards runs three replicas behind one stable endpoint.", "scenario: ''"),
			wantErr:    "scenario must not be empty",
		},
		{
			name:       "empty consequence",
			comparison: comparisonSub(t, "consequence: Scaling the Deployment does not change its stable endpoint.", "consequence: ''"),
			wantErr:    "consequence must not be empty",
		},
		{
			name:       "empty omissions",
			comparison: comparisonSub(t, "omissions: Health probes, resources, and rollout policy.", "omissions: ''"),
			wantErr:    "omissions must not be empty",
		},
		{
			name:       "one alignment",
			comparison: comparisonSub(t, validComparisonAlignments, oneAlignment),
			wantErr:    "requires 2 to 4 alignments",
		},
		{
			name:       "five alignments",
			comparison: comparisonSub(t, "      consequence:", extraAlignments+"      consequence:"),
			wantErr:    "requires 2 to 4 alignments",
		},
		{
			name:       "empty alignment ecs",
			comparison: comparisonSub(t, "ecs: service.desiredCount", "ecs: ''"),
			wantErr:    "alignment #1 ecs must not be empty",
		},
		{
			name:       "empty alignment kubernetes",
			comparison: comparisonSub(t, "kubernetes: Deployment.spec.replicas", "kubernetes: ''"),
			wantErr:    "alignment #1 kubernetes must not be empty",
		},
		{
			name:       "empty alignment caveat",
			comparison: comparisonSub(t, "caveat: Both set the desired replica count.", "caveat: ''"),
			wantErr:    "alignment #1 caveat must not be empty",
		},
		{
			name:       "unknown mapping",
			comparison: comparisonSub(t, "mapping: direct", "mapping: approximate"),
			wantErr:    `alignment #1 mapping "approximate" is not supported`,
		},
	})
}

func TestRequiresResolvesAndRejectsBadGraphs(t *testing.T) {
	t.Parallel()

	card := func(id string, requires ...string) string {
		s := "  - id: " + id + "\n    q: q\n    a: a\n"
		if len(requires) > 0 {
			s += "    requires: [" + strings.Join(requires, ", ") + "]\n"
		}

		return s
	}

	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name:  "a resolvable edge across two decks",
			files: map[string]string{"a": card("term-x"), "b": card("concept", "term-x")},
		},
		{
			name:    "dangling requires",
			files:   map[string]string{"a": card("concept", "term-nope")},
			wantErr: `card "concept" requires unknown card "term-nope"`,
		},
		{
			name:    "self require",
			files:   map[string]string{"a": card("concept", "concept")},
			wantErr: "requires cycle",
		},
		{
			name:    "two-node cycle across files",
			files:   map[string]string{"a": card("one", "two"), "b": card("two", "one")},
			wantErr: "requires cycle",
		},
		{
			name:    "three-node cycle across files",
			files:   map[string]string{"a": card("one", "two"), "b": card("two", "three"), "c": card("three", "one")},
			wantErr: "requires cycle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fsys := fstest.MapFS{}
			for name, cards := range tt.files {
				fsys["decks/"+name+".yaml"] = &fstest.MapFile{Data: []byte("deck: " + name + "\ncards:\n" + cards)}
			}

			_, err := deck.Load(fsys, "decks")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected the graph to load, got %v", err)
				}

				return
			}

			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

// The cycle error has to name the cards in it, or a 60-card glossary graph is
// undebuggable.
func TestCycleErrorNamesTheCycle(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"decks/a.yaml": {Data: []byte("deck: A\ncards:\n  - id: one\n    q: q\n    a: a\n    requires: [two]\n")},
		"decks/b.yaml": {Data: []byte("deck: B\ncards:\n  - id: two\n    q: q\n    a: a\n    requires: [one]\n")},
	}

	_, err := deck.Load(fsys, "decks")
	if err == nil {
		t.Fatal("expected a cycle error")
	}

	for _, want := range []string{"one", "two"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("cycle error %q does not name card %q", err, want)
		}
	}
}

// A checkpoint card is the module's exam, so it depends on the whole module.
// Expanding that at load time keeps one rule for prerequisites: everything is a
// `requires:` edge, and the existing dangling/cycle validation covers it.
func TestCheckpointExpandsToModuleEdges(t *testing.T) {
	t.Parallel()

	const glossary = `
deck: Glossary
cards:
  - id: term-pod
    term: Pod
    q: q
    a: a
`

	const module = `
deck: Foundations
module: M0
cards:
  - id: m0-one
    q: q
    a: a
    requires: [term-pod]
  - id: m0-two
    q: q
    a: a
  - id: m0-checkpoint
    checkpoint: M0
    q: q
    a: a
    requires: [term-pod]
  - id: m0-checkpoint-two
    checkpoint: M0
    q: q
    a: a
`

	lib, err := deck.Load(fstest.MapFS{
		"decks/00-glossary.yaml":    {Data: []byte(glossary)},
		"decks/01-foundations.yaml": {Data: []byte(module)},
	}, "decks")
	if err != nil {
		t.Fatal(err)
	}

	c, ok := lib.Get(checkpointCard)
	if !ok {
		t.Fatal("no checkpoint card")
	}

	// Explicit edges are additive, and the two checkpoint cards must not require
	// each other or themselves — that would be a cycle, or an exam gated on an
	// exam.
	want := map[string]bool{"term-pod": true, moduleCard: true, "m0-two": true}

	got := map[string]bool{}
	for _, id := range c.Requires {
		got[id] = true
	}

	if len(got) != len(want) {
		t.Fatalf("requires = %v, want %v", c.Requires, want)
	}

	for id := range want {
		if !got[id] {
			t.Errorf("checkpoint card does not require %q; got %v", id, c.Requires)
		}
	}

	// Non-checkpoint cards keep exactly what they declared.
	if one, _ := lib.Get(moduleCard); len(one.Requires) != 1 || one.Requires[0] != "term-pod" {
		t.Errorf("m0-one.Requires = %v, want [term-pod]", one.Requires)
	}
}

// The Deck view of a card must not disagree with the Library view, or the index
// page and the drill would be reading two different graphs.
func TestCheckpointExpansionReachesTheDeckCards(t *testing.T) {
	t.Parallel()

	lib, err := loadOne(t, "deck: D\nmodule: M0\ncards:\n  - id: a\n    q: q\n    a: a\n  - id: cp\n    checkpoint: M0\n    q: q\n    a: a\n")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range lib.Decks[0].Cards {
		if c.ID != "cp" {
			continue
		}

		if len(c.Requires) != 1 || c.Requires[0] != "a" {
			t.Errorf("deck view of the checkpoint card has Requires = %v, want [a]", c.Requires)
		}
	}
}

func TestLoadEmptyDirIsAnError(t *testing.T) {
	t.Parallel()

	if _, err := deck.Load(fstest.MapFS{}, "decks"); err == nil {
		t.Fatal("expected an error for a directory with no decks")
	}
}

func TestDeckTagsMergeIntoCards(t *testing.T) {
	t.Parallel()

	lib, err := loadOne(t, "deck: D\ntags: [shared]\ncards:\n  - id: a\n    q: q\n    a: a\n    tags: [own]\n")
	if err != nil {
		t.Fatal(err)
	}

	c, _ := lib.Get("a")
	if len(c.Tags) != 2 || c.Tags[0] != "own" || c.Tags[1] != "shared" {
		t.Errorf("expected card and deck tags merged, got %v", c.Tags)
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"decks/01-one.yaml": {Data: []byte("deck: One\nmodule: M1\ncards:\n  - id: a\n    q: q\n    a: a\n    tags: [rbac]\n")},
		"decks/02-two.yaml": {Data: []byte("deck: Two\nmodule: M2\ncards:\n  - id: b\n    q: q\n    a: a\n    tags: [probes]\n")},
	}

	lib, err := deck.Load(fsys, "decks")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		filter deck.Filter
		want   int
	}{
		{"empty matches all", deck.Filter{}, 2},
		{"by module", deck.Filter{Module: "M1"}, 1},
		{"module is case insensitive", deck.Filter{Module: "m1"}, 1},
		{"by deck substring", deck.Filter{Deck: "02"}, 1},
		{"by tag", deck.Filter{Tag: "rbac"}, 1},
		{"combined miss", deck.Filter{Module: "M1", Tag: "probes"}, 0},
		{"unknown module", deck.Filter{Module: "M9"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := len(lib.Select(tt.filter)); got != tt.want {
				t.Errorf("got %d cards, want %d", got, tt.want)
			}
		})
	}
}
