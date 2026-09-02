package web_test

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	flashcards "github.com/iadk/k8s-flashcards"
	"github.com/iadk/k8s-flashcards/internal/deck"
	"github.com/iadk/k8s-flashcards/internal/review"
	"github.com/iadk/k8s-flashcards/internal/web"
)

const testDeck = `
deck: Test deck
module: M1
tags: [testing]
cards:
  - id: card-one
    q: |
      What is a **Pod**?
    a: |
      The smallest deployable unit.
    ecs: |
      An ECS Task.
    tags: [pod]
  - id: card-two
    q: |
      What is a Service?
    a: |
      A stable virtual IP.
    tags: [service]
`

const glossaryDeckFilename = "00-glossary.yaml"

const comparisonDeck = `
deck: Comparison deck
module: M1
cards:
  - id: compare-card
    q: What owns the replicas?
    a: The **Deployment** owns them.
    ecs: The nearest hook is an ECS Service, but networking is separate.
    ecs_comparison: &comparison
      scenario: "**flashcards** runs three replicas. <script>alert('scenario')</script>"
      ecs_json: |
        {"serviceName":"flashcards","image":"<unsafe>","desiredCount":3}
      kubernetes_yaml: |
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: flashcards
          annotations: {example: "<unsafe>"}
        spec:
          replicas: 3
          selector: {matchLabels: {app: flashcards}}
          template:
            metadata: {labels: {app: flashcards}}
            spec:
              containers: [{name: flashcards, image: flashcards:dev}]
      alignments:
        - ecs: "service.desiredCount"
          kubernetes: "Deployment.spec.replicas"
          mapping: direct
          caveat: "Both set the *desired* replica count."
        - ecs: "service load-balancer wiring"
          kubernetes: "Service selector and ports"
          mapping: split
          caveat: "Kubernetes puts routing in a **separate object**."
      consequence: "Scaling the Deployment does *not* change its endpoint."
      omissions: "Health probes, resources, and rollout policy."
  - id: compare-checkpoint
    checkpoint: M1
    q: Explain the ownership split.
    a: Use the rubric.
    ecs_comparison: *comparison
`

const comparisonGlossary = `
deck: Comparison glossary
cards:
  - id: term-pod
    term: Pod
    q: Pod
    a: The smallest deployable unit.
    ecs_comparison:
      scenario: "**flashcards** runs three replicas. <script>alert('scenario')</script>"
      ecs_json: |
        {"serviceName":"flashcards","image":"<unsafe>","desiredCount":3}
      kubernetes_yaml: |
        apiVersion: apps/v1
        kind: Deployment
        metadata: {name: flashcards, annotations: {example: "<unsafe>"}}
        spec:
          replicas: 3
          selector: {matchLabels: {app: flashcards}}
          template:
            metadata: {labels: {app: flashcards}}
            spec:
              containers: [{name: flashcards, image: flashcards:dev}]
      alignments:
        - {ecs: service.desiredCount, kubernetes: Deployment.spec.replicas, mapping: direct, caveat: "Both set the *desired* count."}
        - {ecs: service networking, kubernetes: Service, mapping: split, caveat: "Routing is a **separate object**."}
      consequence: "Scaling does *not* change the endpoint."
      omissions: "Health probes, resources, and rollout policy."
  - {id: term-node, term: node, q: node, a: A worker machine.}
  - {id: term-service, term: Service, q: Service, a: A stable endpoint.}
  - {id: term-deployment, term: Deployment, q: Deployment, a: A workload controller.}
`

func comparisonServer(t *testing.T) http.Handler {
	t.Helper()

	h, _, _ := serverOver(t, map[string]string{
		glossaryDeckFilename: comparisonGlossary,
		"02-core.yaml":       comparisonDeck,
	})

	return h
}

func assertECSComparison(t *testing.T, body string) {
	t.Helper()

	for _, want := range []string{
		"Coming from ECS",
		"ECS/Fargate JSON",
		"Kubernetes YAML",
		"service.desiredCount",
		"Deployment.spec.replicas",
		"direct",
		"Consequence",
		"Omissions",
		"<strong>flashcards</strong>",
		"<em>desired</em>",
		"<em>not</em>",
		"&lt;unsafe&gt;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("comparison does not contain %q:\n%s", want, body)
		}
	}

	for _, unsafe := range []string{"<script>alert('scenario')</script>", "<unsafe>"} {
		if strings.Contains(body, unsafe) {
			t.Errorf("comparison rendered unsafe input %q:\n%s", unsafe, body)
		}
	}
}

func TestECSComparisonRendersOnEveryCardSurface(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "free recall result", method: http.MethodPost, path: "/drill/compare-card/reveal"},
		{name: "recognition result", method: http.MethodPost, path: "/drill/term-pod/pick?p=term-pod"},
		{name: "browse", method: http.MethodGet, path: "/browse?deck=core"},
		{name: "checkpoint reveal", method: http.MethodPost, path: "/checkpoint/compare-checkpoint/reveal?module=M1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := do(t, comparisonServer(t), tt.method, tt.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s = %d: %s", tt.method, tt.path, rec.Code, rec.Body)
			}

			assertECSComparison(t, rec.Body.String())
		})
	}
}

func TestECSComparisonMarkupSupportsResponsiveOrdering(t *testing.T) {
	t.Parallel()

	body := do(t, comparisonServer(t), http.MethodGet, "/browse?deck=core").Body.String()
	for _, want := range []string{
		`class="ecs-comparison-grid"`,
		`class="ecs-comparison-excerpt ecs-comparison-ecs"`,
		`class="ecs-comparison-excerpt ecs-comparison-kubernetes"`,
		"@media (max-width: 48rem)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("responsive comparison markup does not contain %q", want)
		}
	}

	if strings.Index(body, "ecs-comparison-ecs") > strings.Index(body, "ecs-comparison-kubernetes") {
		t.Error("ECS excerpt must precede Kubernetes excerpt in document order")
	}
}

