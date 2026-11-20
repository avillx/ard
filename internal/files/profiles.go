package files

import (
	"ard/internal/profile"
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

func (s *ProfileFiles) Load() (map[profile.ID]profile.Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}

	profiles := map[profile.ID]profile.Profile{}
	for id, dto := range cfg {
		profiles[id] = dtoToProfile(id, dto)
	}

	return profiles, nil
}

func (s *ProfileFiles) Save(profiles map[profile.ID]profile.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := map[profile.ID]ProfileDTO{}
	for id, p := range profiles {
		cfg[id] = ProfileDTO{
			Description:  p.Description(),
			Model:        p.Model(),
			HasMemory:    p.HasMemory(),
			ToolServers:  p.ToolServers(),
			SystemPrompt: p.SystemPrompt(),
		}
	}

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

func dtoToProfile(id profile.ID, p ProfileDTO) profile.Profile {
	return profile.NewProfile(
		id,
		p.Description,
		p.SystemPrompt,
		p.Model,
		p.ToolServers,
		p.HasMemory,
	)
}
