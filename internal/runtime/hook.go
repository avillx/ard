package runtime

import (
	"ard/internal/agent"
	"context"
)

// TODO:
// Rename to PostCompletion / PreToolCall / PostToolCall.
type (
	CompletionHook Hook[*agent.Completion]
	ToolCallHook   Hook[*agent.ToolCall]
	ToolResultHook Hook[*AfterToolCall]
)

type Hook[T any] interface {
	Apply(context.Context, T) (T, error)
}

type AfterToolCall struct {
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
