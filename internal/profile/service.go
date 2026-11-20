package profile

import (
	"ard/internal/agent"
	"errors"
	"fmt"
)

type Repo interface {
	All() ([]Profile, error)
	Get(ID) (Profile, error)
	Save(Profile) error

	// TODO: tasks is not updating automaticly
	Delete(ID) error
}

var _ Repo = (*Service)(nil)

type Sync interface {
	DeleteProfile(ID) error
}

type Service struct {
	toolReg   agent.ToolRegistry
	modelRepo agent.ModelRegistry
	storage   Repo
	syncs     []Sync
}

func NewService(
	toolReg agent.ToolRegistry,
	modelRepo agent.ModelRegistry,
	storage Repo,
	syncs []Sync,
) (*Service, error) {
	svc := &Service{
		toolReg:   toolReg,
		modelRepo: modelRepo,
		storage:   storage,
		syncs:     syncs,
	}

	agts, err := svc.All()
	if err != nil {
		return nil, err
	}

	if len(agts) == 0 {
		if err := storage.Save(buildDefaultAgent()); err != nil {
			return nil, fmt.Errorf("failed to create default agent, %w", err)
		}
	}

	return svc, nil
}

func (s *Service) All() ([]Profile, error) {
	return s.storage.All()
}

func (s *Service) Get(id ID) (Profile, error) {
	return s.storage.Get(id)
}

func (s *Service) Save(p Profile) error {
	// validate model existence
	if _, err := s.modelRepo.Get(p.Model()); err != nil {
		return err
	}

	// validate tool servers existence
	if _, err := s.toolReg.ToolServers(p.ToolServers()...); err != nil {
		return err
	}

	return s.storage.Save(p)
}

func (s *Service) Delete(id ID) error {
	// validate agent existence
	if _, err := s.storage.Get(id); err != nil {
		return err
	}

	if err := s.storage.Delete(id); err != nil {
		return err
	}

	var errs []error
	for _, syncr := range s.syncs {
		err := syncr.DeleteProfile(id)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func buildDefaultAgent() Profile {
	return NewProfile(
		"default",
		"agent placeholder",
		"",
		"",
		[]string{"filesystem"},
		false,
	)
}
