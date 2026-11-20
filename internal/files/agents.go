package files

import (
	"ard/internal/agent"
	"ard/internal/types"
	"path/filepath"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	AgentFile     = "profiles.toml"
	AgentFileMode = 0o755
)

var AgentFilePath = filepath.Join(ConfigDir, AgentFile) //nolint: gochecknoglobals // os safe path

var _ agent.Repo = (*AgentFiles)(nil)

type AgentFiles struct {
	storage FileStorage
	mu      sync.RWMutex
}

func NewAgentFiles(
	storage FileStorage,
) *AgentFiles {
	return &AgentFiles{
		storage: storage,
	}
}

func (s *AgentFiles) All() ([]agent.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}

	agents := []agent.Agent{}
	for id, profile := range cfg {
		agt := profileToAgent(id, profile)
		agents = append(agents, agt)
	}

	return agents, nil
}

func (s *AgentFiles) Get(id agent.ID) (agent.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}

	if profile, ok := cfg[id]; ok {
		return profileToAgent(id, profile), nil
	}

	return nil, types.ErrIsNotExist
}

func (s *AgentFiles) Save(agt agent.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}

	cfg[agt.ID()] = ProfileDTO{
		Description:  agt.Description(),
		Model:        agt.Model(),
		HasMemory:    agt.HasMemory(),
		ToolServers:  agt.ToolServers(),
		SystemPrompt: agt.SystemPrompt(),
	}

	return s.saveConfig(cfg)
}

func (s *AgentFiles) Delete(agentID agent.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}

	delete(cfg, agentID)

	return s.saveConfig(cfg)
}

func (s *AgentFiles) loadConfig() (map[agent.ID]ProfileDTO, error) {
	data, err := s.storage.ReadFile(AgentFilePath)
	if err != nil {
		return nil, err
	}

	var dto map[agent.ID]ProfileDTO
	if err := toml.Unmarshal(data, &dto); err != nil {
		return nil, err
	}

	return dto, nil
}

func (s *AgentFiles) saveConfig(cfg map[agent.ID]ProfileDTO) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return nil
	}

	return s.storage.WriteFile(AgentFilePath, data, AgentFileMode)
}

type ProfileDTO struct {
	Description  string   `toml:"description,omitempty"`
	Model        string   `toml:"model,omitempty"`
	HasMemory    bool     `toml:"memory,omitempty"`
	ToolServers  []string `toml:"tool_servers,omitempty"`
	SystemPrompt string   `toml:"prompt"                 multiline:"true"`
}

func profileToAgent(id agent.ID, profile ProfileDTO) agent.Agent {
	return agent.NewAgent(
		id,
		profile.Description,
		profile.SystemPrompt,
		profile.Model,
		profile.ToolServers,
		profile.HasMemory,
	)
}
