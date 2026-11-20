package profile

import (
	"ard/internal/agent"
	"errors"
	"fmt"
)

type Repo interface {
	All() ([]agent.Agent, error)
	Get(agent.ID) (agent.Agent, error)
	Save(agent.Agent) error

	// TODO: tasks is not updating automaticly
	Delete(agent.ID) error
}

var _ Repo = (*Service)(nil)

type Sync interface {
	DeleteProfile(agent.ID) error
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

func (s *Service) All() ([]agent.Agent, error) {
	return s.storage.All()
}

func (s *Service) Get(agentID agent.ID) (agent.Agent, error) {
	return s.storage.Get(agentID)
}

func (s *Service) Save(agt agent.Agent) error {
	// validate model existence
	if _, err := s.modelRepo.Get(agt.Model()); err != nil {
		return err
	}

	// validate tool servers existence
	if _, err := s.toolReg.ToolServers(agt.ToolServers()...); err != nil {
		return err
	}

	return s.storage.Save(agt)
}

func (s *Service) Delete(agentID agent.ID) error {
	// validate agent existence
	if _, err := s.storage.Get(agentID); err != nil {
		return err
	}

	if err := s.storage.Delete(agentID); err != nil {
		return err
	}

	var errs []error
	for _, syncr := range s.syncs {
		err := syncr.DeleteProfile(agentID)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func buildDefaultAgent() agent.Agent {
	return agent.NewAgent(
		"default",
		"agent placeholder",
		"",
		"",
		[]string{"filesystem"},
		false,
	)
}
