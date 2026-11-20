package migrations

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// legacyAgent is the agent description parsed from the legacy markdown file.
// It is a frozen copy of the layout at that time and must never follow the living agent config.
type legacyAgent struct {
	ID          string   `yaml:"id"                     toml:"id"`
	Description string   `yaml:"description,omitempty"  toml:"description,omitempty"`
	Model       string   `yaml:"model,omitempty"        toml:"model,omitempty"`
	HasMemory   bool     `yaml:"memory,omitempty"       toml:"memory,omitempty"`
	ToolServers []string `yaml:"tool_servers,omitempty" toml:"tool_servers,omitempty"`

	SystemPrompt string `yaml:"-" toml:"prompt" multiline:"true"`
}

// parseLegacyAgentMarkdown splits the legacy "---" frontmatter from the system prompt body.
func parseLegacyAgentMarkdown(data []byte) (legacyAgent, error) {
	const delim = "---"

	s := strings.ReplaceAll(string(data), "\r\n", "\n")

	after, ok := strings.CutPrefix(s, delim+"\n")
	if !ok {
		return legacyAgent{}, errors.New("agent file must start with ---")
	}

	frontmatter, body, ok := strings.Cut(after, "\n"+delim)
	if !ok {
		return legacyAgent{}, errors.New("unclosed frontmatter")
	}

	var agent legacyAgent
	if err := yaml.Unmarshal([]byte(frontmatter), &agent); err != nil {
		return legacyAgent{}, err
	}

	agent.SystemPrompt = strings.TrimPrefix(body, "\n")

	return agent, nil
}

// migration7 converts the legacy per-agent "agent.md" file into "agent.toml".
func migration7(root Storage, logger *slog.Logger) error {
	const (
		legacyMarkdownFile = "agent.md"
		configFile         = "agent.toml"

		filePerm = 0o644
	)

	exists := func(p string) bool {
		_, err := root.Stat(p)
		return err == nil
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return fmt.Errorf("read data root: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		src := path.Join(e.Name(), legacyMarkdownFile)
		dst := path.Join(e.Name(), configFile)

		if !exists(src) || exists(dst) {
			continue
		}

		data, err := root.ReadFile(src)
		if err != nil {
			logger.Warn("read legacy agent file failed", "agent", e.Name(), "error", err)
			continue
		}

		agent, err := parseLegacyAgentMarkdown(data)
		if err != nil {
			logger.Warn("parse legacy agent file failed", "agent", e.Name(), "error", err)
			continue
		}

		// the folder name is the agent id in the storage layout
		agent.ID = e.Name()

		cfg, err := toml.Marshal(agent)
		if err != nil {
			logger.Warn("marshal legacy agent config failed", "agent", e.Name(), "error", err)
			continue
		}

		if err := root.WriteFile(dst, cfg, filePerm); err != nil {
			logger.Warn("write migrated agent config failed", "agent", e.Name(), "error", err)
			continue
		}

		if err := root.Remove(src); err != nil {
			logger.Warn("remove migrated agent file failed", "agent", e.Name(), "error", err)
		}
	}

	return nil
}