func newServer(t *testing.T) http.Handler {
	t.Helper()

	lib, err := deck.Load(fstest.MapFS{"decks/test.yaml": {Data: []byte(testDeck)}}, "decks")
	if err != nil {
		t.Fatalf("loading test deck: %v", err)
	}

	store, err := review.Open(filepath.Join(t.TempDir(), "review.json"), 20)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := web.New(lib, store, web.Config{})
	if err != nil {
		t.Fatalf("building server: %v", err)
	}

	return srv.Routes()
}

// unwritableServer builds a server whose review store cannot save, by taking
// away write permission on the directory holding it.
func unwritableServer(t *testing.T) (http.Handler, string) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permissions do not block writes")
	}

	lib, err := deck.Load(fstest.MapFS{"decks/test.yaml": {Data: []byte(testDeck)}}, "decks")
	if err != nil {
		t.Fatalf("loading test deck: %v", err)
	}

	dir := t.TempDir()

	store, err := review.Open(filepath.Join(dir, "review.json"), 20)
	if err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // G302: a directory needs its search bit; 0600 would make it unusable.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	// Restore write permission or TempDir cleanup cannot remove the directory.
	//nolint:gosec // G302: as above — directory permissions, not file permissions.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	srv, err := web.New(lib, store, web.Config{})
	if err != nil {
		t.Fatalf("building server: %v", err)
	}

	return srv.Routes(), dir
}

// A failing store must not narrate itself to the client: its errors carry the
// filesystem paths the process runs on, and the response is the one place those
// should never appear. The detail belongs in the logs.
func TestInternalErrorsAreNotLeakedToTheClient(t *testing.T) {
	t.Parallel()

	h, dir := unwritableServer(t)

	for _, tt := range []struct {
		name   string
		method string
		target string
		status int
	}{
		{"grading a card", http.MethodPost, "/drill/card-one/grade?g=3", http.StatusInternalServerError},
		{"readiness", http.MethodGet, "/readyz", http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := do(t, h, tt.method, tt.target)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}

			if body := rec.Body.String(); strings.Contains(body, dir) {
				t.Errorf("response leaks the store path %q:\n%s", dir, body)
			}
		})
	}
}

func do(tb testing.TB, h http.Handler, method, target string) *httptest.ResponseRecorder {
	tb.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(tb.Context(), method, target, nil))

	return rec
}

// Templates only fail at execution time, so rendering every page is the test
// that catches a bad field reference.
func TestPagesRender(t *testing.T) {
	t.Parallel()

	h := newServer(t)
	for _, path := range []string{"/", "/drill", "/drill?module=M1", "/browse", "/browse?tag=pod"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			rec := do(t, h, "GET", path)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, body: %s", path, rec.Code, rec.Body)
			}

			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("Content-Type = %q", ct)
			}
		})
	}
}

func TestDrillShowsQuestionButNotAnswer(t *testing.T) {
	t.Parallel()

	rec := do(t, newServer(t), "GET", "/drill")
	body := rec.Body.String()

	if !strings.Contains(body, "What is a") {
		t.Error("drill page should show a question")
	}

	if strings.Contains(body, "smallest deployable unit") {
		t.Error("drill page leaked the answer before reveal")
	}

	if !strings.Contains(body, "/reveal") {
		t.Error("drill page should offer a reveal action")
	}
}

func TestRevealReturnsAnswerAndEcsAnchor(t *testing.T) {
	t.Parallel()

	rec := do(t, newServer(t), "POST", "/drill/card-one/reveal")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	body := rec.Body.String()

	if !strings.Contains(body, "smallest deployable unit") {
		t.Error("reveal should include the answer")
	}

	if !strings.Contains(body, "An ECS Task") {
		t.Error("reveal should include the ECS anchor")
	}
	// Markdown should be rendered, not shown raw.
	if strings.Contains(body, "**Pod**") {
		t.Error("markdown was not rendered")
	}

	if !strings.Contains(body, "<strong>Pod</strong>") {
		t.Error("expected bold markup in the rendered question")
	}
}

func TestGradeAdvancesToNextCard(t *testing.T) {
	t.Parallel()

	h := newServer(t)

	rec := do(t, h, "POST", "/drill/card-one/grade?g=3")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	if !strings.Contains(rec.Body.String(), "card-two") {
		t.Error("grading should advance to the next card in scope")
	}
}

func TestGradeKeepsTheFilterScope(t *testing.T) {
	t.Parallel()

	h := newServer(t)
	rec := do(t, h, "POST", "/drill/card-one/grade?g=3&tag=service")

	body := rec.Body.String()
	if !strings.Contains(body, "tag=service") {
		t.Error("the drill scope should survive into the next fragment's URLs")
	}
}

func TestGradeRejectsBadInput(t *testing.T) {
	t.Parallel()

	h := newServer(t)

	tests := []struct {
		name, target string
		want         int
	}{
		{"unknown card", "/drill/nope/grade?g=3", http.StatusNotFound},
		{"grade too high", "/drill/card-one/grade?g=9", http.StatusBadRequest},
		{"grade too low", "/drill/card-one/grade?g=0", http.StatusBadRequest},
		{"grade missing", "/drill/card-one/grade", http.StatusBadRequest},
		{"grade not a number", "/drill/card-one/grade?g=good", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := do(t, h, "POST", tt.target).Code; got != tt.want {
				t.Errorf("POST %s = %d, want %d", tt.target, got, tt.want)
			}
		})
	}
}

