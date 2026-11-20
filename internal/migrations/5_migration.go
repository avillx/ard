package migrations

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
)

// migration5 moves per-agent "memory" folders into the root "memory" folder.
func migration5(root Storage, logger *slog.Logger) error {
	const (
		memoryDir     = "memory"
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

	freePathWithPrefix := func(p string) string {
		if !exists(p) {
			return p
		}

		dir, name := path.Split(p)
		for i := 1; ; i++ {
			candidate := path.Join(dir, fmt.Sprintf("%d-%s", i, name))
			if !exists(candidate) {
				return candidate
			}
		}
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

			// same-named memory files never overwrite each other
			if err := root.Rename(srcPath, freePathWithPrefix(dstPath)); err != nil {
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

		src := path.Join(e.Name(), memoryDir)
		if !exists(src) {
			continue
		}

		// nothing to merge with: move the whole folder
		if !exists(memoryDir) {
			if err := moveTo(src, memoryDir); err == nil {
				continue
			}
		}

		if err := mergeDirs(src, memoryDir); err != nil {
			logger.Warn("migrate memory folder failed", "agent", e.Name(), "error", err)
			continue
		}

		if err := root.Remove(src); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("remove migrated memory folder failed", "agent", e.Name(), "error", err)
		}
	}

	return nil
}
