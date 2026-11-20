package memory

import (
	"ard/internal/agent"
	"ard/internal/prompt"
	"ard/internal/runtime"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

var ErrDisabledMemory = errors.New("agent memory is disabled")

const (
	// buffer of events emitted by a running consolidation loop.
	runtimeEventBuffer = 16
	// consolidation runs once a day.
	consolidationPeriod = 24 * time.Hour
)

type ConsolidatorConfig struct {
	Model       string `toml:"model"`
	Enabled     bool   `toml:"enabled"`
	Instruction string `toml:"instruction"`
}

func (c ConsolidatorConfig) IsZero() bool {
	return !c.Enabled &&
		c.Model == "" &&
		c.Instruction == ""
}

type ConsolidatorRepo interface {
	Load() (ConsolidatorConfig, error)
	Save(ConsolidatorConfig) error
}

type ConsolidationService struct {
	modelName string
	model     agent.Model
	modelRepo agent.ModelRegistry

	toolServer []agent.ToolServer
	memoryRepo ConsolidatorRepo

	resolveHooks func() []any
	enabled      bool
	instruction  string

	logger *slog.Logger

	// for lock on set model, enable/disable consolidation and instruction.
	// it is one mutex because no need more. so it any way rare used.
	mu sync.RWMutex
}

func NewConsolidationService(
	toolServer []agent.ToolServer,
	hooksReslover func() []any,
	modelRepo agent.ModelRegistry,
	memoryRepo ConsolidatorRepo,
	logger *slog.Logger,
) (*ConsolidationService, error) {
	if len(toolServer) == 0 {
		return nil, fmt.Errorf("no tools for managing memory")
	}

	svc := &ConsolidationService{
		memoryRepo:   memoryRepo,
		modelRepo:    modelRepo,
		toolServer:   toolServer,
		resolveHooks: hooksReslover,
		enabled:      false,
		logger:       logger.WithGroup("memory"),
	}

	if err := svc.load(); err != nil {
		svc.logger.Error("service is not loaded", "error", err)
	}

	return svc, nil
}

func (m *ConsolidationService) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.load(); err != nil {
		return err
	}

	m.logger.Info("reloaded")

	return nil
}

func (m *ConsolidationService) load() error {
	cfg, err := m.memoryRepo.Load()
	if err != nil {
		return err
	}

	if cfg.IsZero() {
		m.logger.Warn("config is not specified, memory consolidation disabled")
		return nil
	}

	model, err := m.modelRepo.Get(cfg.Model)
	if err != nil {
		return err
	}

	m.model = model
	m.modelName = cfg.Model
	m.instruction = cfg.Instruction
	m.enabled = cfg.Enabled

	return nil
}

func (m *ConsolidationService) ConsolidateImmidate(
	ctx context.Context,
	evCh chan runtime.Event,
) error {
	return m.runConsolidation(ctx, evCh)
}

func (m *ConsolidationService) runConsolidation(
	ctx context.Context,
	evCh chan runtime.Event,
) error {
	m.mu.RLock()
	model := m.model
	instruction := m.instruction
	m.mu.RUnlock()

	if model == nil {
		return fmt.Errorf("consolidation model is not set")
	}

	m.logger.InfoContext(ctx, "consolidation started")

	systemPrompt := prompt.Memorization()
	systemMessage := agent.NewSystemMessage(systemPrompt)

	memoRequest := prompt.MemorizationRequest(time.Now(), instruction)
	userMessage := agent.NewUserMessage(memoRequest)

	messages := []agent.Message{systemMessage, userMessage}

	tools := []agent.Tool{}
	for _, ts := range m.toolServer {
		tools = append(tools, ts.Tools()...)
	}

	runtime.RunAgentLoop(
		ctx,
		model,
		messages,
		tools,
		evCh,
		m.resolveHooks(),
	)

	return nil
}

func (m *ConsolidationService) Config() ConsolidatorConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ConsolidatorConfig{
		Instruction: m.instruction,
		Model:       m.modelName,
		Enabled:     m.enabled,
	}
}

func (m *ConsolidationService) SaveConfig(cfg ConsolidatorConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// validate model existence
	_, err := m.modelRepo.Get(cfg.Model)
	if err != nil {
		return err
	}

	return m.memoryRepo.Save(cfg)
}

func (m *ConsolidationService) consolidateInBackground(ctx context.Context) error {
	evCh := make(chan runtime.Event, runtimeEventBuffer)
	defer close(evCh)

	// for drop consolidation events
	go func() {
		for range evCh {
		}
	}()

	return m.runConsolidation(ctx, evCh)
}

func (m *ConsolidationService) Run(ctx context.Context) {
	for {
		ticker := time.Tick(time.Until(NextConsolidation(time.Now())))
		select {
		case <-ticker:
			m.logger.InfoContext(ctx, "automatic consolidation started")

			m.mu.RLock()
			enabled := m.enabled
			model := m.model
			m.mu.RUnlock()

			if !enabled {
				// unneccecary noise in logs better than silent disable consolidation
				m.logger.InfoContext(ctx, "skip memory consolidation, consolidation is disabled")
				continue
			}

			if model == nil {
				m.logger.WarnContext(ctx, "consolidation declined, model is not defined")
				continue
			}

			if err := m.consolidateInBackground(ctx); err != nil {
				m.logger.ErrorContext(ctx, "consolidation", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func NextConsolidation(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(consolidationPeriod)
	}
	return next
}