func TestRevealUnknownCardIs404(t *testing.T) {
	t.Parallel()

	if got := do(t, newServer(t), "POST", "/drill/nope/reveal").Code; got != http.StatusNotFound {
		t.Errorf("status = %d, want 404", got)
	}
}

func TestExhaustedScopeShowsDoneNotAnError(t *testing.T) {
	t.Parallel()

	h := newServer(t)
	// One card in scope; grading it leaves nothing due.
	rec := do(t, h, "POST", "/drill/card-two/grade?g=4&tag=service")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	if !strings.Contains(rec.Body.String(), "Nothing due") {
		t.Errorf("expected a done fragment, got: %s", rec.Body)
	}
}

func TestHealthAndReadiness(t *testing.T) {
	t.Parallel()

	h := newServer(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		rec := do(t, h, "GET", path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, body: %s", path, rec.Code, rec.Body)
		}
	}

	if body := do(t, h, "GET", "/readyz").Body.String(); !strings.Contains(body, "2 cards") {
		t.Errorf("readiness should report what loaded, got %q", body)
	}
}

func TestStaticAssetIsServedLocally(t *testing.T) {
	t.Parallel()

	rec := do(t, newServer(t), "GET", "/static/htmx.min.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("htmx should be vendored and served locally, got %d", rec.Code)
	}

	if rec.Body.Len() < 1000 {
		t.Errorf("htmx asset looks truncated: %d bytes", rec.Body.Len())
	}
}

// fence is the markdown code fence, as a constant because a Go raw string
// literal cannot contain a backtick.
const fence = "```"

const diagramDeck = `
deck: Diagram deck
module: M1
cards:
  - id: card-with-diagram
    q: |
      What acts on an assignment?
    a: |
      The node agent does.

      ` + fence + `mermaid
      flowchart LR
        A[control plane] --> K[node agent]
      ` + fence + `
`

// The extension's default MermaidURL is a jsdelivr CDN link, and markdown()
// runs per card field, so the failure mode of losing NoScript or RenderMode is
// a script tag pointing at the internet — silent on any machine that has it.
// This holds the no-network requirement to a test rather than an observation
// made once at wiring time.
func TestMarkdownMermaidHasNoCDN(t *testing.T) {
	t.Parallel()

	h, _, _ := serverOver(t, map[string]string{"diagram.yaml": diagramDeck})

	rec := do(t, h, "POST", "/drill/card-with-diagram/reveal")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	body := rec.Body.String()
	// Without this the test passes vacuously the day the extension stops being
	// wired up at all.
	if !strings.Contains(body, `<pre class="mermaid">`) {
		t.Fatalf("the reveal rendered no mermaid container, so nothing here is being checked:\n%s", body)
	}

	if url := regexp.MustCompile(`https?://[^\s"'<]+`).FindString(body); url != "" {
		t.Errorf("the fragment references %q; MermaidJS is vendored under /static/", url)
	}

	if strings.Contains(body, "<script") {
		t.Errorf("the fragment carries its own script tag; layout.html holds the one copy:\n%s", body)
	}
}

// The real decks must render through the real templates, not just the fixture.
func TestRealDecksRenderEndToEnd(t *testing.T) {
	t.Parallel()

	lib, err := deck.Load(flashcards.Decks, "decks")
	if err != nil {
		t.Fatal(err)
	}

	store, err := review.Open(filepath.Join(t.TempDir(), "review.json"), 20)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := web.New(lib, store, web.Config{})
	if err != nil {
		t.Fatal(err)
	}

	h := srv.Routes()

	for _, path := range []string{"/browse", "/drill"} {
		if rec := do(t, h, "GET", path); rec.Code != http.StatusOK {
			t.Fatalf("GET %s over the real decks failed: %d", path, rec.Code)
		}
	}

	for _, c := range lib.Cards {
		rec := do(t, h, "POST", "/drill/"+c.ID+"/reveal")
		if rec.Code != http.StatusOK {
			t.Fatalf("revealing %q = %d: %s", c.ID, rec.Code, rec.Body)
		}
	}
}

// --- Prerequisite expansion --------------------------------------------------

// Glossary terms carry no module — a term like RBAC spans several — so a
// module-filtered drill has to pull in the terms it depends on or it dead-ends
// behind cards it can never unlock.
const gatedDecks = `
deck: Glossary
tags: [glossary]
cards:
  - id: term-pod
    term: Pod
    q: |
      Pod
    a: |
      The smallest deployable unit.
  - id: term-node
    term: node
    q: |
      node
    a: |
      A machine that runs workloads.
`

const gatedModule = `
deck: Foundations
module: M0
tags: [foundations]
cards:
  - id: m0-concept
    q: |
      What does a Deployment manage?
    a: |
      Replicas.
    requires: [term-pod]
  - id: m0-ungated-one
    q: |
      Why three machines?
    a: |
      One hides everything that matters.
  - id: m0-ungated-two
    q: |
      Where do Events live?
    a: |
      In the datastore, for about an hour.
`

// serverOver builds a server over the given decks, named by file so the deck
// number that orders them is part of the fixture.
func serverOver(t *testing.T, decks map[string]string) (http.Handler, *deck.Library, *review.Store) {
	t.Helper()

	return serverWith(t, decks, web.Config{})
}

// serverWith is serverOver with the server's collaborators supplied — the clock,
// for every caller here. It takes testing.TB so BenchmarkDrill can share the
// fixture rather than growing its own copy of it.
func serverWith(tb testing.TB, decks map[string]string, cfg web.Config) (http.Handler, *deck.Library, *review.Store) {
	tb.Helper()

	files := fstest.MapFS{}
	for name, body := range decks {
		files["decks/"+name] = &fstest.MapFile{Data: []byte(body)}
	}

	lib, err := deck.Load(files, "decks")
	if err != nil {
		tb.Fatal(err)
	}

	store, err := review.Open(filepath.Join(tb.TempDir(), "review.json"), 20)
	if err != nil {
		tb.Fatal(err)
	}

	srv, err := web.New(lib, store, cfg)
	if err != nil {
		tb.Fatal(err)
	}

	return srv.Routes(), lib, store
}

