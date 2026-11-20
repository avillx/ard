package chat

import (
	"ard/internal/agent"
	"ard/internal/profile"
	"ard/internal/runtime"
	"ard/internal/session"
	"ard/internal/types"
	"context"
	"fmt"
	"log/slog"
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
	cache                *Cache
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
		cache:                NewCache(DefaultCacheLifeTime),
		logger:               logger.WithGroup("chat"),
	}
}

func (s *Service) buildChat(
	ctx context.Context,
	r Request,
) (*Chat, error) {
	// get cached
	if c, ok := s.cache.Get(r.SessionID); ok {
		return c, nil
	}

	// validate request
	if err := r.validate(); err != nil {
		return nil, err
	}

	// session
	sess, err := s.sessionRepo.Session(r.SessionID)
	if err != nil {
		return nil, err
	}

	// profile
	p, err := s.profileSvc.Get(sess.ProfileID())
	if err != nil {
		return nil, err
	}

	// model
	model, err := s.modelRepo.Get(p.Model())
	if err != nil {
		return nil, err
	}

	// tools
	toolServers, err := resolveToolServers(s.toolRegistry, p, r.ProvidedToolServers, s.logger)
	if err != nil {
		return nil, err
	}

	// build context
	systemMessage, err := s.systemMessageBuilder.BuildSystemMessage(
		ctx,
		p,
		toolServers,
		sess,
	)
	if err != nil {
		return nil, err
	}

	c, err := NewChat(systemMessage, sess, p, extractTools(toolServers), model)
	if err != nil {
		return nil, err
	}

	// put cache
	s.cache.Put(r.SessionID, c)

	return c, nil
}

func (s *Service) Chat(
	ctx context.Context,
	r Request,
) error {
	c, err := s.buildChat(ctx, r)
	if err != nil {
		return err
	}

	if r.Logging {
		s.activityLogger.Commit(c.Session().ID(), []agent.Message{r.UserMessage})
	}

	// inject id's
	ctx = withProfileID(ctx, c.Profile().ID())
	ctx = withSessionID(ctx, r.SessionID)

	logger := s.logger.With(
		"profile", c.Profile().ID(),
		"session", c.Session().ID(),
	)

	h := NewChatEventHandler(c.Profile().ID(), c.Session(), s.sessionRepo, s.activityLogger, r.Logging, logger)

	c.Run(
		ctx,
		r.UserMessage,
		[]EventHandler{h},
		s.hooks,
		r.Sink,
		logger,
	)

	return nil
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
