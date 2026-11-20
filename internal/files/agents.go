package files

import (
	"ard/internal/agent"
	"ard/internal/types"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
)

var _ agent.Repo = (*AgentFiles)(nil)

const AgentFile = "agent.toml"

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

	entries, err := fs.ReadDir(s.storage.FS(), ".")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var dtos []AgentDTO
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		dto, err := s.readConfig(agent.ID(e.Name()))
		if err != nil {
			continue
		}
		dtos = append(dtos, dto)
	}

	return s.fromDTO(dtos...), nil
}

func (s *AgentFiles) Get(id agent.ID) (agent.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dto, err := s.readConfig(id)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("agent %s: %w", id, types.ErrIsNotExist)
		}
		return nil, err
	}

	agts := s.fromDTO(dto)

	if len(agts) > 0 {
		return agts[0], nil
	}

	return nil, errors.New("can't get agent")
}

func validateAgentName(agentName string) error {
	reservedNames := []string{
		ConfigDir,
		ReposDir,
		SkillsDir,
		TMPDir,
	}

	if slices.Contains(reservedNames, agentName) {
		return fmt.Errorf("name %s is reserved", agentName)
	}

	return nil
}

func (s *AgentFiles) Save(agt agent.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateAgentName(string(agt.ID())); err != nil {
		return err
	}

	data, err := marshalAgentFile(agt)
	if err != nil {
		return err
	}

	if err := s.storage.MkdirAll(string(agt.ID()), ModeDirPerm); err != nil {
		return err
	}

	return s.storage.WriteFile(resolveAgentFilePath(agt.ID()), data, ModeFilePerm)
}

func (s *AgentFiles) Delete(agentID agent.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.storage.RemoveAll(fmt.Sprintf("/%s", agentID))
}

func (s *AgentFiles) readConfig(id agent.ID) (AgentDTO, error) {
	data, err := s.storage.ReadFile(resolveAgentFilePath(id))
	if err != nil {
		return AgentDTO{}, err
	}

	return parseTomlAgentFile(data)
}

func (s *AgentFiles) fromDTO(dtos ...AgentDTO) []agent.Agent {
	agents := []agent.Agent{}

	for _, dto := range dtos {
		agents = append(agents, agent.NewAgent(
			dto.ID,
			dto.Description,
			dto.SystemPrompt,
			dto.Model,
			dto.ToolServers,
			dto.HasMemory,
		))
	}

	return agents
}

type AgentDTO struct {
	ID          agent.ID `yaml:"id"                     toml:"id"`
	Description string   `yaml:"description,omitempty"  toml:"description,omitempty"`
	Model       string   `yaml:"model,omitempty"        toml:"model,omitempty"`
	HasMemory   bool     `yaml:"memory,omitempty"       toml:"memory,omitempty"`
	ToolServers []string `yaml:"tool_servers,omitempty" toml:"tool_servers,omitempty"`

	SystemPrompt string `yaml:"-" toml:"prompt" multiline:"true"`
}

func parseTomlAgentFile(data []byte) (AgentDTO, error) {
	var dto AgentDTO
	if err := toml.Unmarshal(data, &dto); err != nil {
		return AgentDTO{}, err
	}
	return dto, nil
}

func marshalAgentFile(agt agent.Agent) ([]byte, error) {
	return toml.Marshal(AgentDTO{
		ID:           agt.ID(),
		Description:  agt.Description(),
		Model:        agt.Model(),
		HasMemory:    agt.HasMemory(),
		SystemPrompt: agt.SystemPrompt(),
		ToolServers:  agt.ToolServers(),
	})
}

func resolveAgentFilePath(agentID agent.ID) string {
	return filepath.Join(string(agentID), AgentFile)
}
