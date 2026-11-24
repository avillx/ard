package migrations

import (
	"fmt"
	"log/slog"
	"path"
)

// migration3 moves config files from the data root into the "conf" folder.
func migration3(root Storage, logger *slog.Logger) error {
	const (
		configDir     = "conf"
		configDirMode = 0o755
	)

	exists := func(p string) bool {
		_, err := root.Stat(p)
		return err == nil
	}

	configs := []string{
		"mcp.toml",
		"memory.toml",
		"models.toml",
		"secrets.toml",
		"tasks.toml",
	}

	if err := root.MkdirAll(configDir, configDirMode); err != nil {
		return fmt.Errorf("create %s folder: %w", configDir, err)
	}

	for _, configFile := range configs {
		if exists(configFile) {
			dst := path.Join(configDir, configFile)

			if exists(dst) {
				continue
			}

			if err := root.Rename(configFile, dst); err != nil {
				logger.Warn("move config file failed", "file", configFile, "error", err)
			}
		}
	}

	return nil
}
