package review_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	flashcards "github.com/iadk/k8s-flashcards"
	"github.com/iadk/k8s-flashcards/internal/deck"
	"github.com/iadk/k8s-flashcards/internal/review"
)

func cards(ids ...string) []deck.Card {
	out := make([]deck.Card, 0, len(ids))
	for _, id := range ids {
		out = append(out, deck.Card{ID: id, Q: "q", A: "a"})
	}

	return out
}

func openStore(t *testing.T, newPerDay int) (*review.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "review.json")

	s, err := review.Open(path, newPerDay)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	return s, path
}

// The whole point of persisting state is that it survives a restart — which on
// Kubernetes means surviving a rescheduled Pod, provided there's a volume.
func TestScheduleSurvivesRestart(t *testing.T) {
	t.Parallel()

	s, path := openStore(t, 20)
	now := time.Now()

	due, err := s.Grade("m1-pod", review.Easy, now)
	if err != nil {
		t.Fatalf("grading: %v", err)
	}

	if !due.After(now) {
		t.Fatalf("Easy should schedule into the future, got %v", due)
	}

	reopened, err := review.Open(path, 20)
	if err != nil {
		t.Fatalf("reopening store: %v", err)
	}

	st := reopened.Stats(cards("m1-pod"), now)
	if st.New != 0 {
		t.Errorf("card should not be new after restart, stats: %+v", st)
	}

	if st.Due != 0 {
		t.Errorf("card was scheduled into the future, should not be due: %+v", st)
	}

	if _, ok := reopened.Next(cards("m1-pod"), now); ok {
		t.Error("no card should be due immediately after grading Easy")
	}
}

func TestAgainReturnsSoonerThanGood(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	againDue, err := s.Grade("card-again", review.Again, now)
	if err != nil {
		t.Fatal(err)
	}

	goodDue, err := s.Grade("card-good", review.Good, now)
	if err != nil {
		t.Fatal(err)
	}

	if !againDue.Before(goodDue) {
		t.Errorf("Again (%v) should come back before Good (%v)", againDue, goodDue)
	}
}

func TestDueCardIsPreferredOverNew(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	if _, err := s.Grade("seen", review.Again, now); err != nil {
		t.Fatal(err)
	}

	// An Again-graded card is due within minutes; look slightly ahead.
	later := now.Add(30 * time.Minute)

	got, ok := s.Next(cards("unseen", "seen"), later)
	if !ok {
		t.Fatal("expected a card")
	}

	if got.ID != "seen" {
		t.Errorf("due card should win over a new one, got %q", got.ID)
	}
}

func TestNewCardsAreCappedPerDay(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 2)
	now := time.Now()

	for i, id := range []string{"a", "b"} {
		if _, err := s.Grade(id, review.Good, now); err != nil {
			t.Fatalf("grading card %d: %v", i, err)
		}
	}

	if _, ok := s.Next(cards("c"), now); ok {
		t.Error("third new card should be withheld once the daily cap is reached")
	}

	tomorrow := now.AddDate(0, 0, 1)
	if _, ok := s.Next(cards("c"), tomorrow); !ok {
		t.Error("the cap should reset the next day")
	}
}

func TestInvalidGradeIsRejected(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	for _, g := range []review.Grade{0, 5, -1} {
		if _, err := s.Grade("x", g, time.Now()); err == nil {
			t.Errorf("grade %d should have been rejected", g)
		}
	}
}

func TestCramIgnoresSchedulingAndAvoidsRepeats(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	// Schedule both far into the future, so nothing is due.
	for _, id := range []string{"a", "b"} {
		if _, err := s.Grade(id, review.Easy, now); err != nil {
			t.Fatal(err)
		}
	}

	all := cards("a", "b")

	if _, ok := s.Next(all, now); ok {
		t.Fatal("precondition: nothing should be due")
	}

	got, ok := s.Cram(all, "")
	if !ok {
		t.Fatal("cram should return a card even when nothing is due")
	}

	if next, ok := s.Cram(all, got.ID); !ok || next.ID == got.ID {
		t.Errorf("cram should not immediately repeat %q", got.ID)
	}
}

