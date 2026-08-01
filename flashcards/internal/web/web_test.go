package web_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

	srv, err := web.New(lib, store)
	if err != nil {
		t.Fatalf("building server: %v", err)
	}

	return srv.Routes()
}

func do(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, target, nil))

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

	srv, err := web.New(lib, store)
	if err != nil {
		t.Fatal(err)
	}

	h := srv.Routes()

	if rec := do(t, h, "GET", "/browse"); rec.Code != http.StatusOK {
		t.Fatalf("browsing every real card failed: %d", rec.Code)
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

func gatedServer(t *testing.T) (http.Handler, *deck.Library) {
	t.Helper()

	lib, err := deck.Load(fstest.MapFS{
		"decks/00-glossary.yaml":    {Data: []byte(gatedDecks)},
		"decks/01-foundations.yaml": {Data: []byte(gatedModule)},
	}, "decks")
	if err != nil {
		t.Fatal(err)
	}

	store, err := review.Open(filepath.Join(t.TempDir(), "review.json"), 20)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := web.New(lib, store)
	if err != nil {
		t.Fatal(err)
	}

	return srv.Routes(), lib
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

func checkpointServer(t *testing.T) (http.Handler, *review.Store) {
	t.Helper()

	lib, err := deck.Load(fstest.MapFS{
		"decks/01-foundations.yaml": {Data: []byte(checkpointModule)},
	}, "decks")
	if err != nil {
		t.Fatal(err)
	}

	store, err := review.Open(filepath.Join(t.TempDir(), "review.json"), 20)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := web.New(lib, store)
	if err != nil {
		t.Fatal(err)
	}

	return srv.Routes(), store
}

// masterM0 puts both ordinary M0 cards into FSRS Review state, which is the
// precondition for the checkpoint being offered at all.
func masterM0(t *testing.T, store *review.Store) {
	t.Helper()

	now := time.Now()

	for _, id := range []string{"m0-one", "m0-two"} {
		for i := range 10 {
			if store.Mastered(id) {
				break
			}

			if _, err := store.Grade(id, review.Easy, now.AddDate(0, 0, i)); err != nil {
				t.Fatal(err)
			}
		}

		if !store.Mastered(id) {
			t.Fatalf("card %q never reached Review state", id)
		}
	}
}

// sitCheckpoint drives a whole session over HTTP, grading every card the same
// way, and returns the final fragment.
func sitCheckpoint(t *testing.T, h http.Handler, grade int) string {
	t.Helper()

	if rec := do(t, h, "GET", "/checkpoint?module=M0"); rec.Code != http.StatusOK {
		t.Fatalf("opening the checkpoint = %d: %s", rec.Code, rec.Body)
	}

	body := ""

	for _, id := range []string{"m0-checkpoint-split", "m0-checkpoint-contexts"} {
		rec := do(t, h, "POST", "/checkpoint/"+id+"/grade?module=M0&g="+strconv.Itoa(grade))
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

	h, store := checkpointServer(t)
	masterM0(t, store)

	if got := sitCheckpoint(t, h, int(review.Good)); !strings.Contains(got, "Passed") {
		t.Errorf("a clean sweep should report a pass, got: %s", got)
	}

	body := do(t, h, "GET", "/").Body.String()
	if !strings.Contains(body, "Passed on "+time.Now().Format("2006-01-02")) {
		t.Errorf("the index should show M0's checkpoint as passed, with the date; got: %s", body)
	}
}

func TestCheckpointFailureNamesTheRetryDate(t *testing.T) {
	t.Parallel()

	h, store := checkpointServer(t)
	masterM0(t, store)

	if got := sitCheckpoint(t, h, int(review.Again)); !strings.Contains(got, "Failed") {
		t.Errorf("an Again should fail the attempt, got: %s", got)
	}

	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

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