func gatedServer(t *testing.T) (http.Handler, *deck.Library) {
	t.Helper()

	h, lib, _ := serverOver(t, map[string]string{
		glossaryDeckFilename:  gatedDecks,
		"01-foundations.yaml": gatedModule,
	})

	return h, lib
}

func TestModuleDrillIntroducesItsPrerequisiteTerms(t *testing.T) {
	t.Parallel()

	h, _ := gatedServer(t)

	rec := do(t, h, "GET", "/drill?module=M0")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	body := rec.Body.String()
	if strings.Contains(body, "Nothing due") {
		t.Fatal("a fresh M0 drill reported an empty queue instead of teaching its vocabulary")
	}

	if !strings.Contains(body, "term-pod") {
		t.Errorf("expected the drill to introduce the prerequisite term, got: %s", body)
	}
}

func TestDrillScopeNamesThePulledInTermCount(t *testing.T) {
	t.Parallel()

	h, _ := gatedServer(t)

	body := do(t, h, "GET", "/drill?module=M0").Body.String()
	if !strings.Contains(body, "1 prerequisite term") {
		t.Errorf("drill scope should name how many prerequisite terms it pulled in, got: %s", body)
	}
}

// The index is an inventory: expanding a deck row would count the same glossary
// cards in every row, so the rows must sum to the library total.
// The index is an inventory: expanding a deck row would count the same glossary
// cards in every row, so the rendered rows must still sum to the library total.
func TestIndexDeckRowsAreNotExpanded(t *testing.T) {
	t.Parallel()

	h, lib := gatedServer(t)

	rec := do(t, h, "GET", "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	// Each row renders New, Locked and Total in that order, so every third
	// count is a row's own card total.
	counts := regexp.MustCompile(`<td class="num muted">(\d+)</td>`).
		FindAllStringSubmatch(rec.Body.String(), -1)
	if len(counts)%3 != 0 || len(counts)/3 != len(lib.Decks) {
		t.Fatalf("parsed %d counts for %d decks; the index markup changed shape",
			len(counts), len(lib.Decks))
	}

	var (
		sum    int
		totals []int
	)

	for i := 2; i < len(counts); i += 3 {
		n, err := strconv.Atoi(counts[i][1])
		if err != nil {
			t.Fatal(err)
		}

		totals = append(totals, n)
		sum += n
	}

	if sum != len(lib.Cards) {
		t.Errorf("rendered deck totals %v sum to %d, library has %d cards: the drill expansion leaked into the inventory",
			totals, sum, len(lib.Cards))
	}
}

// --- Multiple choice ---------------------------------------------------------

// A glossary big enough to fill four options, plus a concept card that must
// stay on the free-recall path.
const recognitionDeck = `
deck: Glossary
module: M0
tags: [glossary]
cards:
  - id: term-pod
    term: Pod
    tags: [workload]
    q: |
      Pod
    a: |
      The smallest deployable unit.
  - id: term-node
    term: node
    tags: [machine]
    q: |
      node
    a: |
      A machine that runs workloads.
  - id: term-etcd
    term: etcd
    tags: [storage]
    q: |
      etcd
    a: |
      The cluster datastore.
  - id: term-kubelet
    term: kubelet
    tags: [machine]
    q: |
      kubelet
    a: |
      The per-machine agent.
  - id: concept-one
    q: |
      Why is scheduling a placement decision?
    a: |
      Because capacity is finite.
`

func recognitionServer(t *testing.T) (http.Handler, *review.Store) {
	t.Helper()

	h, _, store := serverOver(t, map[string]string{glossaryDeckFilename: recognitionDeck})

	return h, store
}

// master drives a card to FSRS Review state, which is what flips it from
// recognition back to free recall.
func master(t *testing.T, store *review.Store, id string) {
	t.Helper()

	masterAt(t, store, id, time.Now())
}

// masterAt is master on a caller-chosen day, for a test whose server runs on a
// fixed clock: grading on real time would put the reviews on a different day
// from the one the server thinks it is.
func masterAt(t *testing.T, store *review.Store, id string, base time.Time) {
	t.Helper()

	for i := range 10 {
		if store.Mastered(id) {
			return
		}

		if _, err := store.Grade(id, review.Easy, base.AddDate(0, 0, i)); err != nil {
			t.Fatal(err)
		}
	}

	if !store.Mastered(id) {
		t.Fatalf("card %q never reached Review state", id)
	}
}

func TestNewGlossaryCardDrillsAsMultipleChoice(t *testing.T) {
	t.Parallel()

	h, _ := recognitionServer(t)

	rec := do(t, h, "GET", "/drill?tag=workload")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "/pick") {
		t.Fatalf("a new glossary card should offer options to pick from, got: %s", body)
	}

	if strings.Contains(body, "/reveal") {
		t.Error("a recognition rep should not offer a free-recall reveal")
	}

	if n := strings.Count(body, "/pick?"); n != 4 {
		t.Errorf("rendered %d options, want 4", n)
	}

	// Every option is a definition, and the right one is among them.
	if !strings.Contains(body, "smallest deployable unit") {
		t.Error("the correct definition should be one of the options")
	}
}

func TestRetainedGlossaryCardDrillsAsFreeRecall(t *testing.T) {
	t.Parallel()

	h, store := recognitionServer(t)
	master(t, store, "term-pod")

	rec := do(t, h, "POST", "/drill/term-pod/reveal?tag=workload")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "smallest deployable unit") {
		t.Error("a retained glossary card should reveal its answer as prose")
	}

	if strings.Contains(body, "/pick") {
		t.Error("a retained glossary card was still offered as multiple choice")
	}
}

