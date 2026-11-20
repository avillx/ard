package fstools

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"ard/internal/agent"
	"ard/internal/files"
	"ard/internal/tools"
	"ard/internal/types"
	"github.com/bmatcuk/doublestar/v4"
)

const DirectoryReadDepthLimit = 2

type ReadTool struct {
	storage      files.FileStorage
	skipPatterns []string
}

func NewReadTool(storage files.FileStorage, skipPatterns []string) (*ReadTool, error) {
	// validate skip patterns
	for _, p := range skipPatterns {
		if !doublestar.ValidatePattern(p) {
			return nil, fmt.Errorf("invalid pattern '%s'", p)
		}
	}

	return &ReadTool{
		storage:      storage,
		skipPatterns: skipPatterns,
	}, nil
}

func (t *ReadTool) Name() agent.ToolName { return "read" }

func (t *ReadTool) Description() string {
	return `Read text files, image files and directories, text files optionally
limited to a line range, reads dir structure in 2 levels depth`
}

func (t *ReadTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to file or directory, e.g. `shared` 'shared/project-x/README.md', 'shared/img.png'",
			},
			"start_line": map[string]any{
				"type":        "number",
				"description": "First line of text file to read (default: 1)",
			},
			"end_line": map[string]any{
				"type":        "number",
				"description": "Last line of text file to read (default: end of file)",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadTool) Call(_ context.Context, rawArgs agent.ToolArguments) ([]agent.ContentPart, error) {
	args, err := tools.UnwrapArgs[struct {
		Path      string `json:"path"`
		StartLine *int   `json:"start_line,omitempty"`
		EndLine   *int   `json:"end_line,omitempty"`
	}](rawArgs)
	if err != nil {
		return nil, err
	}

	// read directory
	info, err := t.storage.Stat(args.Path)
	if err != nil {
		return nil, mapErrs(err)
	}
	if info.IsDir() {
		repr, err := t.ReadDir(args.Path)
		if err != nil {
			return nil, mapErrs(err)
		}
		return tools.Result(repr), nil
	}

	// if is image
	if imageType := detectImageType(args.Path); imageType != "" {
		data, err := t.storage.ReadFile(args.Path)
		if err != nil {
			return nil, err
		}

		content, err := agent.NewImageContent(imageType, data)
		if err != nil {
			return nil, types.NewAgentMistakeError(err.Error())
		}

		return []agent.ContentPart{content}, nil
	}

	text, err := t.ReadTextFile(args.Path, args.StartLine, args.EndLine)
	if err != nil {
		return nil, mapErrs(err)
	}
	return tools.Result(text), nil
}

func (t *ReadTool) ReadTextFile(p string, start, end *int) (string, error) {
	data, err := t.storage.ReadFile(p)
	if err != nil {
		return "", mapErrs(err)
	}

	if isBinary(data) {
		return "", types.NewAgentMistakeError("you can't read binary file")
	}

	if start != nil || end != nil {
		lines := extractLines(data, start, end)
		return lines, nil
	}

	return string(data), nil
}

func (t *ReadTool) ReadDir(p string) (string, error) {
	var sb strings.Builder
	rootParts := strings.Split(p, string(os.PathSeparator))
	err := fs.WalkDir(t.storage.FS(), filepath.ToSlash(p), func(path string, d fs.DirEntry, err error) error {
		// Should skip by pattern
		for _, pattern := range t.skipPatterns {
			if doublestar.PathMatchUnvalidated(pattern, path) {
				return fs.SkipDir
			}
		}

		// Represent path
		labels := t.defineLabels(path, d, err)
		fmt.Fprintf(&sb, "%s %s\n", path, strings.Join(labels, " "))

		if err != nil {
			return fs.SkipDir
		}

		// Skip if depth limit reached
		if d.IsDir() && isDepthLevelLimit(rootParts, filepath.Clean(path), DirectoryReadDepthLimit) {
			return fs.SkipDir
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return sb.String(), nil
}

func isDepthLevelLimit(rootPathParts []string, currentPath string, depthLimit int) bool {
	parts := strings.Split(currentPath, string(os.PathSeparator))
	return len(parts) > len(rootPathParts)+depthLimit
}

func (t *ReadTool) defineLabels(path string, d fs.DirEntry, err error) []string {
	labels := []string{}

	// error label
	if err != nil {
		errLabel := fmt.Sprintf("%s [err: %v]\n", path, err)
		labels = append(labels, errLabel)
		return labels
	}

	// directory label
	if d.IsDir() {
		labels = append(labels, "[dir]")
	}

	// size label
	info, err := d.Info()
	if err == nil {
		size := int(info.Size())
		labels = append(labels, files.FormatSize(size))
	}

	data, err := t.storage.ReadFile(path)
	if err == nil {
		if isBinary(data) {
			// Binary file add binary label
			labels = append(labels, "[bin]")
		} else {
			// Text file add a lines label
			lineCount := bytes.Count(data, []byte("\n"))
			if len(data) > 0 && data[len(data)-1] != '\n' {
				lineCount++
			}
			labels = append(labels, fmt.Sprintf("[%d lines]", lineCount))
		}
	}

	return labels
}

func detectImageType(p string) agent.AllowedMIME {
	switch strings.TrimPrefix(path.Ext(p), ".") {
	case "png":
		return agent.Png
	case "bmp":
		return agent.Bmp
	case "jpeg", "jpg":
		return agent.Jpeg
	case "webp":
		return agent.Webp
	default:
		return ""
	}
}

func extractLines(data []byte, from, to *int) string {
	lines := strings.Split(string(data), "\n")
	total := len(lines)

	startLine := 1
	endLine := total

	if from != nil {
		startLine = *from
	}
	if to != nil {
		endLine = *to
	}

	startLine = max(1, min(startLine, total))
	endLine = max(startLine, min(endLine, total))

	return strings.Join(lines[startLine-1:endLine], "\n")
}
