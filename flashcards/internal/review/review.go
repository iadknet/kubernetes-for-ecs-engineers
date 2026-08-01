// Package review schedules cards with FSRS and persists review history.
//
// The store is a single JSON file written atomically. That is a deliberate
// choice for this training program: deploy it on Kubernetes with no volume and
// your review history vanishes when the Pod reschedules, which is the
// "containers are ephemeral" lesson delivered firsthand and the motivation for
// the M3 PersistentVolumeClaim exercise.
package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"

	"github.com/iadk/k8s-flashcards/internal/deck"
)

// Grade is a self-assessed answer quality, matching FSRS ratings.
type Grade = fsrs.Rating

// The four FSRS ratings a reviewer can give an answer.
const (
	Again = fsrs.Again // forgot it
	Hard  = fsrs.Hard  // recalled with real difficulty
	Good  = fsrs.Good  // recalled
	Easy  = fsrs.Easy  // trivial
)

// stateVersion 2 added the per-module checkpoint attempts map. Older files are
// migrated in Open; nothing in a v1 file is rewritten.
const stateVersion = 2

type persisted struct {
	Version     int                          `json:"version"`
	Cards       map[string]fsrs.Card         `json:"cards"`
	Reviews     map[string]int               `json:"reviewsPerDay"`
	NewSeen     map[string]int               `json:"newPerDay"`
	Checkpoints map[string]checkpointAttempt `json:"checkpoints"`
}

// checkpointAttempt is one module's most recent checkpoint sitting.
//
// Grades are buffered rather than applied as they are given: a failed attempt
// must leave no FSRS trace, or a checkpoint card would enter the drill queue
// through the back door of a failure. On a pass they are replayed as the cards'
// first reviews.
type checkpointAttempt struct {
	Day    string           `json:"day"`
	Grades map[string]Grade `json:"grades"`
	Failed bool             `json:"failed"`
	Done   bool             `json:"done"`
}

func (a checkpointAttempt) passed() bool { return a.Done && !a.Failed }

// Store holds scheduling state for every card that has been reviewed at least
// once. Cards absent from the store are "new".
type Store struct {
	mu        sync.Mutex
	path      string
	fsrs      *fsrs.FSRS
	st        persisted
	newPerDay int
}

// Open loads the store at path, creating it if absent. newPerDay caps how many
// previously unseen cards are introduced per day.
func Open(path string, newPerDay int) (*Store, error) {
	s := &Store{
		path:      path,
		fsrs:      fsrs.NewFSRS(fsrs.DefaultParam()),
		newPerDay: newPerDay,
		st: persisted{
			Version:     stateVersion,
			Cards:       map[string]fsrs.Card{},
			Reviews:     map[string]int{},
			NewSeen:     map[string]int{},
			Checkpoints: map[string]checkpointAttempt{},
		},
	}

	//nolint:gosec // G304: path is the operator-supplied DATA_DIR, not user input.
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil // fresh start
	case err != nil:
		return nil, fmt.Errorf("reading review state: %w", err)
	}

	if err := json.Unmarshal(data, &s.st); err != nil {
		return nil, fmt.Errorf("parsing review state at %s: %w", path, err)
	}

	if s.st.Cards == nil {
		s.st.Cards = map[string]fsrs.Card{}
	}

	if s.st.Reviews == nil {
		s.st.Reviews = map[string]int{}
	}

	if s.st.NewSeen == nil {
		s.st.NewSeen = map[string]int{}
	}

	s.migrate()

	return s, nil
}

// migrate brings an older state file up to stateVersion. It only ever adds
// fields: card entries and day counters are never rewritten, because a lost
// review history is the one failure of this store that cannot be undone.
func (s *Store) migrate() {
	from := s.st.Version
	if from >= stateVersion {
		s.st.Checkpoints = orEmpty(s.st.Checkpoints)
		return
	}

	s.st.Checkpoints = orEmpty(s.st.Checkpoints)
	s.st.Version = stateVersion

	slog.Info("migrated review state",
		"path", s.path, "from", from, "to", stateVersion, "cards", len(s.st.Cards))
}

func orEmpty(m map[string]checkpointAttempt) map[string]checkpointAttempt {
	if m == nil {
		return map[string]checkpointAttempt{}
	}

	return m
}

func day(t time.Time) string { return t.Format("2006-01-02") }

