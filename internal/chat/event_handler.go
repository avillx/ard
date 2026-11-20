package chat

import (
	"ard/internal/agent"
	"ard/internal/profile"
	"ard/internal/prompt"
	"ard/internal/runtime"
	"ard/internal/session"
	"ard/internal/types"
	"context"
	"errors"
	"log/slog"
)

type EventHandler interface {
	HandleCompletionMistakeEvent(ctx context.Context, ev *runtime.CompletionMistakeEvent) error
	HandleCompactionEvent(ctx context.Context, ev *runtime.CompactionEvent) error
	HandleCompleteEvent(ctx context.Context, ev *runtime.CompleteEvent) error
	HandleLoopExitEvent(ctx context.Context, ev *runtime.LoopExitEvent) error
	HandleToolCallErrEvent(ctx context.Context, ev *runtime.ToolCallErrEvent) error
	HandleToolResultEvent(ctx context.Context, ev *runtime.ToolResultEvent) error
}

type SessionSaveSink interface {
	Save(session.Session) error
}

type ChatEventHandler struct { //nolint: revive // EventHandler is interface, so this is chat impl of EventHandler
	profileID      profile.ID
	sess           session.Session
	sessSink       SessionSaveSink
	logger         *slog.Logger
	activityLogger ActivityLogger
	observe        bool
}

func NewChatEventHandler(
	profileID profile.ID,
	sess session.Session,
	sessSink SessionSaveSink,
	activityLogger ActivityLogger,
	observe bool,
	logger *slog.Logger,
) *ChatEventHandler {
	return &ChatEventHandler{
		profileID:      profileID,
		sess:           sess,
		sessSink:       sessSink,
		observe:        observe,
		activityLogger: activityLogger,
		logger:         logger.WithGroup("event_handler"),
	}
}

func (h *ChatEventHandler) HandleLoopExitEvent(ctx context.Context, ev *runtime.LoopExitEvent) error {
	switch err := ev.Err(); {
	case err == nil:
		h.logger.InfoContext(ctx, "loop exit")

	case errors.Is(err, context.Canceled):
		h.sess.AddMessages(agent.NewUserMessage(prompt.InterruptionExplanation()))
		h.logger.InfoContext(ctx, "loop interrupted")

	default:
		h.logger.ErrorContext(ctx, "loop exit with error", "error", err)
	}

	if err := h.sessSink.Save(h.sess); err != nil {
		h.logger.ErrorContext(ctx, "save session", "error", err)
	}
	return nil
}

func (h *ChatEventHandler) HandleToolCallErrEvent(ctx context.Context, ev *runtime.ToolCallErrEvent) error {
	err := ev.Err()
	if _, ok := errors.AsType[*types.AgentMistakeError](err); ok {
		h.logger.WarnContext(ctx, "bad tool call",
			"tool", ev.ToolName(),
		)
	} else {
		h.logger.ErrorContext(ctx, "tool call error",
			"tool", ev.ToolName(),
			"error", ev.Err(),
		)
	}
	return nil
}

func (h *ChatEventHandler) HandleToolResultEvent(ctx context.Context, ev *runtime.ToolResultEvent) error {
	h.logger.InfoContext(ctx, "tool result recivied",
		"tool", ev.Call().ToolName,
	)
	h.sess.AddMessages(agent.NewToolResultMessage(ev.Result()))
	return nil
}

func (h *ChatEventHandler) HandleCompletionMistakeEvent(ctx context.Context, ev *runtime.CompletionMistakeEvent) error {
	h.logger.InfoContext(ctx, "compltion mistake",
		"error", ev.Err(),
	)
	return nil
}

func (h *ChatEventHandler) HandleCompleteEvent(ctx context.Context, ev *runtime.CompleteEvent) error {
	c := ev.Complete()
	h.logger.InfoContext(ctx, "completion",
		"tool_calls", len(c.ToolCalls),
		"input_tokens", c.InputTokens,
		"output_tokens", c.CompletionTokens,
	)
	// modify session
	h.sess.ApplyCompletion(c)

	// observe
	if h.observe {
		completion := ev.Complete()
		msg := agent.NewAgentMessage(completion.Content, completion.ToolCalls)
		h.activityLogger.Commit(h.sess.ID(), []agent.Message{msg})
	}
	return nil
}

func (h *ChatEventHandler) HandleCompactionEvent(ctx context.Context, ev *runtime.CompactionEvent) error {
	h.logger.WarnContext(ctx, "session compacted")
	h.sess.OverwriteMessages(0, ev.CompactedContext())
	return nil
}