func TestStatsCountsScopeOnly(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)

	now := time.Now()
	if _, err := s.Grade("a", review.Good, now); err != nil {
		t.Fatal(err)
	}

	st := s.Stats(cards("a", "b", "c"), now)
	if st.Total != 3 {
		t.Errorf("Total = %d, want 3", st.Total)
	}

	if st.New != 2 {
		t.Errorf("New = %d, want 2", st.New)
	}

	if st.ReviewsToday != 1 {
		t.Errorf("ReviewsToday = %d, want 1", st.ReviewsToday)
	}

	if st.NextDue.IsZero() {
		t.Error("NextDue should be set for the scheduled card")
	}
}

func TestStreakCountsConsecutiveDays(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 50)
	now := time.Now()

	for i := range 3 {
		day := now.AddDate(0, 0, -i)
		if _, err := s.Grade("card", review.Good, day); err != nil {
			t.Fatal(err)
		}
	}

	if got := s.Streak(now); got != 3 {
		t.Errorf("Streak = %d, want 3", got)
	}

	// A gap ends the streak.
	s2, _ := openStore(t, 50)
	if _, err := s2.Grade("card", review.Good, now.AddDate(0, 0, -5)); err != nil {
		t.Fatal(err)
	}

	if got := s2.Streak(now); got != 0 {
		t.Errorf("Streak after a 5-day gap = %d, want 0", got)
	}
}

// Unhappy path: the volume is gone or read-only. The store must report that
// rather than pretend the review was saved — this is what the readiness probe
// keys off.
func TestWritableFailsOnReadOnlyDir(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits are not enforced")
	}

	dir := t.TempDir()

	s, err := review.Open(filepath.Join(dir, "review.json"), 20)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Writable(); err != nil {
		t.Fatalf("precondition: store should start writable, got %v", err)
	}

	//nolint:gosec // G302: making the dir read-only is the point of this test.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G302: 0o700 restores owner access so t.TempDir cleanup can remove the dir.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := s.Writable(); err == nil {
		t.Error("expected Writable to fail on a read-only directory")
	}

	if _, err := s.Grade("a", review.Good, time.Now()); err == nil {
		t.Error("expected Grade to fail when state cannot be persisted")
	}
}

func TestOpenRejectsCorruptState(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := review.Open(path, 20); err == nil {
		t.Error("expected an error for corrupt review state, not a silent reset")
	}
}

// --- Prerequisite gating -----------------------------------------------------

// gated builds "concept requires term", the smallest shape the gate has to get
// right.
func gated() []deck.Card {
	return []deck.Card{
		{ID: "term", Q: "q", A: "a"},
		{ID: "concept", Q: "q", A: "a", Requires: []string{"term"}},
	}
}

