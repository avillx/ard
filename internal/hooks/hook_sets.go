package hooks

import (
	"ard/internal/agent"
	"ard/internal/files"
	"ard/internal/logging"
	"path/filepath"
)

const _24kb = 24 * 1024

func NewAgentHooks(
	todoStorage todoStorage,
	r Replcaer,
) ([]any, error) {
	shellHook, err := NewShellHook()
	if err != nil {
		return nil, err
	}

	accessRules := []Rule{
		{Pattern: logging.AgentLogFile, Access: Read},
		{Pattern: files.SecretsConfigFile, Access: Read},
		{Pattern: files.ProfileFilePath, Access: No},
		{Pattern: filepath.Join(files.SessionsFolder, "**"), Access: No},
		{Pattern: filepath.Join(files.ActivityFolder, "**"), Access: Read},
		{Pattern: "**", Access: Write},
	}

	accessHook, err := NewFileAccessHook(accessRules...)
	if err != nil {
		return nil, err
	}

	return []any{
		accessHook,
		shellHook,
		// &EmptyAnswerHook{},
		NewUndoneTodoHook(todoStorage),
		&ContentSizeLimitHook{limitBytes: _24kb},
		&NeverReadSecretsHook{r},
		&NeverToolCallSecretsHook{r},
		&NeverTypeSecretsHook{r},
	}, nil
}

func NewMemoryHooksResolver(
	indexer agent.MemoryIndexer,
) (func() []any, error) {
	// for validation path patterns
	_, err := NewMemoryHooks(indexer)
	if err != nil {
		return nil, err
	}

	// produce factory
	return func() []any {
		hooks, _ := NewMemoryHooks(indexer)
		return hooks
	}, nil
}

func NewMemoryHooks(
	indexer agent.MemoryIndexer,
) ([]any, error) {
	accessRules := []Rule{
		{Pattern: filepath.Join(files.ActivityFolder, "**"), Access: Read},
		{Pattern: filepath.Join(files.MemoryFolder, "**"), Access: Write},
	}

	accessHook, err := NewFileAccessHook(accessRules...)
	if err != nil {
		return nil, err
	}

	return []any{
		accessHook,
		&OnlyValidMemoryFrontmatterHook{indexer: indexer},
		&EmptyAnswerHook{},
	}, nil
}
