package files

import (
	"ard/internal/profile"
	"ard/internal/types"
	"path/filepath"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	ProfileFile     = "profiles.toml"
	ProfileFileMode = 0o755
)

var ProfileFilePath = filepath.Join(ConfigDir, ProfileFile) //nolint: gochecknoglobals // os safe path

var _ profile.Repo = (*ProfileFiles)(nil)

type ProfileFiles struct {
	storage FileStorage
	mu      sync.RWMutex
}

func NewProfileFiles(
	storage FileStorage,
) *ProfileFiles {
	return &ProfileFiles{
		storage: storage,
	}
}

func (s *ProfileFiles) All() ([]profile.Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}

	profiles := []profile.Profile{}
	for id, profile := range cfg {
		agt := dtoToAgent(id, profile)
		profiles = append(profiles, agt)
	}

	return profiles, nil
}

func (s *ProfileFiles) Get(id profile.ID) (profile.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}

	if profile, ok := cfg[id]; ok {
		return dtoToAgent(id, profile), nil
	}

	return nil, types.ErrIsNotExist
}

func (s *ProfileFiles) Save(p profile.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}

	cfg[p.ID()] = ProfileDTO{
		Description:  p.Description(),
		Model:        p.Model(),
		HasMemory:    p.HasMemory(),
		ToolServers:  p.ToolServers(),
		SystemPrompt: p.SystemPrompt(),
	}

	return s.saveConfig(cfg)
}

func (s *ProfileFiles) Delete(profileID profile.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}

	delete(cfg, profileID)

	return s.saveConfig(cfg)
}

func (s *ProfileFiles) loadConfig() (map[profile.ID]ProfileDTO, error) {
	data, err := s.storage.ReadFile(ProfileFilePath)
	if err != nil {
		return nil, err
	}

	var dto map[profile.ID]ProfileDTO
	if err := toml.Unmarshal(data, &dto); err != nil {
		return nil, err
	}

	return dto, nil
}

func (s *ProfileFiles) saveConfig(cfg map[profile.ID]ProfileDTO) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	return s.storage.WriteFile(ProfileFilePath, data, ProfileFileMode)
}

type ProfileDTO struct {
	Description  string   `toml:"description,omitempty"`
	Model        string   `toml:"model,omitempty"`
	HasMemory    bool     `toml:"memory,omitempty"`
	ToolServers  []string `toml:"tool_servers,omitempty"`
	SystemPrompt string   `toml:"prompt"                 multiline:"true"`
}

func dtoToAgent(id profile.ID, p ProfileDTO) profile.Profile {
	return profile.NewProfile(
		id,
		p.Description,
		p.SystemPrompt,
		p.Model,
		p.ToolServers,
		p.HasMemory,
	)
}