// Day renders t as the store's day key. It is exported so callers that need to
// change something once a day — the multiple-choice option seed — re-roll on
// exactly the same boundary the review counters do, rather than on a second
// definition of "today" that could drift from this one.
func Day(t time.Time) string { return day(t) }

// Grade records an answer and returns when the card is next due.
func (s *Store) Grade(id string, g Grade, now time.Time) (time.Time, error) {
	if g < fsrs.Again || g > fsrs.Easy {
		return time.Time{}, fmt.Errorf("review: invalid grade %d", g)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, seen := s.st.Cards[id]; !seen {
		s.st.NewSeen[day(now)]++
	}

	s.st.Reviews[day(now)]++

	next := s.schedule(id, g, now)
	if err := s.save(); err != nil {
		return time.Time{}, err
	}

	return next.Due, nil
}

// schedule advances a card's FSRS state without touching the day counters.
// Checkpoint passes replay their grades through here: a checkpoint is an event,
// not a review day, so it must not move the streak or ReviewsToday.
func (s *Store) schedule(id string, g Grade, now time.Time) fsrs.Card {
	card, seen := s.st.Cards[id]
	if !seen {
		card = fsrs.NewCard()
	}

	next := s.fsrs.Next(card, now, g).Card
	s.st.Cards[id] = next

	return next
}

// save writes the state atomically: temp file in the same directory, then
// rename, so a crash mid-write can't truncate the history.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding review state: %w", err)
	}

	dir := filepath.Dir(s.path)

	tmp, err := os.CreateTemp(dir, ".review-*.json")
	if err != nil {
		return fmt.Errorf("creating temp state file in %s: %w", dir, err)
	}
	// Cleanup for the failure paths below. After a successful Rename the temp
	// file no longer exists, so this Remove fails harmlessly; the error is
	// discarded deliberately rather than masking the real return value.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() // already failing; the write error is the one worth reporting
		return fmt.Errorf("writing review state: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing review state: %w", err)
	}

	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("replacing review state: %w", err)
	}

	return nil
}

// Writable reports whether the store can persist. Used by the readiness probe:
// an instance that cannot record reviews should not receive traffic.
func (s *Store) Writable() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.save()
}

// Next picks the card to show: the most overdue card first, otherwise a new
// card if today's introduction budget allows. Returns false when nothing is
// due.
func (s *Store) Next(cards []deck.Card, now time.Time) (deck.Card, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		due   []deck.Card
		fresh []deck.Card
	)

	for _, c := range cards {
		if s.withheld(c) {
			continue
		}

		st, seen := s.st.Cards[c.ID]
		switch {
		case !seen:
			// Gate introduction only. A card already in the schedule keeps its
			// reviews: withdrawing something you have started learning because a
			// prerequisite lapsed would strand it.
			if s.locked(c) {
				continue
			}

			fresh = append(fresh, c)
		case !st.Due.After(now):
			due = append(due, c)
		}
	}

	if len(due) > 0 {
		sort.Slice(due, func(i, j int) bool {
			return s.st.Cards[due[i].ID].Due.Before(s.st.Cards[due[j].ID].Due)
		})

		return due[0], true
	}

	if len(fresh) > 0 && s.st.NewSeen[day(now)] < s.newPerDay {
		return fresh[0], true
	}

	return deck.Card{}, false
}

// Mastered reports whether a card has reached FSRS Review state — retained,
// not merely seen. It is the mastery signal prerequisite gating reads, and the
// analogue of a WaniKani item reaching "Guru".
func (s *Store) Mastered(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.mastered(id)
}

// mastered is Mastered without the lock, for callers already holding it.
func (s *Store) mastered(id string) bool {
	st, seen := s.st.Cards[id]
	return seen && st.State == fsrs.Review
}

// locked reports whether a card is being withheld because a prerequisite is not
// yet retained. Satisfaction is read from the store by card id, so it is
// evaluated over the whole library: no filter can make an unmastered
// prerequisite look satisfied.
func (s *Store) locked(c deck.Card) bool {
	for _, id := range c.Requires {
		if !s.mastered(id) {
			return true
		}
	}

	return false
}

