package cleanup

import (
	"ard/internal/session"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type SessionsCleaner struct {
	sessRepo session.SessionsRepo
	logger   *slog.Logger

	mu sync.Mutex
}

func NewSessionsCleaner(
	sessRepo session.SessionsRepo,

	logger *slog.Logger,
) *SessionsCleaner {
	return &SessionsCleaner{
		sessRepo: sessRepo,
		logger:   logger.WithGroup("session_cleaner"),
	}
}

func (c *SessionsCleaner) Clean(retention time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	headers, err := c.sessRepo.Headers()
	if err != nil {
		if brokenHeadersErr, ok := errors.AsType[*session.BrokenHeadersError](err); ok {
			c.handleBrokenSessions(brokenHeadersErr)
		}
	}

	c.eliminateDepricatedSessions(retention, headers)

	return nil
}

func (c *SessionsCleaner) handleBrokenSessions(brokenHeadersErr *session.BrokenHeadersError) {
	for _, brokenHeaderErr := range brokenHeadersErr.Errors {
		logger := c.logger.With(
			"session", brokenHeaderErr.SessID,
			"cause", brokenHeaderErr.Unwrap(),
		)

		if err := c.sessRepo.Delete(brokenHeaderErr.SessID); err != nil {
			logger.Error("deletion broken session", "error", err)
			continue
		}

		logger.Warn("broken session deleted")
	}
}

func (c *SessionsCleaner) eliminateDepricatedSessions(
	retention time.Duration,
	headers []session.Header,
) {
	cutDate := time.Now().Add(-retention)
	for _, h := range headers {
		if h.UpdatedAt().After(cutDate) {
			continue
		}

		logger := c.logger.With("session", h.ID())

		if err := c.sessRepo.Delete(h.ID()); err != nil {
			logger.Error("deletion deprecated session", "error", err)
			continue
		}
		logger.Info("deprecated session deleted")
	}
}
