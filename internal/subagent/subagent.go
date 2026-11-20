package subagent

import (
	"ard/internal/agent"
	"ard/internal/chat"
	"ard/internal/profile"
	"ard/internal/prompt"
	"ard/internal/runtime"
	"ard/internal/session"
	"context"
	"errors"
	"fmt"
	"log/slog"
)

var ErrCallStackOverflow = errors.New("sub agent call is overflow")

const maxSubAgentDepth = 3

// buffer of events emitted by a running sub agent loop.
const runtimeEventBuffer = 16

type Service struct {
	chatExecutor chat.Executor
	sessService  *session.Service
	logger       *slog.Logger
}

func NewService(
	chatExecutor chat.Executor,
	sessService *session.Service,
	logger *slog.Logger,
) *Service {
	return &Service{
		chatExecutor: chatExecutor,
		sessService:  sessService,
		logger:       logger.WithGroup("sub_agent"),
	}
}

func (s *Service) Call(
	ctx context.Context,
	profileID profile.ID,
	request string,
) (string, error) {
	ctx, isOverflow := subAgentCallStack(ctx, subAgentCall{subagentProfile: profileID})
	if isOverflow {
		return "", ErrCallStackOverflow
	}

	// create session
	sessID, err := s.sessService.Create(profileID, prompt.SubAgentGuidance())
	if err != nil {
		return "", err
	}

	logger := s.logger.With("profile", profileID, "session", sessID)

	evCh := make(chan runtime.Event, runtimeEventBuffer)
	chatErrCh := make(chan error, 1)

	go func() {
		err := s.chatExecutor.Chat(
			ctx,
			chat.Request{
				SessionID:   sessID,
				UserMessage: agent.NewUserMessage(request),
				Logging:     false,
				Sink:        evCh,
			},
		)
		close(evCh)
		chatErrCh <- err
	}()

	logger.InfoContext(ctx, "running")

	lastAgentMessageContent := ""

	for ev := range evCh {
		switch ev := ev.(type) {
		case *runtime.LoopExitEvent:
			if ev.Err() != nil {
				message := fmt.Sprintf("sub agent: profile %s: fall with error", profileID)
				lastAgentMessageContent += message

				logger.ErrorContext(ctx,
					"fall with error",
					"error", ev.Err(),
				)
			}
		case *runtime.CompleteEvent:
			lastAgentMessageContent = ev.Complete().Content
		}
	}

	if err := <-chatErrCh; err != nil {
		logger.ErrorContext(ctx, "agent has problems with sub agent",
			"error", err,
		)
		return "", err
	}

	return lastAgentMessageContent, nil
}

type ctxKey struct{}

type subAgentCall struct {
	subagentProfile profile.ID
}

func subAgentCallStack(ctx context.Context, s subAgentCall) (context.Context, bool) {
	callStack, _ := ctx.Value(ctxKey{}).([]subAgentCall)
	if len(callStack) >= maxSubAgentDepth {
		return ctx, true
	}
	newStack := append([]subAgentCall{s}, callStack...)
	return context.WithValue(ctx, ctxKey{}, newStack), false
}