// Recognition is objectively graded: the pick is right or wrong, and the rating
// follows from that rather than from self-assessment.
func TestPickGradesCorrectAsGoodAndWrongAsAgain(t *testing.T) {
	t.Parallel()

	nextDue := func(t *testing.T, pick string) time.Time {
		t.Helper()

		h, store := recognitionServer(t)

		rec := do(t, h, "POST", "/drill/term-pod/pick?p="+pick)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
		}

		return store.Stats([]deck.Card{{ID: "term-pod"}}, time.Now()).NextDue
	}

	right := nextDue(t, "term-pod")
	wrong := nextDue(t, "term-node")

	if right.IsZero() || wrong.IsZero() {
		t.Fatalf("a pick should schedule the card; got right=%v wrong=%v", right, wrong)
	}

	if !wrong.Before(right) {
		t.Errorf("a wrong pick (%v) should come back sooner than a correct one (%v)", wrong, right)
	}
}

// A wrong pick is a taught moment, not just an Again: the rep has to say which
// option was right.
func TestPickShowsTheCorrectAnswer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, pick, want string
	}{
		{"correct", "term-pod", "Correct"},
		{"wrong", "term-etcd", "Not quite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := recognitionServer(t)

			rec := do(t, h, "POST", "/drill/term-pod/pick?p="+tt.pick)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
			}

			body := rec.Body.String()
			if !strings.Contains(body, tt.want) {
				t.Errorf("expected the result to say %q, got: %s", tt.want, body)
			}

			if !strings.Contains(body, "smallest deployable unit") {
				t.Error("the result should show the correct definition")
			}

			if !strings.Contains(body, "/advance") {
				t.Error("the result should offer a way on to the next card")
			}
		})
	}
}

func TestAdvanceMovesToTheNextCard(t *testing.T) {
	t.Parallel()

	h, _ := recognitionServer(t)

	if rec := do(t, h, "POST", "/drill/term-pod/pick?p=term-pod"); rec.Code != http.StatusOK {
		t.Fatalf("pick = %d: %s", rec.Code, rec.Body)
	}

	rec := do(t, h, "POST", "/drill/term-pod/advance")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	if body := rec.Body.String(); strings.Contains(body, "term-pod/pick") {
		t.Errorf("advance re-served the card just answered: %s", body)
	}
}

func TestPickRejectsBadInput(t *testing.T) {
	t.Parallel()

	h, store := recognitionServer(t)
	master(t, store, "term-kubelet")

	tests := []struct {
		name, target string
		want         int
	}{
		{"unknown card", "/drill/nope/pick?p=term-pod", http.StatusNotFound},
		{"missing pick", "/drill/term-pod/pick", http.StatusBadRequest},
		{"pick is not an option", "/drill/term-pod/pick?p=concept-one", http.StatusBadRequest},
		{"concept card", "/drill/concept-one/pick?p=concept-one", http.StatusBadRequest},
		{"retained card", "/drill/term-kubelet/pick?p=term-kubelet", http.StatusBadRequest},
		{"advance on an unknown card", "/drill/nope/advance", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := do(t, h, "POST", tt.target).Code; got != tt.want {
				t.Errorf("POST %s = %d, want %d", tt.target, got, tt.want)
			}
		})
	}
}

// Concept answers are prose. Distractors for them would be hand-written
// falsehoods rehearsed as reading, so they stay on the free-recall path.
func TestConceptCardsAreNotMultipleChoice(t *testing.T) {
	t.Parallel()

	h, store := recognitionServer(t)
	for _, id := range []string{"term-pod", "term-node", "term-etcd", "term-kubelet"} {
		master(t, store, id)
	}

	rec := do(t, h, "POST", "/drill/concept-one/reveal")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	if body := rec.Body.String(); strings.Contains(body, "/pick") {
		t.Errorf("a concept card was offered as multiple choice: %s", body)
	}
}

// --- Checkpoints -------------------------------------------------------------

const checkpointModule = `
deck: Foundations
module: M0
tags: [foundations]
cards:
  - id: m0-one
    q: |
      Why three machines?
    a: |
      One hides everything that matters.
  - id: m0-two
    q: |
      Where do Events live?
    a: |
      In the datastore, for about an hour.
  - id: m0-checkpoint-split
    checkpoint: M0
    q: |
      Name the control-plane/worker split.
    a: |
      Rubric: full credit names both sides.
  - id: m0-checkpoint-contexts
    checkpoint: M0
    q: |
      What does a kubeconfig context select?
    a: |
      Rubric: full credit names cluster, user and namespace.
`

// checkpointDecks is the module the checkpoint tests sit: two ordinary cards to
// master and a two-question exam over them.
func checkpointDecks() map[string]string {
	return map[string]string{"01-foundations.yaml": checkpointModule}
}

func checkpointServer(t *testing.T) (http.Handler, *review.Store) {
	t.Helper()

	h, _, store := serverOver(t, checkpointDecks())

	return h, store
}

// checkpointServerAt is checkpointServer on a fixed clock, for the tests that
// assert on a date the server derived. Reading time.Now() in the test and
// comparing it against a date the server read for itself is a midnight away
// from failing.
func checkpointServerAt(t *testing.T, at time.Time) (http.Handler, *review.Store) {
	t.Helper()

	h, _, store := serverWith(t, checkpointDecks(), web.Config{Now: fixedClock(at)})

	return h, store
}

// masterM0 puts both ordinary M0 cards into FSRS Review state, which is the
// precondition for the checkpoint being offered at all.
func masterM0(t *testing.T, store *review.Store) {
	t.Helper()

	masterM0At(t, store, time.Now())
}

