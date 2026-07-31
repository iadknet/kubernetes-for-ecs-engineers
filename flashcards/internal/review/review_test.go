package review_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	s, _ := openStore(t, 20)
	for _, g := range []review.Grade{0, 5, -1} {
		if _, err := s.Grade("x", g, time.Now()); err == nil {
			t.Errorf("grade %d should have been rejected", g)
		}
	}
}

func TestCramIgnoresSchedulingAndAvoidsRepeats(t *testing.T) {
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
	s, _ := openStore(t, 50)
	now := time.Now()

	for i := 0; i < 3; i++ {
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

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	if err := s.Writable(); err == nil {
		t.Error("expected Writable to fail on a read-only directory")
	}
	if _, err := s.Grade("a", review.Good, time.Now()); err == nil {
		t.Error("expected Grade to fail when state cannot be persisted")
	}
}

func TestOpenRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := review.Open(path, 20); err == nil {
		t.Error("expected an error for corrupt review state, not a silent reset")
	}
}
