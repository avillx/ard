package migrations

import (
	"fmt"
	"io/fs"
	"log/slog"
	"path"
)

// migration1 renames the legacy "shared" folder to "repos".
func migration1(root Storage, logger *slog.Logger) error {
	const (
		sharedDir = "shared"
		reposDir  = "repos"
		dirPerm   = 0o755
	)

	exists := func(p string) bool {
		_, err := root.Stat(p)
		return err == nil
	}

	freePath := func(p string) string {
		candidate := p
		for i := 1; exists(candidate); i++ {
			candidate = fmt.Sprintf("%s-%d", p, i)
		}
		return candidate
	}

	moveTo := func(src, dst string) error {
		if parent := path.Dir(dst); parent != "." {
			if err := root.MkdirAll(parent, dirPerm); err != nil {
				return err
			}
		}
		return root.Rename(src, dst)
	}

	stat, err := root.Stat(sharedDir)
	if err != nil {
		return nil
	}
	if !stat.IsDir() {
		logger.Warn("shared entry is not a folder, skip", "path", sharedDir)
		return nil
	}

	// no repos yet: rename the whole folder
	if !exists(reposDir) {
		if err := moveTo(sharedDir, reposDir); err == nil {
			return nil
		}
	}

	// repos already exist: move shared content inside
	entries, err := fs.ReadDir(root.FS(), sharedDir)
	if err != nil {
		logger.Warn("read shared folder failed", "error", err)
		return nil
	}

	for _, e := range entries {
		src := path.Join(sharedDir, e.Name())
		dst := freePath(path.Join(reposDir, e.Name()))
		if err := moveTo(src, dst); err != nil {
			logger.Warn("move shared entry failed", "entry", src, "error", err)
		}
	}

	if err := root.Remove(sharedDir); err != nil {
		logger.Warn("remove shared folder failed", "error", err)
	}

	return nil
}
