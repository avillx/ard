package migrations

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
)

// migration4 moves per-agent "activity" folders into the root "activity" folder.
func migration4(root Storage, logger *slog.Logger) error {
	const (
		activityDir   = "activity"
		agentMarkdown = "agent.md"
		agentConfig   = "agent.toml"

		dirPerm  = 0o755
		filePerm = 0o644
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

	mergeFiles := func(src, dst string) error {
		if !exists(dst) {
			return root.Rename(src, dst)
		}

		srcData, err := root.ReadFile(src)
		if err != nil {
			return err
		}
		dstData, err := root.ReadFile(dst)
		if err != nil {
			return err
		}

		if err := root.WriteFile(dst, append(dstData, srcData...), filePerm); err != nil {
			return err
		}

		return root.Remove(src)
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

			if err := mergeFiles(srcPath, dstPath); err != nil {
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

		src := path.Join(e.Name(), activityDir)
		if !exists(src) {
			continue
		}

		// nothing to merge with: move the whole folder
		if !exists(activityDir) {
			if err := moveTo(src, activityDir); err == nil {
				continue
			}
		}

		if err := mergeDirs(src, activityDir); err != nil {
			logger.Warn("migrate activity folder failed", "agent", e.Name(), "error", err)
			continue
		}

		if err := root.Remove(src); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("remove migrated activity folder failed", "agent", e.Name(), "error", err)
		}
	}

	return nil
}
