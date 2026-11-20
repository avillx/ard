package chat

import (
	"ard/internal/agent"
	"ard/internal/profile"
	"ard/internal/runtime"
	"ard/internal/session"
	"ard/internal/types"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

type ActivityLogger interface {
	Commit(session.ID, []agent.Message)
}

type Request struct {
	SessionID           session.ID
	UserMessage         *agent.UserMessage
	ProvidedToolServers []agent.ToolServer
	Logging             bool
	Sink                chan runtime.Event
}

func (r Request) validate() error {
	if r.SessionID == "" {
		return fmt.Errorf("completion request must include sessionID")
	}

	if r.UserMessage == nil {
		return fmt.Errorf("completion request must include user message")
	}

	if len(r.UserMessage.Content()) == 0 {
		return fmt.Errorf("user message must has content")
	}

	if r.Sink == nil {
		return fmt.Errorf("sink is required")
	}

	for _, c := range r.UserMessage.Content() {
		if c.ImageURL == "" && c.Text == "" {
			return fmt.Errorf("user message must has no empty content parts")
		}
	}

	return nil
}

var _ Executor = (*Service)(nil)

// buffer of events emitted by a running agent loop.
const runtimeEventBuffer = 16

type Executor interface {
	Chat(ctx context.Context, r Request) error
}

type SystemMessageBuilder interface {
	BuildSystemMessage(
		ctx context.Context,
		p profile.Profile,
		toolServers []agent.ToolServer,
		sess session.Session,
	) (*agent.SystemMessage, error)
}

type Service struct {
	profileSvc           *profile.Service
	sessionRepo          session.SessionsRepo
	modelRepo            agent.ModelRegistry
	toolRegistry         agent.ToolRegistry
	systemMessageBuilder SystemMessageBuilder
	activityLogger       ActivityLogger
	hooks                []any
	logger               *slog.Logger
}

func NewService(
	profileSvc *profile.Service,
	sessionRepo session.SessionsRepo,
	modelRepo agent.ModelRegistry,
	toolRegistry agent.ToolRegistry,
	systemMessageBuilder SystemMessageBuilder,
	activityLogger ActivityLogger,
	hooks []any,
	logger *slog.Logger,
) *Service {
	return &Service{
		profileSvc:           profileSvc,
		sessionRepo:          sessionRepo,
		modelRepo:            modelRepo,
		toolRegistry:         toolRegistry,
		activityLogger:       activityLogger,
		systemMessageBuilder: systemMessageBuilder,
		hooks:                hooks,
		logger:               logger.WithGroup("chat"),
	}
}

func (s *Service) Chat(
	ctx context.Context,
	r Request,
) error {
	// validate request
	if err := r.validate(); err != nil {
		return err
	}

	// session
	sess, err := s.sessionRepo.Session(r.SessionID)
	if err != nil {
		return err
	}

	// profile
	p, err := s.profileSvc.Get(sess.ProfileID())
	if err != nil {
		return err
	}

	// model
	model, err := s.modelRepo.Get(p.Model())
	if err != nil {
		return err
	}

	// tools
	toolServers, err := resolveToolServers(s.toolRegistry, p, r.ProvidedToolServers, s.logger)
	if err != nil {
		return err
	}

	sess.AddMessages(r.UserMessage)

	// build context
	systemMessage, err := s.systemMessageBuilder.BuildSystemMessage(
		ctx,
		p,
		toolServers,
		sess,
	)
	if err != nil {
		return err
	}

	agentContext := []agent.Message{}
	agentContext = append(agentContext, systemMessage)
	agentContext = append(agentContext, sess.Messages()...)

	if r.Logging {
		s.activityLogger.Commit(sess.ID(), []agent.Message{r.UserMessage})
	}

	// inject id's
	ctx = withProfileID(ctx, p.ID())
	ctx = withSessionID(ctx, r.SessionID)

	logger := s.logger.With(
		"profile", p.ID(),
		"session", sess.ID(),
	)

	h := NewChatEventHandler(p.ID(), sess, s.sessionRepo, s.activityLogger, r.Logging, logger)

	runAgentLoopWithHandlers(
		ctx,
		model,
		agentContext,
		extractTools(toolServers),
		[]EventHandler{h},
		s.hooks,
		r.Sink,
		logger,
	)

	return nil
}

func runAgentLoopWithHandlers(
	ctx context.Context,
	model agent.Model,
	agentContext []agent.Message,
	toolKit []agent.Tool,
	eventHandlers []EventHandler,
	hooks []any,
	sink chan runtime.Event,
	logger *slog.Logger,
) {
	// sink
	evCh := make(chan runtime.Event, runtimeEventBuffer)

	go func() {
		defer close(evCh)
		runtime.RunAgentLoop(
			ctx,
			model,
			agentContext,
			toolKit,
			evCh,
			hooks,
		)
	}()

	for rawEv := range evCh {
		sinkEvent(rawEv, sink)
		handleEventHandlerGroup(ctx, rawEv, eventHandlers, logger)
	}
}

func sinkEvent(ev runtime.Event, sink chan runtime.Event) {
	outEv := ev
	if ev, ok := outEv.(*runtime.LoopExitEvent); ok {
		if errors.Is(ev.Err(), context.Canceled) {
			outEv = runtime.NewLoopExitEvent(nil)
		}
	}
	sink <- outEv
}

func handleEventHandlerGroup(ctx context.Context, ev runtime.Event, eventHandlers []EventHandler, logger *slog.Logger) {
	var wg sync.WaitGroup
	for _, h := range eventHandlers {
		wg.Go(func() {
			if err := handleEvent(ctx, h, ev); err != nil {
				logger.ErrorContext(ctx, "handling runtime problems", "error", err)
			}
		})
	}
	wg.Wait()
}

func handleEvent(ctx context.Context, h EventHandler, rawEv runtime.Event) error {
	switch ev := rawEv.(type) {
	case *runtime.LoopExitEvent:
		return h.HandleLoopExitEvent(ctx, ev)
	case *runtime.ToolCallErrEvent:
		return h.HandleToolCallErrEvent(ctx, ev)
	case *runtime.ToolResultEvent:
		return h.HandleToolResultEvent(ctx, ev)
	case *runtime.CompletionMistakeEvent:
		return h.HandleCompletionMistakeEvent(ctx, ev)
	case *runtime.CompleteEvent:
		return h.HandleCompleteEvent(ctx, ev)
	case *runtime.CompactionEvent:
		return h.HandleCompactionEvent(ctx, ev)
	default:
		return fmt.Errorf("unknown event")
	}
}

func resolveToolServers(
	reg agent.ToolRegistry,
	p profile.Profile,
	provided []agent.ToolServer,
	logger *slog.Logger,
) ([]agent.ToolServer, error) {
	toolServers, err := reg.ToolServers(p.ToolServers()...)
	if err != nil {
		if err := types.DistillErrNotExist(fmt.Sprintf("agent %s", err), err, logger); err != nil {
			return nil, err
		}
	}

	if provided != nil {
		toolServers = append(toolServers, provided...)
	}

	return toolServers, nil
}

func extractTools(toolServers []agent.ToolServer) []agent.Tool {
	toolKit := []agent.Tool{}
	for _, ts := range toolServers {
		toolKit = append(toolKit, ts.Tools()...)
	}
	return toolKit
}

type (
	agentIDCTXKey   struct{}
	sessionIDCTXKey struct{}
)

func withProfileID(ctx context.Context, id profile.ID) context.Context {
	return context.WithValue(ctx, agentIDCTXKey{}, id)
}

func ProfileIDFromContext(ctx context.Context) (profile.ID, bool) {
	id, ok := ctx.Value(agentIDCTXKey{}).(profile.ID)
	return id, ok
}

func withSessionID(ctx context.Context, id session.ID) context.Context {
	return context.WithValue(ctx, sessionIDCTXKey{}, id)
}

func SessionIDFromContext(ctx context.Context) (session.ID, bool) {
	id, ok := ctx.Value(sessionIDCTXKey{}).(session.ID)
	return id, ok
}
