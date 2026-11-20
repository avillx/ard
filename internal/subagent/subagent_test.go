package subagent

import (
	"ard/internal/agent"
	"ard/internal/chat"
	"ard/internal/runtime"
	"ard/internal/session"
	"context"
	"errors"
	"log/slog"
	"testing"
)

type stubExecutor struct {
	events []runtime.Event
}

func (e *stubExecutor) Chat(_ context.Context, r chat.Request) error {
	for _, ev := range e.events {
		r.Sink <- ev
	}

	return nil
}

type stubSessionsRepo struct{}

func (r *stubSessionsRepo) Session(session.ID) (session.Session, error) {
	return nil, nil
}

func (r *stubSessionsRepo) Save(_ session.Session) error {
	return nil
}

func (r *stubSessionsRepo) Delete(session.ID) error {
	return nil
}

func (r *stubSessionsRepo) Headers() ([]session.Header, error) {
	return nil, nil
}

type stubUUID struct {
	id string
}

func (u stubUUID) New() string {
	return u.id
}

func newTestService(t *testing.T, chatExec chat.Executor, repo session.SessionsRepo) *Service {
	t.Helper()

	sessService := session.NewService(
		repo,
		stubUUID{id: "session-1"},
		slog.New(slog.DiscardHandler),
	)

	return NewService(
		chatExec,
		sessService,
		slog.New(slog.DiscardHandler),
	)
}

func TestCall_HappyPath(t *testing.T) {
	repo := &stubSessionsRepo{}
	chatExec := &stubExecutor{
		events: []runtime.Event{
			runtime.NewCompleteEvent(&agent.Completion{Content: "done", Done: true}),
		},
	}

	svc := newTestService(t, chatExec, repo)

	got, err := svc.Call(context.Background(), agent.ID("sub-agent"), "please do")
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if got != "done" {
		t.Fatalf("Call() = %q, want %q", got, "done")
	}
}

func TestCall_CallStackOverflow(t *testing.T) {
	repo := &stubSessionsRepo{}
	svc := newTestService(t, &stubExecutor{}, repo)

	ctx := context.Background()
	for range maxSubAgentDepth {
		ctx, _ = subAgentCallStack(ctx, subAgentCall{subagent: agent.ID("nested")})
	}

	got, err := svc.Call(ctx, agent.ID("sub-agent"), "please do")
	if !errors.Is(err, ErrCallStackOverflow) {
		t.Fatalf("Call() error = %v, want ErrCallStackOverflow", err)
	}
	if got != "" {
		t.Fatalf("Call() = %q, want empty result", got)
	}
}