// Cram ignores scheduling entirely and returns the least recently seen card —
// for the night before an interview, when spaced repetition is not the point.
func (s *Store) Cram(cards []deck.Card, exclude string) (deck.Card, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	best := -1

	var bestSeen time.Time

	for i, c := range cards {
		if c.ID == exclude && len(cards) > 1 {
			continue
		}

		st, seen := s.st.Cards[c.ID]
		switch {
		case !seen:
			return c, true // never studied wins outright
		case best == -1 || st.LastReview.Before(bestSeen):
			best, bestSeen = i, st.LastReview
		}
	}

	if best == -1 {
		return deck.Card{}, false
	}

	return cards[best], true
}

// Stats summarizes progress over a set of cards.
type Stats struct {
	Total    int
	New      int
	Due      int
	Learning int
	Known    int
	// Locked counts unseen cards held back by an unmastered prerequisite. They
	// are also counted in New: locked is a reason, not a separate bucket.
	Locked       int
	ReviewsToday int
	NextDue      time.Time
}

// Stats computes counts for the given cards.
func (s *Store) Stats(cards []deck.Card, now time.Time) Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := Stats{ReviewsToday: s.st.Reviews[day(now)]}

	for _, c := range cards {
		if s.withheld(c) {
			continue
		}

		out.Total++

		st, seen := s.st.Cards[c.ID]
		if !seen {
			out.New++

			if s.locked(c) {
				out.Locked++
			}

			continue
		}

		if !st.Due.After(now) {
			out.Due++
		} else if out.NextDue.IsZero() || st.Due.Before(out.NextDue) {
			out.NextDue = st.Due
		}

		if st.State == fsrs.Review {
			out.Known++
		} else {
			out.Learning++
		}
	}

	return out
}

// --- Checkpoints -------------------------------------------------------------

// ErrCheckpointUnavailable is returned when a checkpoint session is started or
// graded while the module's checkpoint is not being offered — locked behind an
// unmastered module, already passed, or inside its post-failure cool-down.
//
// Callers are expected to read CheckpointStatus and never provoke this; it is
// the backstop that keeps a hand-crafted request from erasing a recorded pass.
var ErrCheckpointUnavailable = errors.New("review: checkpoint is not available")

// ErrCardNotInCheckpoint is returned when a grade names a card the module's
// exam does not contain.
//
// An attempt completes when it holds as many grades as the exam has cards, so
// an unchecked id is not a harmless typo: enough of them satisfy the count and
// record a pass having answered nothing.
var ErrCardNotInCheckpoint = errors.New("review: card is not in the checkpoint")

// CheckpointState is where one module's checkpoint stands.
type CheckpointState int

// The checkpoint states, with a sentinel at zero: an uninitialized status must
// not read as a deliberate one.
const (
	CheckpointUnknown CheckpointState = iota
	CheckpointLocked
	CheckpointReady
	CheckpointFailed
	CheckpointPassed
)

