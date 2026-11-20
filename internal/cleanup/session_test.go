package cleanup_test

import (
	"ard/internal/cleanup"
	"ard/internal/session"
	"errors"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"
)

func newCleaner(sessRepo *mockSessionsRepo) *cleanup.SessionsCleaner {
	return cleanup.NewSessionsCleaner(sessRepo, slog.New(slog.DiscardHandler))
}

func TestClean_DeletesDeprecatedSessions(t *testing.T) {
	sessRepo := &mockSessionsRepo{
		headers: []session.Header{
			oldHeader("sess-old", time.Now().Add(-24*time.Hour)),
			oldHeader("sess-new", time.Now()),
		},
	}
	cleaner := newCleaner(sessRepo)

	if err := cleaner.Clean(testRetention); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []session.ID{"sess-old"}
	if !reflect.DeepEqual(sessRepo.deleted, want) {
		t.Fatalf("deleted sessions %v, want %v", sessRepo.deleted, want)
	}
}

func TestClean_KeepsRecentSessions(t *testing.T) {
	sessRepo := &mockSessionsRepo{
		headers: []session.Header{
			oldHeader("s1", time.Now()),
			oldHeader("s2", time.Now().Add(-time.Hour)),
		},
	}
	cleaner := newCleaner(sessRepo)

	if err := cleaner.Clean(testRetention); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sessRepo.deleted) != 0 {
		t.Fatalf("expected no deletions, got %v", sessRepo.deleted)
	}
}

func TestClean_DeletesBrokenSessions(t *testing.T) {
	sessRepo := &mockSessionsRepo{
		headersErr: session.NewBrokenHeadersError(
			session.NewBrokenHeaderError("broken-1", errors.New("cause")),
			session.NewBrokenHeaderError("broken-2", errors.New("cause")),
		),
	}
	cleaner := newCleaner(sessRepo)

	if err := cleaner.Clean(testRetention); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []session.ID{"broken-1", "broken-2"}
	if !reflect.DeepEqual(sessRepo.deleted, want) {
		t.Fatalf("deleted sessions %v, want %v", sessRepo.deleted, want)
	}
}

func TestClean_DeletesBrokenAndDeprecatedSessions(t *testing.T) {
	sessRepo := &mockSessionsRepo{
		headers: []session.Header{
			oldHeader("sess-old", time.Now().Add(-24*time.Hour)),
		},
		headersErr: session.NewBrokenHeadersError(
			session.NewBrokenHeaderError("broken-1", errors.New("cause")),
		),
	}
	cleaner := newCleaner(sessRepo)

	if err := cleaner.Clean(testRetention); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []session.ID{"broken-1", "sess-old"}
	if !reflect.DeepEqual(sessRepo.deleted, want) {
		t.Fatalf("deleted sessions %v, want %v", sessRepo.deleted, want)
	}
}

func TestClean_IgnoresHeadersError(t *testing.T) {
	sessRepo := &mockSessionsRepo{
		headersErr: errors.New("read failure"),
	}
	cleaner := newCleaner(sessRepo)

	if err := cleaner.Clean(testRetention); err != nil {
		t.Fatalf("headers error must not fail cleanup: %v", err)
	}

	if len(sessRepo.deleted) != 0 {
		t.Fatalf("expected no deletions, got %v", sessRepo.deleted)
	}
}

func TestClean_ContinuesOnDeleteError(t *testing.T) {
	old := time.Now().Add(-24 * time.Hour)
	sessRepo := &mockSessionsRepo{
		headers: []session.Header{
			oldHeader("s1", old),
			oldHeader("s2", old),
		},
		deleteErr: errors.New("disk full"),
	}
	cleaner := newCleaner(sessRepo)

	if err := cleaner.Clean(testRetention); err != nil {
		t.Fatalf("delete error must not abort cleanup: %v", err)
	}

	if got := sessRepo.deleteAttemptCount(); got != 2 {
		t.Fatalf("expected both sessions attempted for deletion, got %d", got)
	}
	if len(sessRepo.deleted) != 0 {
		t.Fatalf("expected no successful deletions, got %v", sessRepo.deleted)
	}
}

func TestClean_IsConcurrencySafe(t *testing.T) {
	old := time.Now().Add(-24 * time.Hour)
	sessRepo := &mockSessionsRepo{
		headers: []session.Header{
			oldHeader("s1", old),
			oldHeader("s2", old),
		},
	}
	cleaner := newCleaner(sessRepo)

	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if err := cleaner.Clean(testRetention); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()

	if got := sessRepo.deleteAttemptCount(); got != 4 {
		t.Fatalf("expected 4 delete attempts (2 passes x 2 sessions), got %d", got)
	}
}