// masterM0At is masterM0 on a caller-chosen day. See masterAt.
func masterM0At(t *testing.T, store *review.Store, base time.Time) {
	t.Helper()

	for _, id := range []string{"m0-one", "m0-two"} {
		masterAt(t, store, id, base)
	}
}

// sitCheckpoint drives a whole session over HTTP, grading every card the same
// way, and returns the final fragment.
func sitCheckpoint(t *testing.T, h http.Handler, grade int) string {
	t.Helper()

	return sitCheckpointAs(t, h, "M0", grade)
}

// sitCheckpointAs is sitCheckpoint with the module spelled as the caller
// chooses, which is the whole point: the URL is where a non-canonical name
// gets in.
func sitCheckpointAs(t *testing.T, h http.Handler, module string, grade int) string {
	t.Helper()

	if rec := do(t, h, "GET", "/checkpoint?module="+module); rec.Code != http.StatusOK {
		t.Fatalf("opening the checkpoint = %d: %s", rec.Code, rec.Body)
	}

	body := ""

	for _, id := range []string{"m0-checkpoint-split", "m0-checkpoint-contexts"} {
		rec := do(t, h, "POST", "/checkpoint/"+id+"/grade?module="+module+"&g="+strconv.Itoa(grade))
		if rec.Code != http.StatusOK {
			t.Fatalf("grading %q = %d: %s", id, rec.Code, rec.Body)
		}

		body = rec.Body.String()
	}

	return body
}

func TestCheckpointIsLockedOnAFreshStore(t *testing.T) {
	t.Parallel()

	h, _ := checkpointServer(t)

	rec := do(t, h, "GET", "/checkpoint?module=M0")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "locked") {
		t.Errorf("a fresh checkpoint should render as locked, got: %s", body)
	}

	// The status has to name how much study is left, or "locked" is not
	// actionable.
	if !strings.Contains(body, "2") {
		t.Errorf("locked status should name the unmastered prerequisite count, got: %s", body)
	}

	// And it must not leak the exam.
	if strings.Contains(body, "control-plane/worker split") {
		t.Error("a locked checkpoint showed its questions")
	}
}

func TestCheckpointIsOfferedOnceTheModuleIsMastered(t *testing.T) {
	t.Parallel()

	h, store := checkpointServer(t)
	masterM0(t, store)

	body := do(t, h, "GET", "/checkpoint?module=M0").Body.String()
	if !strings.Contains(body, "control-plane/worker split") {
		t.Errorf("expected the first checkpoint question, got: %s", body)
	}

	if strings.Contains(body, "Rubric") {
		t.Error("the checkpoint page leaked the answer before reveal")
	}
}

func TestCheckpointPassIsRecordedAndShownOnTheIndex(t *testing.T) {
	t.Parallel()

	h, store := checkpointServerAt(t, clockStart)
	masterM0At(t, store, clockStart)

	if got := sitCheckpoint(t, h, int(review.Good)); !strings.Contains(got, "Passed") {
		t.Errorf("a clean sweep should report a pass, got: %s", got)
	}

	body := do(t, h, "GET", "/").Body.String()
	if !strings.Contains(body, "Passed on "+clockStart.Format("2006-01-02")) {
		t.Errorf("the index should show M0's checkpoint as passed, with the date; got: %s", body)
	}
}

func TestCheckpointPassIsRecordedUnderTheCanonicalModule(t *testing.T) {
	t.Parallel()

	h, lib, store := serverWith(t, checkpointDecks(), web.Config{Now: fixedClock(clockStart)})
	masterM0At(t, store, clockStart)

	if got := sitCheckpointAs(t, h, "m0", int(review.Good)); !strings.Contains(got, "Passed") {
		t.Errorf("a clean sweep should report a pass, got: %s", got)
	}

	// The module's own page, read through the canonical spelling.
	body := do(t, h, "GET", "/checkpoint?module=M0").Body.String()
	if !strings.Contains(body, "Passed") {
		t.Errorf("GET /checkpoint?module=M0 should show the pass recorded under m0, got: %s", body)
	}

	// The index.
	body = do(t, h, "GET", "/").Body.String()
	if !strings.Contains(body, "Passed on "+clockStart.Format("2006-01-02")) {
		t.Errorf("the index should show M0's checkpoint as passed; got: %s", body)
	}

	// The drill's withholding gate: withheld excludes an unpassed checkpoint
	// card from Total entirely, so seeing both cards counted proves the pass
	// released them.
	if got := store.Stats(lib.Checkpoints("M0"), clockStart).Total; got != 2 {
		t.Errorf("Stats(Checkpoints(\"M0\")).Total = %d, want 2 (both cards released)", got)
	}
}

func TestCheckpointPageLinksUseTheCanonicalModule(t *testing.T) {
	t.Parallel()

	h, store := checkpointServerAt(t, clockStart)
	masterM0At(t, store, clockStart)

	body := do(t, h, "GET", "/checkpoint?module=m0").Body.String()

	if !strings.Contains(body, "module=M0") {
		t.Errorf("checkpoint page should link with the canonical module, got: %s", body)
	}

	if strings.Contains(body, "module=m0") {
		t.Errorf("checkpoint page should not carry the non-canonical spelling forward, got: %s", body)
	}
}

func TestCheckpointFailureNamesTheRetryDate(t *testing.T) {
	t.Parallel()

	h, store := checkpointServerAt(t, clockStart)
	masterM0At(t, store, clockStart)

	if got := sitCheckpoint(t, h, int(review.Again)); !strings.Contains(got, "Failed") {
		t.Errorf("an Again should fail the attempt, got: %s", got)
	}

	tomorrow := clockStart.AddDate(0, 0, 1).Format("2006-01-02")

	// Both the retake page and the index have to say when it reopens.
	for _, path := range []string{"/checkpoint?module=M0", "/"} {
		body := do(t, h, "GET", path).Body.String()
		if !strings.Contains(body, tomorrow) {
			t.Errorf("GET %s should name the retry date %s, got: %s", path, tomorrow, body)
		}
	}
}