// master grades a card until FSRS puts it in Review state — the mastery signal
// the gate reads. Easy from new usually gets there in one, but the number of
// steps is FSRS's business, not this test's.
func master(t *testing.T, s *review.Store, id string, now time.Time) {
	t.Helper()

	for i := range 10 {
		if s.Mastered(id) {
			return
		}

		if _, err := s.Grade(id, review.Easy, now.Add(time.Duration(i)*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	t.Fatalf("card %q never reached Review state", id)
}

func TestNextWithholdsCardsWithUnmasteredPrerequisites(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	got, ok := s.Next(gated(), now)
	if !ok || got.ID != "term" {
		t.Fatalf("Next introduced %q (ok=%v), want the prerequisite term first", got.ID, ok)
	}

	// Seeing the term is not enough; it has to be retained.
	if _, err := s.Grade("term", review.Again, now); err != nil {
		t.Fatal(err)
	}

	if got, ok := s.Next([]deck.Card{gated()[1]}, now.Add(time.Hour)); ok {
		t.Errorf("Next returned %q while its prerequisite is still being learned", got.ID)
	}

	master(t, s, "term", now)

	got, ok = s.Next([]deck.Card{gated()[1]}, now.Add(30*24*time.Hour))
	if !ok || got.ID != "concept" {
		t.Errorf("Next = %q (ok=%v), want concept once its prerequisite is mastered", got.ID, ok)
	}
}

func TestStatsCountsLockedCards(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	st := s.Stats(gated(), now)
	if st.Locked != 1 {
		t.Errorf("Locked = %d, want 1 (concept is behind an unmastered term)", st.Locked)
	}

	if st.New != 2 {
		t.Errorf("New = %d, want 2: a locked card is still unseen, not a separate bucket", st.New)
	}

	master(t, s, "term", now)

	if st := s.Stats(gated(), now); st.Locked != 0 {
		t.Errorf("Locked = %d after mastering the prerequisite, want 0", st.Locked)
	}
}

// Cram is the night-before escape hatch. Gating it would remove it.
func TestCramIgnoresPrerequisites(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)

	got, ok := s.Cram([]deck.Card{gated()[1]}, "")
	if !ok || got.ID != "concept" {
		t.Errorf("Cram = %q (ok=%v), want concept regardless of its prerequisites", got.ID, ok)
	}
}

// Satisfaction is read from the store globally, so narrowing the card set can
// never make an unmastered prerequisite look satisfied.
func TestGatingIsNotFooledByANarrowSelection(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)

	if got, ok := s.Next([]deck.Card{gated()[1]}, time.Now()); ok {
		t.Errorf("Next = %q for a selection containing only the locked card", got.ID)
	}
}

func TestMasteredReportsReviewState(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	if s.Mastered("term") {
		t.Error("an unseen card should not count as mastered")
	}

	if _, err := s.Grade("term", review.Again, now); err != nil {
		t.Fatal(err)
	}

	if s.Mastered("term") {
		t.Error("a card just failed should not count as mastered")
	}

	master(t, s, "term", now)

	if !s.Mastered("term") {
		t.Error("a card in Review state should count as mastered")
	}
}

// --- Checkpoints -------------------------------------------------------------

// checkpointDeck is "two module cards plus a two-card exam over them" — the
// smallest shape the checkpoint state machine has to get right. The exam's edges
// are what deck.Load synthesizes from `checkpoint: M0`.
func checkpointDeck() []deck.Card {
	requires := []string{"m0-a", "m0-b"}

	return []deck.Card{
		{ID: "m0-a", Q: "q", A: "a", Module: "M0"},
		{ID: "m0-b", Q: "q", A: "a", Module: "M0"},
		{ID: "m0-cp-one", Q: "q", A: "a", Module: "M0", Checkpoint: "M0", Requires: requires},
		{ID: "m0-cp-two", Q: "q", A: "a", Module: "M0", Checkpoint: "M0", Requires: requires},
	}
}

func exam() []deck.Card { return checkpointDeck()[2:] }

// masterModule brings the ordinary M0 cards to Review state, which is the
// precondition for the checkpoint being offered at all.
func masterModule(t *testing.T, s *review.Store, now time.Time) {
	t.Helper()

	for _, c := range checkpointDeck() {
		if c.Checkpoint == "" {
			master(t, s, c.ID, now)
		}
	}
}

// sit drives a whole checkpoint session through the exported API, grading every
// card the same way.
func sit(t *testing.T, s *review.Store, g review.Grade, now time.Time) {
	t.Helper()

	if err := s.StartCheckpoint("M0", exam(), now); err != nil {
		t.Fatalf("starting the checkpoint: %v", err)
	}

	for range 10 {
		c, ok := s.NextCheckpoint("M0", exam())
		if !ok {
			return
		}

		if err := s.GradeCheckpoint("M0", c.ID, g, exam(), now); err != nil {
			t.Fatalf("grading %q: %v", c.ID, err)
		}
	}

	t.Fatal("the checkpoint session never ran out of cards")
}

// A checkpoint is only offered once its module is retained, so a failure means a
// transfer gap rather than an exposure gap.
func TestCheckpointIsLockedUntilTheModuleIsMastered(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	got := s.CheckpointStatus("M0", exam(), now)
	if got.State != review.CheckpointLocked {
		t.Errorf("State = %v, want locked on a fresh store", got.State)
	}

	if got.Unmastered != 2 {
		t.Errorf("Unmastered = %d, want 2 (both M0 cards)", got.Unmastered)
	}

	masterModule(t, s, now)

	if got := s.CheckpointStatus("M0", exam(), now); got.State != review.CheckpointReady {
		t.Errorf("State = %v, want ready once the module is mastered", got.State)
	}
}

// An unknown module must not read as a deliberate status.
func TestCheckpointStatusForAModuleWithNoExam(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)

	if got := s.CheckpointStatus("M9", nil, time.Now()); got.State != review.CheckpointUnknown {
		t.Errorf("State = %v, want unknown for a module with no checkpoint cards", got.State)
	}
}

