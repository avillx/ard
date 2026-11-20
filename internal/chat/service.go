package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"ard/internal/agent"
	"ard/internal/runtime"
	"ard/internal/session"
	"ard/internal/types"
)

type ActivityLogger interface {
	Commit(agent.ID, session.ID, []agent.Message)
}

type Request struct {
	AgentID             agent.ID
	SessionID           session.ID
	UserMessage         *agent.UserMessage
	ProvidedToolServers []agent.ToolServer
	Logging             bool
	Sink                chan runtime.Event
}

func (r Request) validate() error {
	if r.AgentID == "" {
		return fmt.Errorf("completion request must include agentID")
	}

	if r.SessionID == "" {
		return fmt.Errorf("completion request must include sessionID")
	}

	if r.UserMessage == nil {
		return fmt.Errorf("completion request must include user message")
	}

	if len(r.UserMessage.Content()) <= 0 {
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

type Executor interface {
	Chat(ctx context.Context, r Request) error
}

type SystemMessageBuilder interface {
	BuildSystemMessage(
		ctx context.Context,
		agt agent.Agent,
		toolServers []agent.ToolServer,
		sess session.Session,
	) (*agent.SystemMessage, error)
}

type Service struct {
	agentRepo            agent.Repo
	sessionRepo          session.SessionsRepo
	modelRepo            agent.ModelRegistry
	toolRegistry         agent.ToolRegistry
	systemMessageBuilder SystemMessageBuilder
	activityLogger       ActivityLogger
	hooks                []any
	logger               *slog.Logger
}

func NewService(
	agentRepo agent.Repo,
	sessionRepo session.SessionsRepo,
	modelRepo agent.ModelRegistry,
	toolRegistry agent.ToolRegistry,
	systemMessageBuilder SystemMessageBuilder,
	activityLogger ActivityLogger,
	hooks []any,
	logger *slog.Logger,
) *Service {
	return &Service{
		agentRepo:            agentRepo,
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

	// agent
	agt, err := s.agentRepo.Get(r.AgentID)
	if err != nil {
		return err
	}

	// model
	model, err := s.modelRepo.Get(agt.Model())
	if err != nil {
		return err
	}

	// tools
	toolServers, err := resolveToolServers(s.toolRegistry, agt, r.ProvidedToolServers)
	if err != nil {
		return err
	}

	// session
	sess, err := s.sessionRepo.Session(agt.ID(), r.SessionID)
	if err != nil {
		return err
	}

	sess.AddMessages(r.UserMessage)

	// build context
	systemMessage, err := s.systemMessageBuilder.BuildSystemMessage(
		ctx,
		agt,
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
		s.activityLogger.Commit(agt.ID(), sess.ID(), []agent.Message{r.UserMessage})
	}

	// inject id's
	ctx = withAgentID(ctx, r.AgentID)
	ctx = withSessionID(ctx, r.SessionID)

	logger := s.logger.With(
		"agent", r.AgentID,
		"session", sess.ID(),
	)

	h := NewChatEventHandler(r.AgentID, sess, s.sessionRepo, s.activityLogger, r.Logging, logger)

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
	evCh := make(chan runtime.Event, 16)

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
	agt agent.Agent,
	provided []agent.ToolServer,
) ([]agent.ToolServer, error) {
	toolServers, err := reg.ToolServers(agt.ToolServers()...)
	if err != nil {
		if err := types.DistillErrNotExist(fmt.Sprintf("agent %s", err), err); err != nil {
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

func withAgentID(ctx context.Context, id agent.ID) context.Context {
	return context.WithValue(ctx, agentIDCTXKey{}, id)
}

func AgentIDFromContext(ctx context.Context) (agent.ID, bool) {
	id, ok := ctx.Value(agentIDCTXKey{}).(agent.ID)
	return id, ok
}

func withSessionID(ctx context.Context, id session.ID) context.Context {
	return context.WithValue(ctx, sessionIDCTXKey{}, id)
}

func SessionIDFromContext(ctx context.Context) (session.ID, bool) {
	id, ok := ctx.Value(sessionIDCTXKey{}).(session.ID)
	return id, ok
}
