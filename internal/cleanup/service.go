package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	minSessRetention   = 10 * time.Hour
	minCleanUpInterval = 5 * time.Hour
)

type Config struct {
	SessRetention   time.Duration
	MaxLogLines     int
	CleanUpInterval time.Duration
}

type LogTrimmer interface {
	Trim(maxLines int) error
}

type Service struct {
	sessionsCleaner *SessionsCleaner
	logTrimmer      LogTrimmer
	logger          *slog.Logger

	cfg Config
}

func NewService(
	cfg Config,
	sessionsCleaner *SessionsCleaner,
	logTrimmer LogTrimmer,
	logger *slog.Logger,
) (*Service, error) {
	if cfg.SessRetention < minSessRetention {
		return nil, fmt.Errorf("too short session retention, must be greather than 24h")
	}

	if cfg.CleanUpInterval < minCleanUpInterval {
		return nil, fmt.Errorf("too short cleanup interval, must be greather than 5h")
	}

	return &Service{
		cfg:             cfg,
		sessionsCleaner: sessionsCleaner,
		logTrimmer:      logTrimmer,
		logger:          logger.WithGroup("cleanup"),
	}, nil
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.CleanUpInterval)
	defer ticker.Stop()

	// clean immidiate
	s.doCleanUp()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.doCleanUp()
		}
	}
}

func (s *Service) doCleanUp() {
	s.logger.Info("invoke")

	s.logger.Info("cleaning sessions")
	if err := s.sessionsCleaner.Clean(s.cfg.SessRetention); err != nil {
		s.logger.Error("sessions", "error", err)
	}

	s.logger.Info("cleaning log")
	if err := s.logTrimmer.Trim(s.cfg.MaxLogLines); err != nil {
		s.logger.Error("agent log file", "error", err)
	}
}
