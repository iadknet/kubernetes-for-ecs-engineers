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
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"

	"github.com/iadk/k8s-flashcards/internal/deck"
)

// Grade is a self-assessed answer quality, matching FSRS ratings.
type Grade = fsrs.Rating

const (
	Again = fsrs.Again // forgot it
	Hard  = fsrs.Hard  // recalled with real difficulty
	Good  = fsrs.Good  // recalled
	Easy  = fsrs.Easy  // trivial
)

const stateVersion = 1

type persisted struct {
	Version int                  `json:"version"`
	Cards   map[string]fsrs.Card `json:"cards"`
	Reviews map[string]int       `json:"reviewsPerDay"`
	NewSeen map[string]int       `json:"newPerDay"`
}

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
			Version: stateVersion,
			Cards:   map[string]fsrs.Card{},
			Reviews: map[string]int{},
			NewSeen: map[string]int{},
		},
	}

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
	return s, nil
}

func day(t time.Time) string { return t.Format("2006-01-02") }

// Grade records an answer and returns when the card is next due.
func (s *Store) Grade(id string, g Grade, now time.Time) (time.Time, error) {
	if g < fsrs.Again || g > fsrs.Easy {
		return time.Time{}, fmt.Errorf("invalid grade %d", g)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	card, seen := s.st.Cards[id]
	if !seen {
		card = fsrs.NewCard()
		s.st.NewSeen[day(now)]++
	}

	next := s.fsrs.Next(card, now, g).Card
	s.st.Cards[id] = next
	s.st.Reviews[day(now)]++

	if err := s.save(); err != nil {
		return time.Time{}, err
	}
	return next.Due, nil
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
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
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

	var due []deck.Card
	var fresh []deck.Card
	for _, c := range cards {
		st, seen := s.st.Cards[c.ID]
		switch {
		case !seen:
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
	Total        int
	New          int
	Due          int
	Learning     int
	Known        int
	ReviewsToday int
	NextDue      time.Time
}

// Stats computes counts for the given cards.
func (s *Store) Stats(cards []deck.Card, now time.Time) Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := Stats{Total: len(cards), ReviewsToday: s.st.Reviews[day(now)]}
	for _, c := range cards {
		st, seen := s.st.Cards[c.ID]
		if !seen {
			out.New++
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
