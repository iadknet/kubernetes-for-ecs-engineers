// Command flashcards serves the Kubernetes training flashcards over HTTP.
//
// Configuration is environment-driven so the same binary runs on a laptop and
// in a Pod without a rebuild:
//
//	PORT         listen port (default 8080)
//	DATA_DIR     where review state is persisted (default ./data)
//	DECKS_DIR    read decks from disk instead of the embedded copy
//	NEW_PER_DAY  cap on newly introduced cards per day (default 20)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	flashcards "github.com/iadk/k8s-flashcards"
	"github.com/iadk/k8s-flashcards/internal/deck"
	"github.com/iadk/k8s-flashcards/internal/review"
	"github.com/iadk/k8s-flashcards/internal/web"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	var (
		port      = env("PORT", "8080")
		dataDir   = env("DATA_DIR", "./data")
		decksDir  = os.Getenv("DECKS_DIR")
		newPerDay = envInt("NEW_PER_DAY", 20)
	)

	// Decks come from the embedded copy unless DECKS_DIR points at a directory
	// — that override is what lets M2 mount them from a ConfigMap.
	var (
		lib *deck.Library
		err error
	)
	if decksDir != "" {
		lib, err = deck.LoadDir(decksDir)
		log.Info("loading decks from disk", "dir", decksDir)
	} else {
		lib, err = deck.Load(flashcards.Decks, "decks")
	}
	if err != nil {
		return fmt.Errorf("loading decks: %w", err)
	}
	log.Info("decks loaded", "decks", len(lib.Decks), "cards", len(lib.Cards))

	// 0o750, not 0o755: only the running user (65532 in the image) and its group
	// need this directory. gosec G301 flags anything looser.
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}
	store, err := review.Open(filepath.Join(dataDir, "review.json"), newPerDay)
	if err != nil {
		return fmt.Errorf("opening review store: %w", err)
	}

	srv, err := web.New(lib, store)
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:              ":" + port,
		Handler:           logging(log, srv.Routes()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown: SIGTERM is what Kubernetes sends first, and draining
	// cleanly is what makes rolling updates non-disruptive.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", "http://localhost:"+port, "data", dataDir)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received, draining")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	log.Info("stopped cleanly")
	return nil
}

func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		// Health probes fire constantly; logging them buries everything else.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			return
		}
		log.Info("request",
			"method", r.Method, "path", r.URL.Path,
			"status", rec.status, "dur", time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
