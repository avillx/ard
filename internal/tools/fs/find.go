package fstools

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"ard/internal/agent"
	"ard/internal/files"
	"ard/internal/tools"
	"ard/internal/types"
	"github.com/bmatcuk/doublestar/v4"
)

var lineBreaker = []byte("\n")

type FindTool struct {
	storage      files.FileStorage
	skipPatterns []string
}

func NewFindTool(fs files.FileStorage, skipPatterns []string) (*FindTool, error) {
	// validate skip patterns
	for _, p := range skipPatterns {
		if !doublestar.ValidatePattern(p) {
			return nil, fmt.Errorf("invalid pattern '%s'", p)
		}
	}

	return &FindTool{
		storage:      fs,
		skipPatterns: skipPatterns,
	}, nil
}

func (t *FindTool) Name() agent.ToolName { return "find" }

func (t *FindTool) Description() string {
	return "glob-like path search; When regex specified returns matched file content (grep-like behaviour)"
}

func (t *FindTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"glob": map[string]any{
				"type":        "string",
				"description": "Glob pattern e.g. **/**.md",
			},
			"regex": map[string]any{
				"type": "string",
				"description": `Go-compitable regular expression,
				serve to find content in files; Adds matched file entry to results`,
			},
		},
		"required": []string{"glob"},
	}
}

func (t *FindTool) Call(_ context.Context, rawArgs agent.ToolArguments) ([]agent.ContentPart, error) {
	args, err := tools.UnwrapArgs[struct {
		Glob  string  `json:"glob"`
		Regex *string `json:"regex"`
	}](rawArgs)
	if err != nil {
		return nil, err
	}

	globResult, err := t.glob(args.Glob)
	if err != nil {
		// may be return anything?
		return nil, mapErrs(err)
	}

	if args.Regex == nil {
		repr := strings.Join(globResult, "\n")
		return tools.Result(repr), nil
	}

	regex, err := regexp.Compile(*args.Regex)
	if err != nil {
		return nil, types.NewAgentMistakeError("bad regex, allowed only Golang regexp syntax")
	}

	var sb strings.Builder
	for _, path := range globResult {
		matchedLines, err := t.regexMatchedLines(path, regex)
		if err != nil {
			matchedLines = []string{fmt.Sprintf("Can't read file: %v", err)}
		}
		if len(matchedLines) <= 0 {
			// skip when has no matches
			continue
		}
		fileRepr := strings.Join(append([]string{path}, matchedLines...), "\n")
		fmt.Fprintf(&sb, "%s\n", fileRepr)
	}

	return tools.Result(sb.String()), nil
}

func (t *FindTool) regexMatchedLines(path string, regex *regexp.Regexp) ([]string, error) {
	matchedLines := []string{}
	data, err := t.storage.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// is not an actually error, just no need to check binaries
	if isBinary(data) {
		return matchedLines, nil
	}

	for _, match := range regex.FindAllIndex(data, -1) {
		lineNumber, lineContent := lineAt(data, match[0])
		lineRepr := fmt.Sprintf("line: %d | %s", lineNumber, lineContent)
		matchedLines = append(matchedLines, lineRepr)
	}

	return matchedLines, nil
}

func (t *FindTool) glob(glob string) ([]string, error) {
	matches := []string{}
	err := doublestar.GlobWalk(t.storage.FS(), filepath.ToSlash(glob), func(path string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}

		// should skip
		for _, pattern := range t.skipPatterns {
			if doublestar.PathMatchUnvalidated(pattern, path) {
				return nil
			}
		}

		matches = append(matches, path)

		return nil
	}, doublestar.WithCaseInsensitive())
	if err != nil {
		return nil, err
	}

	return matches, nil
}

func lineAt(data []byte, idx int) (int, string) {
	start := bytes.LastIndex(data[:idx], lineBreaker)

	// to exclude start index of "\n" e.g. "\nmatch" -> "match"
	// also it replace -1 to 0. no match -> zero index (start in first line)
	start++

	end := bytes.Index(data[start:], lineBreaker)
	if end == -1 {
		// to avoid -1 if it is last line
		end = len(data)
	} else {
		// cause endline is relative to data[startLineIdx:]
		// start line is guranteed > 0
		end += start
	}

	lineNumber := bytes.Count(data[:idx], lineBreaker) + 1
	lineContent := string(data[start:end])

	return lineNumber, lineContent
}
