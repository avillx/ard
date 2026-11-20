package session

import (
	"ard/internal/profile"
	"log/slog"
)

var _ SessionsRepo = (*Service)(nil)

type SessionsRepo interface {
	Session(SessionID ID) (Session, error)
	Save(Session Session) error
	Delete(SessionID ID) error
	Headers() ([]Header, error)
}

type UUIDGenerator interface {
	New() string
}

type Service struct {
	repo   SessionsRepo
	uuid   UUIDGenerator
	logger *slog.Logger
}

func NewService(
	repo SessionsRepo,
	uuid UUIDGenerator,
	logger *slog.Logger,
) *Service {
	return &Service{
		repo:   repo,
		uuid:   uuid,
		logger: logger.WithGroup("sessions"),
	}
}

func (s *Service) Create(profileID profile.ID, instruction string) (ID, error) {
	// TODO: Check profile existence

	newSessionID := ID(s.uuid.New())
	newSession := NewSession(newSessionID, profileID)

	if instruction != "" {
		newExtras := map[string]any{
			"instruction": instruction,
		}
		newSession.SetExtras(newExtras)
	}

	if err := s.repo.Save(newSession); err != nil {
		return "", err
	}

	s.logger.Info("created new", "session_id", newSession.ID())

	return newSession.ID(), nil
}

func (s *Service) Session(id ID) (Session, error) {
	return s.repo.Session(id)
}

func (s *Service) Save(sess Session) error {
	return s.repo.Save(sess)
}

func (s *Service) Delete(id ID) error {
	return s.repo.Delete(id)
}

func (s *Service) Headers() ([]Header, error) {
	return s.repo.Headers()
}
