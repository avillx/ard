package migrations

import (
	"fmt"
	"io/fs"
	"log/slog"
	"path"
)

// migration10 moves everything unknown out of the data root into the "repos" folder.
func migration10(root Storage, logger *slog.Logger) error {
	const (
		reposDir     = "repos"
		reposDirMode = 0o777
		dirPerm      = 0o755

		agentMarkdown = "agent.md"
		agentConfig   = "agent.toml"
	)

	keepInRoot := map[string]bool{
		"activity": true,
		"memory":   true,
		"sessions": true,
		reposDir:   true,
		"conf":     true,
		"tmp":      true,
		"skills":   true,
		"sys.log":  true,
	}

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

	// dir -> repos/<name>-moved, file -> repos/<name>-moved/<file>
	moveToRepos := func(src, name string, isDir bool) {
		if err := root.MkdirAll(reposDir, reposDirMode); err != nil {
			logger.Warn("create repos folder failed", "error", err)
			return
		}

		if isDir {
			if err := moveTo(src, freePath(path.Join(reposDir, name+"-moved"))); err != nil {
				logger.Warn("move root entry to repos failed", "src", src, "error", err)
			}
			return
		}

		// files share one folder per source name
		dstDir := path.Join(reposDir, name+"-moved")
		if stat, err := root.Stat(dstDir); err == nil && !stat.IsDir() {
			dstDir = freePath(dstDir)
		}
		if err := root.MkdirAll(dstDir, dirPerm); err != nil {
			logger.Warn("create repos folder failed", "folder", dstDir, "error", err)
			return
		}

		if err := moveTo(src, freePath(path.Join(dstDir, path.Base(src)))); err != nil {
			logger.Warn("move root file to repos failed", "src", src, "error", err)
		}
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return fmt.Errorf("read data root: %w", err)
	}

	for _, e := range entries {
		if keepInRoot[e.Name()] || isAgentDir(e) {
			continue
		}
		moveToRepos(e.Name(), e.Name(), e.IsDir())
	}

	return nil
}
