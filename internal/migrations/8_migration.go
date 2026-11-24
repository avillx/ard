package migrations

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
)

// migration8 moves leftover content of agent folders into the "repos" folder.
func migration8(root Storage, logger *slog.Logger) error {
	const (
		reposDir      = "repos"
		reposDirMode  = 0o777
		skillsDir     = "skills"
		skillsDirMode = 0o777
		dirPerm       = 0o755

		agentMarkdown = "agent.md"
		agentConfig   = "agent.toml"
	)

	keepInAgent := map[string]bool{
		"activity":    true,
		"memory":      true,
		"sessions":    true,
		agentMarkdown: true,
		agentConfig:   true,
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
				logger.Warn("move agent folder to repos failed", "src", src, "error", err)
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
			logger.Warn("move agent file to repos failed", "src", src, "error", err)
		}
	}

	moveAgentSkills := func(agent string) {
		agentSkills := path.Join(agent, skillsDir)

		entries, err := fs.ReadDir(root.FS(), agentSkills)
		if err != nil {
			return
		}

		if err := root.MkdirAll(skillsDir, skillsDirMode); err != nil {
			logger.Warn("create skills folder failed", "error", err)
			return
		}

		for _, e := range entries {
			src := path.Join(agentSkills, e.Name())
			dst := freePath(path.Join(skillsDir, e.Name()+"-moved"))
			if err := moveTo(src, dst); err != nil {
				logger.Warn("move agent skill failed", "skill", src, "error", err)
			}
		}

		if err := root.Remove(agentSkills); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("remove agent skills folder failed", "agent", agent, "error", err)
		}
	}

	moveAgentLeftovers := func(agent string) {
		entries, err := fs.ReadDir(root.FS(), agent)
		if err != nil {
			return
		}

		for _, e := range entries {
			if keepInAgent[e.Name()] {
				continue
			}
			moveToRepos(path.Join(agent, e.Name()), agent+"-"+e.Name(), e.IsDir())
		}
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return fmt.Errorf("read data root: %w", err)
	}

	for _, e := range entries {
		if !isAgentDir(e) {
			continue
		}
		moveAgentSkills(e.Name())
		moveAgentLeftovers(e.Name())
	}

	return nil
}
