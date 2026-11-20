package migrations

import (
	"errors"
	"io/fs"
	"log/slog"
)

// migration2 renames the legacy "agent.log" file to "sys.log".
func migration2(root Storage, logger *slog.Logger) error {
	const (
		legacyLogFile = "agent.log"
		logFile       = "sys.log"
	)

	exists := func(p string) bool {
		_, err := root.Stat(p)
		return err == nil
	}

	if exists(logFile) {
		return nil
	}

	if err := root.Rename(legacyLogFile, logFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("rename log file failed", "error", err)
	}

	return nil
}