func TestCheckpointCleanSweepPasses(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	masterModule(t, s, now)
	sit(t, s, review.Good, now)

	got := s.CheckpointStatus("M0", exam(), now)
	if got.State != review.CheckpointPassed {
		t.Fatalf("State = %v, want passed after grading every card Good", got.State)
	}

	if got.Day != now.Format("2006-01-02") {
		t.Errorf("Day = %q, want today's date", got.Day)
	}
}

// Clean sweep means clean: one shaky answer is a failed attempt, not a partial
// pass.
func TestCheckpointFailsOnAnyBadGrade(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		grade review.Grade
	}{
		{"again", review.Again},
		{"hard", review.Hard},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, _ := openStore(t, 20)
			now := time.Now()

			masterModule(t, s, now)

			if err := s.StartCheckpoint("M0", exam(), now); err != nil {
				t.Fatal(err)
			}

			// Fail the first card, then answer the rest cleanly: the attempt is
			// already lost, but the remaining cards are still shown.
			first, ok := s.NextCheckpoint("M0", exam())
			if !ok {
				t.Fatal("expected a checkpoint card")
			}

			if err := s.GradeCheckpoint("M0", first.ID, tt.grade, exam(), now); err != nil {
				t.Fatal(err)
			}

			second, ok := s.NextCheckpoint("M0", exam())
			if !ok {
				t.Fatal("a failed attempt should still show the remaining cards")
			}

			if second.ID == first.ID {
				t.Fatalf("NextCheckpoint repeated %q", first.ID)
			}

			if err := s.GradeCheckpoint("M0", second.ID, review.Easy, exam(), now); err != nil {
				t.Fatal(err)
			}

			if got := s.CheckpointStatus("M0", exam(), now); got.State != review.CheckpointFailed {
				t.Errorf("State = %v, want failed after a %s", got.State, tt.name)
			}
		})
	}
}

// Same-session retakes would test short-term memory of the answer just read.
func TestFailedCheckpointRetakesTheNextDay(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	masterModule(t, s, now)
	sit(t, s, review.Again, now)

	got := s.CheckpointStatus("M0", exam(), now)
	if got.State != review.CheckpointFailed {
		t.Fatalf("State = %v, want failed", got.State)
	}

	tomorrow := now.AddDate(0, 0, 1)
	if got.RetryOn != tomorrow.Format("2006-01-02") {
		t.Errorf("RetryOn = %q, want %q", got.RetryOn, tomorrow.Format("2006-01-02"))
	}

	if got := s.CheckpointStatus("M0", exam(), tomorrow); got.State != review.CheckpointReady {
		t.Errorf("State = %v the next day, want ready", got.State)
	}

	// And the retake can actually be passed.
	sit(t, s, review.Good, tomorrow)

	if got := s.CheckpointStatus("M0", exam(), tomorrow); got.State != review.CheckpointPassed {
		t.Errorf("State = %v, want passed on the retake", got.State)
	}
}

// An unpassed checkpoint is not part of the daily review economy: it surfaces
// only through its status line, never in the drill counts.
func TestCheckpointCardsAreOutsideTheDrillUntilPassed(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	all := checkpointDeck()

	st := s.Stats(all, now)
	if st.Total != 2 || st.New != 2 || st.Locked != 0 {
		t.Errorf("Stats = %+v; the exam should not be counted at all before a pass", st)
	}

	masterModule(t, s, now)

	// Nothing ordinary is due yet, and the exam must not fill the gap.
	if got, ok := s.Next(all, now.Add(time.Hour)); ok && got.Checkpoint != "" {
		t.Errorf("Next returned checkpoint card %q before the checkpoint was passed", got.ID)
	}

	sit(t, s, review.Good, now)

	if st := s.Stats(all, now); st.Total != 4 {
		t.Errorf("Stats.Total = %d after a pass, want 4: the exam joins the deck", st.Total)
	}
}

func TestPassedCheckpointCardsEnterFSRSRotation(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	masterModule(t, s, now)
	sit(t, s, review.Good, now)

	// The pass grades are the cards' first reviews, so they are scheduled, not
	// unseen.
	st := s.Stats(exam(), now)
	if st.New != 0 {
		t.Errorf("New = %d after a pass, want 0: the pass grades are the first reviews", st.New)
	}

	if st.NextDue.IsZero() {
		t.Error("passed checkpoint cards should be scheduled for a future review")
	}

	// And they come back around like any other card.
	if _, ok := s.Next(exam(), now.AddDate(0, 0, 365)); !ok {
		t.Error("a passed checkpoint card should come due eventually")
	}
}

