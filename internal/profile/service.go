package profile

import (
	"ard/internal/agent"
	"ard/internal/prompt"
	"ard/internal/types"
	"errors"
)

const (
	GlobalProfileID           ID     = "global"
	DefaultProfileID          ID     = "default"
	DefaultProfileDescription string = "Agent"
	DefaultMemoryState        bool   = false
	AnyModel                  string = ""

	// FileSystem is it is in profile domain to grant existance this toolnames
	// cause fs for agent is a most important part of buisness logic
	// so it here.
	FileSystem string = "filesystem"
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
	toolReg        agent.ToolRegistry
	modelRepo      agent.ModelRegistry
	storage        Repo
	defaultProfile *profile
	syncs          []Sync
}

func NewService(
	toolReg agent.ToolRegistry,
	modelRepo agent.ModelRegistry,
	storage Repo,
	syncs []Sync,
) (*Service, error) {
	defaultProfile := NewProfile(
		DefaultProfileID,
		DefaultProfileDescription,
		prompt.DefaultAgentProfile(),
		AnyModel,
		[]string{FileSystem},
		DefaultMemoryState,
	)

	svc := &Service{
		defaultProfile: defaultProfile,
		toolReg:        toolReg,
		modelRepo:      modelRepo,
		storage:        storage,
		syncs:          syncs,
	}

	return svc, nil
}

func (s *Service) All() ([]Profile, error) {
	return s.storage.All()
}

func (s *Service) Get(id ID) (Profile, error) {
	defaultCopy := *s.defaultProfile
	p := &defaultCopy

	// default patch with profile
	if id != "" && id != s.defaultProfile.ID() {
		selectedProfile, err := s.storage.Get(id)
		if err != nil {
			return nil, err
		}
		p.Patch(selectedProfile)
	}

	// merge with global
	globalProfile, err := s.storage.Get(GlobalProfileID)
	if err != nil {
		if !errors.Is(err, types.ErrIsNotExist) {
			return nil, err
		}
		globalProfile = NewProfile("", "", "", "", []string{}, false)
	}

	p.Merge(globalProfile)

	return p, nil
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
