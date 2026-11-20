package chat

import (
	"ard/internal/session"
	"context"
	"sync"
)

var _ Executor = (*Dispatcher)(nil)

type requestProcessing struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type Dispatcher struct {
	*Service

	processes map[session.ID]*requestProcessing
	mu        sync.Mutex
}

func NewDispatcher(
	s *Service,
) *Dispatcher {
	return &Dispatcher{
		Service:   s,
		processes: map[session.ID]*requestProcessing{},
	}
}

func (d *Dispatcher) Chat(ctx context.Context, r Request) error {
	ctx, cancel := context.WithCancel(ctx)
	p := &requestProcessing{
		cancel: cancel,
		done:   make(chan struct{}),
	}

	if prev := d.swap(r.SessionID, p); prev != nil {
		prev.cancel()
		<-prev.done
	}

	defer func() {
		cancel()
		close(p.done)
		d.remove(r.SessionID, p)
	}()

	return d.Service.Chat(ctx, r)
}

func (d *Dispatcher) Interrupt(sessID session.ID) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if prev := d.processes[sessID]; prev != nil {
		prev.cancel()
		<-prev.done
		d.logger.Info("agent interrupted", "session", sessID)
	}
}

func (d *Dispatcher) swap(sessID session.ID, p *requestProcessing) *requestProcessing {
	d.mu.Lock()
	defer d.mu.Unlock()

	prev := d.processes[sessID]
	d.processes[sessID] = p
	return prev
}

func (d *Dispatcher) remove(sessID session.ID, p *requestProcessing) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.processes[sessID] == p {
		delete(d.processes, sessID)
	}
}