func TestCheckpointRejectsBadInput(t *testing.T) {
	t.Parallel()

	h, _ := checkpointServer(t)

	tests := []struct {
		name, method, target string
		want                 int
	}{
		{"unknown module", "GET", "/checkpoint?module=M9", http.StatusNotFound},
		{"no module", "GET", "/checkpoint", http.StatusNotFound},
		{"unknown card", "POST", "/checkpoint/nope/grade?module=M0&g=3", http.StatusNotFound},
		{"bad grade", "POST", "/checkpoint/m0-checkpoint-split/grade?module=M0&g=9", http.StatusBadRequest},
		{"grading a locked checkpoint", "POST", "/checkpoint/m0-checkpoint-split/grade?module=M0&g=3", http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := do(t, h, tt.method, tt.target).Code; got != tt.want {
				t.Errorf("%s %s = %d, want %d", tt.method, tt.target, got, tt.want)
			}
		})
	}
}

// The reveal step is what a checkpoint is graded against, rubric and all.
func TestCheckpointRevealShowsTheRubric(t *testing.T) {
	t.Parallel()

	h, store := checkpointServer(t)
	masterM0(t, store)

	if rec := do(t, h, "GET", "/checkpoint?module=M0"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}

	rec := do(t, h, "POST", "/checkpoint/m0-checkpoint-split/reveal?module=M0")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	if !strings.Contains(rec.Body.String(), "Rubric") {
		t.Errorf("reveal should show the answer and its rubric, got: %s", rec.Body)
	}
}

// A checkpoint is a synthesis question with a rubric, not a term with siblings.
// Recognition must not have reached it.
func TestCheckpointCardsStayFreeRecall(t *testing.T) {
	t.Parallel()

	h, store := checkpointServer(t)
	masterM0(t, store)

	body := do(t, h, "GET", "/checkpoint?module=M0").Body.String()
	if strings.Contains(body, "/pick") {
		t.Errorf("a checkpoint card was offered as multiple choice: %s", body)
	}

	if !strings.Contains(body, "/reveal") {
		t.Errorf("a checkpoint card should still be revealed and self-graded, got: %s", body)
	}
}

// An unpassed checkpoint must never reach the drill.
func TestCheckpointCardsAreNotDrilledBeforeAPass(t *testing.T) {
	t.Parallel()

	h, store := checkpointServer(t)
	masterM0(t, store)

	body := do(t, h, "GET", "/drill?module=M0").Body.String()
	if strings.Contains(body, "control-plane/worker split") {
		t.Error("the drill served an unpassed checkpoint card")
	}
}

// Off by default has to mean absent, not merely inert: the deployed image ships
// this build, and a route that exists but is unconfigured is a runtime failure
// waiting for the first curious request.
func TestChatSurfaceIsAbsentWithoutAProvider(t *testing.T) {
	t.Parallel()

	h := newServer(t)

	for _, target := range []string{"/chat", "/chat/reset"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			if rec := do(t, h, http.MethodPost, target); rec.Code != http.StatusNotFound {
				t.Errorf("POST %s = %d, want %d", target, rec.Code, http.StatusNotFound)
			}
		})
	}

	t.Run("drill page carries no panel", func(t *testing.T) {
		t.Parallel()

		body := do(t, h, http.MethodGet, "/drill").Body.String()
		for _, marker := range []string{"chat-form", "chat.js", `id="chat"`} {
			if strings.Contains(body, marker) {
				t.Errorf("drill page contains chat markup %q", marker)
			}
		}
	})
}

// --- The clock ---------------------------------------------------------------

// clockStart is the instant the clocks in this file start at.
//
// It is explicitly UTC. review.Day formats an instant in that instant's own
// location, so a time.Local start would put the day boundary somewhere different
// on a CI box in another zone — the same class of latent flakiness these clocks
// exist to remove.
var clockStart = time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)

// steppingClock returns a clock starting at start that advances by step on every
// read, and a func reporting how many reads have happened.
//
// The step is the point, not the count: a handler that reads twice sees two
// different instants, and far enough apart they are two different days. The
// count is just the cheapest way to observe that from outside.
//
// The counter is atomic because every test here runs in parallel and nothing
// promises a handler is served on the goroutine that asked.
func steppingClock(start time.Time, step time.Duration) (func() time.Time, func() int) {
	var reads atomic.Int64

	now := func() time.Time { return start.Add(time.Duration(reads.Add(1)-1) * step) }

	return now, func() int { return int(reads.Load()) }
}

// fixedClock returns a clock frozen at at, for a test asserting on a date rather
// than on how often the clock was read.
func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

// The clock fixture has to reach every route, so it pairs a glossary still being
// recognised — which is what /drill and /pick need — with a module whose
// ordinary cards are mastered, which is what makes its checkpoint sittable.
// Terms carry no module, so mastering M0 leaves them untouched.
const clockGlossary = `
deck: Glossary
tags: [glossary]
cards:
  - id: term-pod
    term: Pod
    q: |
      Pod
    a: |
      The smallest deployable unit.
  - id: term-node
    term: node
    q: |
      node
    a: |
      A machine that runs workloads.
`

// clockDecks is one fixture that reaches every route: the glossary above for
// /drill and /pick, and the checkpoint module for the three checkpoint routes.
func clockDecks() map[string]string {
	decks := map[string]string{glossaryDeckFilename: clockGlossary}
	maps.Copy(decks, checkpointDecks())

	return decks
}

