package task

import (
	"ard/internal/agent"
	"ard/internal/chat"
	"ard/internal/profile"
	"ard/internal/prompt"
	"ard/internal/runtime"
	"ard/internal/session"
	"context"
	"log/slog"
)

var _ Executor = (*executor)(nil)

type executor struct {
	sessionSvc   *session.Service
	chatExecutor chat.Executor
	logger       *slog.Logger
}

func NewExecutor(
	sessionSvc *session.Service,
	chatExecutor chat.Executor,
	logger *slog.Logger,
) *executor {
	return &executor{
		sessionSvc:   sessionSvc,
		chatExecutor: chatExecutor,
		logger:       logger,
	}
}

func (s *executor) Execute(ctx context.Context, t Config) {
	logger := s.logger.With("task", t.Name)

	for _, r := range t.Recipients {
		logger = logger.With("agent", r)

		logger.InfoContext(ctx, "processing started")
		if err := s.processRecipientTask(ctx, r, t.Request); err != nil {
			logger.ErrorContext(ctx, "processing", "error", err)
		}
	}
}

func (s *executor) processRecipientTask(ctx context.Context, profileID profile.ID, request string) error {
	// TODO: just a user message over session instruction
	sessID, err := s.sessionSvc.Create(profileID, prompt.GetAutonomusGuidance())
	if err != nil {
		return err
	}

	sink := make(chan runtime.Event)
	defer close(sink)

	go func() {
		for range sink {
			// just swallow updates. cause has no recivier
		}
	}()

	return s.chatExecutor.Chat(
		ctx,
		chat.Request{
			SessionID:           sessID,
			UserMessage:         agent.NewUserMessage(request),
			Logging:             true,
			Sink:                sink,
			ProvidedToolServers: []agent.ToolServer{},
		},
	)
}