// A checkpoint is an event, not a review day.
func TestCheckpointSessionsDoNotCountAsReviews(t *testing.T) {
	t.Parallel()

	s, _ := openStore(t, 20)
	now := time.Now()

	masterModule(t, s, now)

	before := s.Stats(checkpointDeck(), now).ReviewsToday

	sit(t, s, review.Good, now)

	if got := s.Stats(checkpointDeck(), now).ReviewsToday; got != before {
		t.Errorf("ReviewsToday = %d after a checkpoint, want %d unchanged", got, before)
	}
}

// The fixtures above are the state machine; this one is the real M0 exam. It
// drives a full pass and a full failure over the shipped deck through the same
// exported API the route uses, against a store pre-mastered by grading rather
// than by waiting out FSRS intervals.
func TestRealM0CheckpointPassesAndFails(t *testing.T) {
	t.Parallel()

	lib, err := deck.Load(flashcards.Decks, "decks")
	if err != nil {
		t.Fatal(err)
	}

	cards := lib.Checkpoints("M0")
	if len(cards) == 0 {
		t.Fatal("M0 has no checkpoint cards")
	}

	sitM0 := func(t *testing.T, g review.Grade) review.CheckpointStatus {
		t.Helper()

		s, _ := openStore(t, 500)
		now := time.Now()

		// The synthesized module edges are exactly the cards that must be
		// retained first.
		for _, c := range cards {
			for _, id := range c.Requires {
				master(t, s, id, now)
			}
		}

		if got := s.CheckpointStatus("M0", cards, now); got.State != review.CheckpointReady {
			t.Fatalf("State = %v with M0 mastered, want ready (%d unmastered)", got.State, got.Unmastered)
		}

		if err := s.StartCheckpoint("M0", cards, now); err != nil {
			t.Fatal(err)
		}

		for range cards {
			c, ok := s.NextCheckpoint("M0", cards)
			if !ok {
				break
			}

			if err := s.GradeCheckpoint("M0", c.ID, g, cards, now); err != nil {
				t.Fatal(err)
			}
		}

		return s.CheckpointStatus("M0", cards, now)
	}

	t.Run("clean sweep", func(t *testing.T) {
		t.Parallel()

		if got := sitM0(t, review.Easy); got.State != review.CheckpointPassed {
			t.Errorf("State = %v, want passed", got.State)
		}
	})

	t.Run("every answer failed", func(t *testing.T) {
		t.Parallel()

		got := sitM0(t, review.Again)
		if got.State != review.CheckpointFailed {
			t.Errorf("State = %v, want failed", got.State)
		}

		if got.RetryOn == "" {
			t.Error("a failed attempt should name its retry date")
		}
	})
}

// The migration must add the attempts map and nothing else: a lost review
// history is the one unrecoverable failure this store has.
func TestOpenMigratesV1State(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile(filepath.Join("testdata", "state_v1.json"))
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "review.json")
	//nolint:gosec // G703: path is under t.TempDir(), not attacker-controlled.
	if err := os.WriteFile(path, fixture, 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := review.Open(path, 20)
	if err != nil {
		t.Fatalf("a v1 state file must still load: %v", err)
	}

	for _, id := range []string{"term-cluster", "term-pod", "m0-control-plane-components"} {
		if !s.Mastered(id) {
			t.Errorf("card %q lost its review history across the migration", id)
		}
	}

	// The captured fixture recorded four reviews across four days.
	if got := s.Streak(time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)); got == 0 {
		t.Error("the day counters did not survive the migration")
	}

	// A migrated store is a working v2 store.
	if got := s.CheckpointStatus("M0", exam(), time.Now()); got.State != review.CheckpointLocked {
		t.Errorf("State = %v on a migrated store, want locked", got.State)
	}

	if err := s.Writable(); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // G304: path is under t.TempDir(), not attacker-controlled.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(after), `"version": 2`) {
		t.Errorf("the rewritten state is not v2:\n%s", after)
	}
}