// drillPath is the unfiltered drill route, named because the tests below reach
// for it often enough that repeating the literal reads as three separate strings
// rather than one route.
const drillPath = "/drill"

// clockServer builds a server whose clock counts its reads, with M0 mastered so
// the checkpoint routes are reachable.
func clockServer(t *testing.T) (http.Handler, func() int) {
	t.Helper()

	now, reads := steppingClock(clockStart, time.Second)

	h, _, store := serverWith(t, clockDecks(), web.Config{Now: now})

	masterM0At(t, store, clockStart)

	return h, reads
}

// The day boundary is where a second clock read stops being a style question.
// A request that starts at 23:59:59 and reads the clock again a moment later
// renders one day's card beside another day's counts.
func TestDrillIsConsistentAcrossMidnight(t *testing.T) {
	t.Parallel()

	midnight := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	// A tenth of a second before midnight, stepping by a second: the first read
	// lands on the day before, every read after it on the day after.
	now, _ := steppingClock(midnight.Add(-100*time.Millisecond), time.Second)

	h, _, store := serverWith(t, map[string]string{"test.yaml": testDeck}, web.Config{Now: now})

	// One review recorded an hour earlier, so the day the request began on has a
	// count the day after it does not.
	if _, err := store.Grade("card-two", review.Good, midnight.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	rec := do(t, h, http.MethodGet, drillPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	if body := rec.Body.String(); !strings.Contains(body, "1 reviewed today") {
		t.Errorf("a request that began before midnight rendered counts from after it:\n%s", body)
	}
}

// The counts a grade returns have to include that grade, which fixes where the
// scope snapshot is built: after the write, never at the top of the handler.
// Nothing else in this suite asserts a value in the drill-stats bar, so nothing
// else catches getting that order backwards.
func TestGradeCountsTheAnswerItJustRecorded(t *testing.T) {
	t.Parallel()

	h, _, _ := serverWith(t,
		map[string]string{"test.yaml": testDeck},
		web.Config{Now: fixedClock(clockStart)})

	rec := do(t, h, http.MethodPost, "/drill/card-one/grade?g=3")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}

	if body := rec.Body.String(); !strings.Contains(body, "1 reviewed today") {
		t.Errorf("the fragment a grade returns does not count that grade:\n%s", body)
	}
}

// BenchmarkDrill measures a whole drill request: scope selection, prerequisite
// expansion, stats, card selection and rendering. It is the measurement behind
// "the scope is computed once" — recomputing it is invisible in the response but
// shows up here as allocations.
func BenchmarkDrill(b *testing.B) {
	h, _, _ := serverWith(b, clockDecks(), web.Config{Now: fixedClock(clockStart)})

	// A frozen clock and a read-only route, so every iteration does the same work
	// over the same store state.
	req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, drillPath, nil)

	b.ReportAllocs()

	for b.Loop() {
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("GET %s = %d: %s", drillPath, rec.Code, rec.Body)
		}
	}
}

// One request, one view of time. Two clock reads in a handler are two different
// instants, and any value keyed on the day — a review counter, a streak, a daily
// option seed — can then disagree with itself inside a single rendered page.
// This is the only durable form that rule takes: it fails the build when a
// second read appears anywhere down the call stack.
func TestHandlersReadTheClockOnce(t *testing.T) {
	t.Parallel()

	// A checkpoint card can only be revealed or graded inside an open attempt,
	// which is what opening the checkpoint page starts.
	openCheckpoint := func(t *testing.T, h http.Handler) {
		t.Helper()

		if rec := do(t, h, http.MethodGet, "/checkpoint?module=M0"); rec.Code != http.StatusOK {
			t.Fatalf("opening the checkpoint = %d: %s", rec.Code, rec.Body)
		}
	}

	tests := []struct {
		name, method, target string
		// setup runs before the request under test, for a route that needs state
		// another route creates. Its own clock reads are not counted.
		setup func(t *testing.T, h http.Handler)
		reads int
	}{
		{name: "index", method: http.MethodGet, target: "/", reads: 1},
		{name: "browse", method: http.MethodGet, target: "/browse", reads: 0},
		{name: "liveness", method: http.MethodGet, target: "/healthz", reads: 0},
		{name: "readiness", method: http.MethodGet, target: "/readyz", reads: 0},
		{name: "drill", method: http.MethodGet, target: drillPath, reads: 1},
		{name: "reveal", method: http.MethodPost, target: "/drill/term-pod/reveal", reads: 1},
		{name: "grade", method: http.MethodPost, target: "/drill/m0-one/grade?g=3", reads: 1},
		{name: "pick", method: http.MethodPost, target: "/drill/term-pod/pick?p=term-pod", reads: 1},
		{name: "advance", method: http.MethodPost, target: "/drill/term-pod/advance", reads: 1},
		{name: "checkpoint", method: http.MethodGet, target: "/checkpoint?module=M0", reads: 1},
		{
			name: "checkpoint reveal", method: http.MethodPost,
			target: "/checkpoint/m0-checkpoint-split/reveal?module=M0",
			setup:  openCheckpoint, reads: 1,
		},
		{
			name: "checkpoint grade", method: http.MethodPost,
			target: "/checkpoint/m0-checkpoint-split/grade?module=M0&g=3",
			setup:  openCheckpoint, reads: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, reads := clockServer(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			before := reads()

			rec := do(t, h, tt.method, tt.target)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s = %d: %s", tt.method, tt.target, rec.Code, rec.Body)
			}

			if got := reads() - before; got != tt.reads {
				t.Errorf("%s %s read the clock %d times, want %d",
					tt.method, tt.target, got, tt.reads)
			}
		})
	}
}
