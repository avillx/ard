package hooks

import (
	"ard/internal/agent"
	"ard/internal/runtime"
	"ard/internal/tools"
	"ard/internal/tools/shell"
	"context"
	"fmt"
	"regexp"
	"slices"
)

var shellToolName = (&shell.Tool{}).Name() //nolint: gochecknoglobals // for tool name consistency

type ShellPattern interface {
	Match(string) bool
	Message() string
}

var _ ShellPattern = (*RegexShellPattern)(nil)

type RegexShellPattern struct {
	regex   *regexp.Regexp
	message string
}

func NewRegexShellPattern(expr, message string) (*RegexShellPattern, error) {
	compiled, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("compile shell pattern %q: %w", expr, err)
	}

	return &RegexShellPattern{
		regex:   compiled,
		message: message,
	}, nil
}

func (r *RegexShellPattern) Match(command string) bool {
	return r.regex.MatchString(command)
}

func (r *RegexShellPattern) Message() string {
	return r.message
}

var _ runtime.ToolResultHook = (*ShellHook)(nil)

type ShellHook struct {
	patterns []ShellPattern
}

func NewShellHook() (*ShellHook, error) {
	specs := []struct {
		expression string
		message    string
	}{
		{`\bcd\b`, "Use the 'cwd' parameter of the shell tool instead of 'cd'"},
		{`\bmv\b`, "Use the 'move' tool instead of 'mv'"},
		{`\btouch\b`, "Use the 'write' tool to create a file"},
		{`\bmkdir\b`, "Use the 'write' tool to create a directory: it creates missing parents automatically"},
		{`\bcat\b`, "Use the 'read' tool instead of 'cat'"},
		{`\bgrep\b`, "Use the 'find' tool instead of 'grep'"},
		{`\bfind\b`, "Use the native 'find' tool instead of the shell 'find' command"},
		{`\brm\b`, "Use the 'move' tool to move the file into the 'tmp' folder instead of 'rm'"},
		{`\brmdir\b`, "Use the 'move' tool to move the directory into the 'tmp' folder instead of 'rmdir'"},
		{`\bls\b`, "Use the 'read' or 'find' tool to list a directory instead of 'ls'"},
		{`\bsed\b`, "Use the 'find' tool to search and replace in files instead of 'sed'"},
	}

	patterns := make([]ShellPattern, 0, len(specs))
	for _, spec := range specs {
		pattern, err := NewRegexShellPattern(spec.expression, spec.message)
		if err != nil {
			return nil, err
		}

		patterns = append(patterns, pattern)
	}

	return &ShellHook{
		patterns: patterns,
	}, nil
}

func (h *ShellHook) Apply(
	_ context.Context,
	tc *runtime.PostToolCall,
) (*runtime.PostToolCall, error) {
	if tc.ToolName != shellToolName {
		return tc, nil
	}

	args, err := tools.UnwrapArgs[struct {
		Command string `json:"command"`
	}](tc.Arguments)
	if err != nil {
		return nil, err
	}

	for _, p := range h.patterns {
		if p.Match(args.Command) {
			caution := fmt.Sprintf("<caution>%s</caution>", p.Message())
			tc.Result = slices.Concat(tc.Result, agent.NewContent(caution))

			return tc, nil
		}
	}

	return tc, nil
}
