package gobash

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/mark3labs/go-bash/internal/ringbuf"
)

type executionScopeKey struct{}

type executionScope struct {
	cmdCount  atomic.Int64
	loopIters atomic.Int64
	globOps   atomic.Int64

	mu       sync.Mutex
	limitErr *ExecutionLimitError
	cancel   context.CancelFunc
	tracker  *ringbuf.Tracker
}

func newExecutionScope() *executionScope { return &executionScope{} }

func executionScopeFromContext(ctx context.Context) (*executionScope, bool) {
	s, ok := ctx.Value(executionScopeKey{}).(*executionScope)
	return s, ok && s != nil
}

func (s *executionScope) setCancel(cancel context.CancelFunc) {
	s.mu.Lock()
	if s.cancel == nil {
		s.cancel = cancel
	}
	s.mu.Unlock()
}

func (s *executionScope) trip(err *ExecutionLimitError, cancel context.CancelFunc) {
	s.mu.Lock()
	if s.limitErr == nil {
		s.limitErr = err
		s.cancel = cancel
	}
	c := s.cancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

func (s *executionScope) err() *ExecutionLimitError {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limitErr
}

func (s *executionScope) outputTracker(limit int64, onOver func(int64) error) *ringbuf.Tracker {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tracker == nil {
		s.tracker = ringbuf.NewTracker(limit, onOver)
	}
	return s.tracker
}
