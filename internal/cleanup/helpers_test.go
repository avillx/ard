package cleanup_test

import (
	"ard/internal/session"
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	testRetention   = 10 * time.Hour
	testInterval    = 5 * time.Hour
	testMaxLogLines = 5
)

var errNotImplemented = errors.New("not implemented in test")

type mockSessionsRepo struct {
	headers    []session.Header
	headersErr error
	deleteErr  error

	mu                sync.Mutex
	deleteAttempts    []session.ID
	deleted           []session.ID
	headersCallsCount int
}

func (m *mockSessionsRepo) Session(session.ID) (session.Session, error) {
	return nil, errNotImplemented
}

func (m *mockSessionsRepo) Save(session.Session) error {
	return errNotImplemented
}

func (m *mockSessionsRepo) Delete(sessID session.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.deleteAttempts = append(m.deleteAttempts, sessID)
	if m.deleteErr != nil {
		return m.deleteErr
	}

	m.deleted = append(m.deleted, sessID)
	return nil
}

func (m *mockSessionsRepo) Headers() ([]session.Header, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.headersCallsCount++

	return m.headers, m.headersErr
}

func (m *mockSessionsRepo) deleteAttemptCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.deleteAttempts)
}

func (m *mockSessionsRepo) headersCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.headersCallsCount
}

func oldHeader(id session.ID, updatedAt time.Time) session.Header {
	return session.NewHeader(id, 0, 0, updatedAt, updatedAt, "test-profile", nil)
}

type mockLogTrimmer struct {
	mu           sync.Mutex
	calls        int
	lastMaxLines int
	trimErr      error
	trimmed      chan struct{}
}

func (m *mockLogTrimmer) Trim(maxLines int) error {
	m.mu.Lock()
	m.calls++
	m.lastMaxLines = maxLines
	m.mu.Unlock()

	if m.trimmed != nil {
		select {
		case m.trimmed <- struct{}{}:
		default:
		}
	}

	return m.trimErr
}

func (m *mockLogTrimmer) trimmedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.calls
}

func (m *mockLogTrimmer) maxLines() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.lastMaxLines
}

type stubHandler struct {
	mu      sync.Mutex
	records map[slog.Level]int
}

func newStubLogger() (*slog.Logger, *stubHandler) {
	h := &stubHandler{records: map[slog.Level]int{}}
	return slog.New(h), h
}

func (h *stubHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *stubHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.records[r.Level]++
	return nil
}

func (h *stubHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *stubHandler) WithGroup(_ string) slog.Handler {
	return h
}

func (h *stubHandler) count(level slog.Level) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.records[level]
}
