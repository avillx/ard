package chat

import (
	"ard/internal/agent"
	"ard/internal/profile"
	"ard/internal/runtime"
	"ard/internal/session"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

type Chat struct {
	SystemMessage agent.Message
	Sess          session.Session
	Profile       profile.Profile
	Tools         []agent.Tool
	Model         agent.Model
}

func (c *Chat) buildContext(um *agent.UserMessage) []agent.Message {
	c.Sess.AddMessages(um)

	agentContext := []agent.Message{}
	agentContext = append(agentContext, c.SystemMessage)
	agentContext = append(agentContext, c.Sess.Messages()...)

	return agentContext
}

func (c *Chat) Run(
	ctx context.Context,
	um *agent.UserMessage,
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
			c.Model,
			c.buildContext(um),
			c.Tools,
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
