package memory

import (
	"ard/internal/agent"
	"ard/internal/chat"
	"ard/internal/prompt"
	"ard/internal/session"
	"ard/internal/types"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

var _ chat.ActivityLogger = (*ActivityService)(nil)

const defaultFlushInterval = 120 // multiplies on seconds

// concurent safe messages buffer.
type messageBuffer struct {
	msgs []agent.Message
	mu   sync.Mutex
}

func newMessageBuffer() *messageBuffer {
	return &messageBuffer{
		msgs: []agent.Message{},
	}
}

func (b *messageBuffer) append(msgs []agent.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.msgs = append(b.msgs, msgs...)
}

func (b *messageBuffer) transcript() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	var gatheredMsgs strings.Builder
	for _, m := range b.msgs {
		gatheredMsgs.WriteString(m.String())
	}

	return fmt.Sprintf(
		"%s\n\n<transcript>%s</transcript>",
		prompt.ReportRequest(),
		gatheredMsgs.String(),
	)
}

type ActivityConfig struct {
	Enabled   bool   `toml:"enabled"  json:"enabled"`
	Interval  int64  `toml:"interval" json:"interval"`
	ModelName string `toml:"model"    json:"model"`
}

func (c ActivityConfig) IsZero() bool {
	return !c.Enabled &&
		c.Interval == 0 &&
		c.ModelName == ""
}

type ActivityConfigRepo interface {
	Save(ActivityConfig) error
	Load() (ActivityConfig, error)
}

type ActivityService struct {
	interval  int64
	modelName string
	enabled   bool
	model     agent.Model
	cfgMu     sync.RWMutex

	messageBuffers map[session.ID]*messageBuffer
	flushTimer     map[session.ID]*time.Timer
	mu             sync.RWMutex

	activityRepo agent.ActivityRepo
	cfgRepo      ActivityConfigRepo
	modelRepo    agent.ModelRegistry

	logger *slog.Logger
}

func NewActivityService(
	modelRepo agent.ModelRegistry,
	cfgRepo ActivityConfigRepo,
	activityRepo agent.ActivityRepo,
	logger *slog.Logger,
) *ActivityService {
	svc := &ActivityService{
		messageBuffers: map[session.ID]*messageBuffer{},
		flushTimer:     map[session.ID]*time.Timer{},
		logger:         logger.WithGroup("activity"),
		activityRepo:   activityRepo,
		interval:       defaultFlushInterval,
		cfgRepo:        cfgRepo,
		modelRepo:      modelRepo,
	}

	if err := svc.Reload(); err != nil {
		svc.logger.Error("service is not loaded", "error", err)
	}

	return svc
}

func (s *ActivityService) Commit(sessID session.ID, msgs []agent.Message) {
	if !s.Config().Enabled {
		// if sevice disabled then drop commits silently
		return
	}

	buf := s.getBuffer(sessID)
	buf.append(msgs)
}

func (s *ActivityService) releaseBuffer(sessID session.ID) (*messageBuffer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	buf, ok := s.messageBuffers[sessID]
	if !ok {
		return nil, types.ErrIsNotExist
	}

	delete(s.flushTimer, sessID)
	delete(s.messageBuffers, sessID)

	return buf, nil
}

func (s *ActivityService) getBuffer(sessID session.ID) *messageBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()

	buf, ok := s.messageBuffers[sessID]
	if !ok {
		buf = newMessageBuffer()
		interval := time.Duration(s.Config().Interval) * time.Second
		timer := time.AfterFunc(interval, func() {
			logger := s.logger.With("session", sessID)

			if !s.Config().Enabled {
				logger.Warn("not logged, service disabled")
				return
			}

			if err := s.flush(context.Background(), sessID); err != nil {
				logger.Error("not logged", "error", err)
				return
			}

			logger.Info("logged")
		})
		s.flushTimer[sessID] = timer
		s.messageBuffers[sessID] = buf
	}

	return buf
}

func (s *ActivityService) flush(ctx context.Context, sessID session.ID) error {
	buf, err := s.releaseBuffer(sessID)
	if err != nil {
		return err
	}

	extract, err := s.extractActivity(ctx, buf)
	if err != nil {
		return err
	}

	return s.activityRepo.Log(
		agent.ActivityRecord{
			Content: extract,
			Stamp:   time.Now(),
		},
	)
}

func (s *ActivityService) extractActivity(ctx context.Context, buf *messageBuffer) (string, error) {
	s.cfgMu.RLock()
	model := s.model
	s.cfgMu.RUnlock()

	if model == nil {
		return "", fmt.Errorf("model is not set")
	}

	msgs := []agent.Message{
		agent.NewSystemMessage(prompt.ReportSystem()),
		agent.NewUserMessage(buf.transcript()),
	}

	// TODO: replace model.Complete on a RunAgenLoop to eliminate buffer overflow case
	completion, err := model.Complete(ctx, nil, msgs)
	if err != nil {
		return "", err
	}

	return completion.Content, nil
}

func (s *ActivityService) Reload() error {
	cfg, err := s.cfgRepo.Load()
	if err != nil {
		return err
	}

	if err := s.applyConfig(cfg); err != nil {
		return err
	}

	s.logger.Info("reloaded")

	return nil
}

func (s *ActivityService) applyConfig(cfg ActivityConfig) error {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()

	if cfg.IsZero() {
		s.logger.Warn("config is not specified, activity logging is disabled")
		return nil
	}

	model, err := s.modelRepo.Get(cfg.ModelName)
	if err != nil {
		return err
	}
	s.model = model
	s.modelName = cfg.ModelName
	s.interval = cfg.Interval
	s.enabled = cfg.Enabled

	return nil
}

func (s *ActivityService) SaveConfig(cfg ActivityConfig) error {
	_, err := s.modelRepo.Get(s.modelName)
	if err != nil {
		return err
	}

	return s.cfgRepo.Save(cfg)
}

func (s *ActivityService) Config() ActivityConfig {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()

	return ActivityConfig{
		Interval:  s.interval,
		ModelName: s.modelName,
		Enabled:   s.enabled,
	}
}
