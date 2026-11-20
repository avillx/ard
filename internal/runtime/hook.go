package runtime

import (
	"ard/internal/agent"
	"context"
)

type (
	CompletionHook Hook[*agent.Completion]
	ToolCallHook   Hook[*PreToolCall]
	ToolResultHook Hook[*PostToolCall]
)

type Hook[T any] interface {
	Apply(context.Context, T) (T, error)
}

type PreToolCall struct {
	ToolName  agent.ToolName
	Arguments agent.ToolArguments
}

type PostToolCall struct {
	*agent.ToolCall
	*agent.ToolResult
}

func ApplyHooks[T any](ctx context.Context, hooks []any, event T) (T, error) {
	for _, h := range hooks {
		typedHook, ok := h.(Hook[T])
		if !ok {
			continue
		}

		newEvent, err := typedHook.Apply(ctx, event)
		if err != nil {
			return newEvent, err
		}
		event = newEvent
	}

	return event, nil
}
