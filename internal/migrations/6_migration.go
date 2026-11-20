package migrations

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
)

// migration6 moves per-agent "sessions" folders into the root "sessions" folder.
func migration6(root Storage, logger *slog.Logger) error {
	const (
		sessionsDir   = "sessions"
		agentMarkdown = "agent.md"
		agentConfig   = "agent.toml"

		dirPerm = 0o755
	)

	exists := func(p string) bool {
		_, err := root.Stat(p)
		return err == nil
	}

	moveTo := func(src, dst string) error {
		if parent := path.Dir(dst); parent != "." {
			if err := root.MkdirAll(parent, dirPerm); err != nil {
				return err
			}
		}
		return root.Rename(src, dst)
	}

	isAgentDir := func(e fs.DirEntry) bool {
		if !e.IsDir() {
			return false
		}

		for _, config := range []string{agentMarkdown, agentConfig} {
			stat, err := root.Stat(path.Join(e.Name(), config))
			if err == nil && !stat.IsDir() {
				return true
			}
		}

		return false
	}

	moveSession := func(src, dst string) error {
		// a session id is unique, so a duplicate name means the same session already moved
		if exists(dst) {
			logger.Warn("skip duplicated session file", "agent", path.Dir(src), "file", path.Base(src))
			return root.Remove(src)
		}
		return root.Rename(src, dst)
	}

	var mergeDirs func(src, dst string) error
	mergeDirs = func(src, dst string) error {
		if err := root.MkdirAll(dst, dirPerm); err != nil {
			return err
		}

		entries, err := fs.ReadDir(root.FS(), src)
		if err != nil {
			return err
		}

		for _, e := range entries {
			srcPath := path.Join(src, e.Name())
			dstPath := path.Join(dst, e.Name())

			if e.IsDir() {
				if err := mergeDirs(srcPath, dstPath); err != nil {
					return err
				}
				if err := root.Remove(srcPath); err != nil {
					return err
				}
				continue
			}

			if err := moveSession(srcPath, dstPath); err != nil {
				return err
			}
		}

		return nil
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return fmt.Errorf("read data root: %w", err)
	}

	for _, e := range entries {
		if !isAgentDir(e) {
			continue
		}

		src := path.Join(e.Name(), sessionsDir)
		if !exists(src) {
			continue
		}

		// nothing to merge with: move the whole folder
		if !exists(sessionsDir) {
			if err := moveTo(src, sessionsDir); err == nil {
				continue
			}
		}

		if err := mergeDirs(src, sessionsDir); err != nil {
			logger.Warn("migrate sessions folder failed", "agent", e.Name(), "error", err)
			continue
		}

		if err := root.Remove(src); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("remove migrated sessions folder failed", "agent", e.Name(), "error", err)
		}
	}

	return nil
}
