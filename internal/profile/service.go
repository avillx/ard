package profile

import (
	"ard/internal/agent"
	"ard/internal/prompt"
	"ard/internal/types"
	"errors"
	"maps"
	"slices"
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
	Load() (map[ID]Profile, error)
	Save(map[ID]Profile) error
}

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
	profiles, err := s.storage.Load()
	if err != nil {
		return nil, err
	}
	return slices.Collect(maps.Values(profiles)), nil
}

func (s *Service) IsExist(id ID) (bool, error) {
	m, err := s.storage.Load()
	if err != nil {
		return false, err
	}
	_, ok := m[id]
	return ok, nil
}

func (s *Service) Get(id ID) (Profile, error) {
	defaultCopy := *s.defaultProfile
	p := &defaultCopy

	stored, err := s.storage.Load()
	if err != nil {
		return nil, err
	}

	// default patch with profile
	if id != "" && id != s.defaultProfile.ID() {
		selectedProfile, ok := stored[id]
		if !ok {
			return nil, types.ErrIsNotExist
		}
		p.Patch(selectedProfile)
	}

	// merge with global
	globalProfile, ok := stored[GlobalProfileID]
	if !ok {
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

	profiles, err := s.storage.Load()
	if err != nil {
		return err
	}

	profiles[p.ID()] = p

	return s.storage.Save(profiles)
}

func (s *Service) Delete(id ID) error {
	// validate agent existence

	profiles, err := s.storage.Load()
	if err != nil {
		return err
	}

	delete(profiles, id)

	if err := s.storage.Save(profiles); err != nil {
		return err
	}

	var errs []error
	for _, syncr := range s.syncs {
		err := syncr.DeleteProfile(id)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