func (c CheckpointState) String() string {
	switch c {
	case CheckpointLocked:
		return "locked"
	case CheckpointReady:
		return "ready"
	case CheckpointFailed:
		return "failed"
	case CheckpointPassed:
		return "passed"
	case CheckpointUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// CheckpointStatus is everything the UI needs to render one module's checkpoint
// line, and everything a route needs to decide whether to offer a session.
type CheckpointStatus struct {
	Module string
	State  CheckpointState
	// Unmastered counts the prerequisite cards still to be retained. Only
	// meaningful while Locked.
	Unmastered int
	// Day is the recorded attempt's date, and RetryOn the earliest day a failed
	// attempt may be retaken. Both are YYYY-MM-DD, the same form the day
	// counters use, so no timezone slips in between recording and comparing.
	Day     string
	RetryOn string
}

// Locked reports whether the checkpoint is still gated on retention, for
// templates that would otherwise compare against a string.
func (c CheckpointStatus) Locked() bool { return c.State == CheckpointLocked }

// Available reports whether a session can be sat right now.
func (c CheckpointStatus) Available() bool { return c.State == CheckpointReady }

// CheckpointStatus reports where module's checkpoint stands. cards is that
// module's checkpoint cards; an empty set means the module has no exam yet.
func (s *Store) CheckpointStatus(module string, cards []deck.Card, now time.Time) CheckpointStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.checkpointStatus(module, cards, now)
}

func (s *Store) checkpointStatus(module string, cards []deck.Card, now time.Time) CheckpointStatus {
	out := CheckpointStatus{Module: module}
	if len(cards) == 0 {
		return out
	}

	attempt := s.st.Checkpoints[module]
	if attempt.passed() {
		out.State, out.Day = CheckpointPassed, attempt.Day
		return out
	}

	// Prerequisite mastery is checked before the cool-down, so a checkpoint whose
	// module has lapsed reads as locked rather than as retakeable.
	seen := map[string]bool{}

	for _, c := range cards {
		for _, id := range c.Requires {
			if seen[id] || s.mastered(id) {
				continue
			}

			seen[id] = true
			out.Unmastered++
		}
	}

	if out.Unmastered > 0 {
		out.State = CheckpointLocked
		return out
	}

	// A failed attempt holds the checkpoint shut for the rest of the day. Same-day
	// retakes would test short-term memory of the answer just read.
	if attempt.Done && attempt.Failed && attempt.Day == day(now) {
		out.State = CheckpointFailed
		out.Day = attempt.Day
		out.RetryOn = day(now.AddDate(0, 0, 1))

		return out
	}

	out.State = CheckpointReady

	return out
}

// StartCheckpoint begins an attempt over the module's checkpoint cards, or
// resumes one already in progress today.
func (s *Store) StartCheckpoint(module string, cards []deck.Card, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.checkpointStatus(module, cards, now).State != CheckpointReady {
		return fmt.Errorf("starting checkpoint %s: %w", module, ErrCheckpointUnavailable)
	}

	if a, ok := s.st.Checkpoints[module]; ok && !a.Done && a.Day == day(now) {
		return nil // resume; re-entering the page must not discard the answers so far
	}

	s.st.Checkpoints[module] = checkpointAttempt{Day: day(now), Grades: map[string]Grade{}}

	return s.save()
}

// NextCheckpoint returns the next card of the attempt in progress. A failed
// attempt keeps handing out cards: the sitting is a full diagnostic, so the
// remaining questions are still worth seeing.
func (s *Store) NextCheckpoint(module string, cards []deck.Card) (deck.Card, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	attempt, ok := s.st.Checkpoints[module]
	if !ok || attempt.Done {
		return deck.Card{}, false
	}

	for _, c := range cards {
		if _, graded := attempt.Grades[c.ID]; !graded {
			return c, true
		}
	}

	return deck.Card{}, false
}

// GradeCheckpoint records one answer in the attempt in progress. Any Again or
// Hard fails the attempt immediately; grading the last card completes it, and a
// clean sweep replays the buffered grades as the checkpoint cards' first FSRS
// reviews.
func (s *Store) GradeCheckpoint(module, id string, g Grade, cards []deck.Card, now time.Time) error {
	if g < fsrs.Again || g > fsrs.Easy {
		return fmt.Errorf("review: invalid grade %d", g)
	}

	if !slices.ContainsFunc(cards, func(c deck.Card) bool { return c.ID == id }) {
		return fmt.Errorf("grading checkpoint %s with %q: %w", module, id, ErrCardNotInCheckpoint)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	attempt, ok := s.st.Checkpoints[module]
	if !ok || attempt.Done {
		return fmt.Errorf("grading checkpoint %s: %w", module, ErrCheckpointUnavailable)
	}

	attempt.Grades[id] = g
	if g < Good {
		attempt.Failed = true
	}

	if len(attempt.Grades) >= len(cards) {
		attempt.Done = true
	}

	s.st.Checkpoints[module] = attempt

	// Only a pass puts the cards into rotation. A failed attempt must leave no
	// FSRS trace, or the exam would enter the drill queue through the back door
	// of a failure.
	if attempt.passed() {
		for _, c := range cards {
			if buffered, graded := attempt.Grades[c.ID]; graded {
				s.schedule(c.ID, buffered, now)
			}
		}
	}

	return s.save()
}

// withheld reports whether a card is outside the daily review economy: an
// unpassed checkpoint card. It is excluded from every Stats bucket, Total
// included, so the buckets still sum. Cram is deliberately unaffected.
func (s *Store) withheld(c deck.Card) bool {
	return c.Checkpoint != "" && !s.st.Checkpoints[c.Checkpoint].passed()
}

// Streak counts consecutive days with at least one review, ending today or
// yesterday.
func (s *Store) Streak(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	d := now
	if s.st.Reviews[day(d)] == 0 {
		d = d.AddDate(0, 0, -1) // today not studied yet; don't break the streak
	}

	n := 0
	for s.st.Reviews[day(d)] > 0 {
		n++
		d = d.AddDate(0, 0, -1)
	}

	return n
}
