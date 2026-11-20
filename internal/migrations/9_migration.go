package migrations

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"

	toml "github.com/pelletier/go-toml/v2"
)

// legacyAgentProfile is the old "agent.toml" layout, frozen in time.
// It must never follow changes of the living profile DTO.
type legacyAgentProfile struct {
	Description  string   `toml:"description,omitempty"`
	Model        string   `toml:"model,omitempty"`
	HasMemory    bool     `toml:"memory,omitempty"`
	ToolServers  []string `toml:"tool_servers,omitempty"`
	SystemPrompt string   `toml:"prompt"                 multiline:"true"`
}

type profileTable = map[string]any

// migration9 collects the legacy per-agent "agent.toml" configs into "conf/profiles.toml"
// and removes the migrated agent files.
func migration9(root Storage, logger *slog.Logger) error {
	const (
		configFile    = "agent.toml"
		configDir     = "conf"
		configDirMode = 0o755
		profilesFile  = "profiles.toml"
		filePerm      = 0o644
	)

	profilesPath := path.Join(configDir, profilesFile)

	exists := func(p string) bool {
		_, err := root.Stat(p)
		return err == nil
	}

	removeEmptyAgentFolder := func(dir string) {
		entries, err := fs.ReadDir(root.FS(), dir)
		if err != nil {
			return
		}
		if len(entries) > 0 {
			return
		}

		if err := root.Remove(dir); err != nil {
			logger.Warn("remove emptied agent folder failed", "agent", dir, "error", err)
		}
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return fmt.Errorf("read data root: %w", err)
	}

	profiles := map[string]legacyAgentProfile{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		src := path.Join(e.Name(), configFile)
		if !exists(src) {
			continue
		}

		data, err := root.ReadFile(src)
		if err != nil {
			logger.Warn("read legacy agent config failed", "agent", e.Name(), "error", err)
			continue
		}

		var profile legacyAgentProfile
		if err := toml.Unmarshal(data, &profile); err != nil {
			logger.Warn("parse legacy agent config failed", "agent", e.Name(), "error", err)
			continue
		}

		profiles[e.Name()] = profile
	}

	if len(profiles) == 0 {
		return nil
	}

	existing := map[string]profileTable{}
	if exists(profilesPath) {
		data, err := root.ReadFile(profilesPath)
		if err != nil {
			logger.Warn("read profiles config failed", "file", profilesPath, "error", err)
			return nil
		}
		if err := toml.Unmarshal(data, &existing); err != nil {
			logger.Warn("parse profiles config failed", "file", profilesPath, "error", err)
			return nil
		}
	}

	added := false
	migrated := map[string]bool{}

	for id, profile := range profiles {
		if _, ok := existing[id]; ok {
			logger.Warn("skip duplicated agent profile", "agent", id)
			migrated[id] = true
			continue
		}

		// keep existing profile entries intact when a newer build adds fields
		data, err := toml.Marshal(profile)
		if err != nil {
			logger.Warn("convert legacy agent config failed", "agent", id, "error", err)
			continue
		}

		var table profileTable
		if err := toml.Unmarshal(data, &table); err != nil {
			logger.Warn("convert legacy agent config failed", "agent", id, "error", err)
			continue
		}

		existing[id] = table
		migrated[id] = true
		added = true
	}

	if added {
		if err := root.MkdirAll(configDir, configDirMode); err != nil {
			return fmt.Errorf("create %s folder: %w", configDir, err)
		}

		data, err := toml.Marshal(existing)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", profilesPath, err)
		}

		if err := root.WriteFile(profilesPath, data, filePerm); err != nil {
			return fmt.Errorf("write %s: %w", profilesPath, err)
		}
	}

	for id := range migrated {
		if err := root.Remove(path.Join(id, configFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("remove legacy agent config failed", "agent", id, "error", err)
			continue
		}

		removeEmptyAgentFolder(id)
	}

	return nil
}
