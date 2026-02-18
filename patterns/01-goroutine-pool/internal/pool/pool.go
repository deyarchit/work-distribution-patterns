package pool

import (
	"errors"
	"sync"
)

// ErrQueueFull is returned by Enqueue when the work queue is at capacity.
var ErrQueueFull = errors.New("task queue is full")

// Pool is a bounded goroutine worker pool.
type Pool struct {
	work chan func()
	wg   sync.WaitGroup
}

// New creates a Pool with the given number of workers and queue capacity.
func New(workers, queueSize int) *Pool {
	p := &Pool{
		work: make(chan func(), queueSize),
	}
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for fn := range p.work {
				fn()
			}
		}()
	}
	return p
}

// Enqueue adds a work item to the pool. Returns ErrQueueFull if the queue is
// at capacity (non-blocking).
func (p *Pool) Enqueue(fn func()) error {
	select {
	case p.work <- fn:
		return nil
	default:
		return ErrQueueFull
	}
}

// Stop closes the pool and waits for all workers to finish.
func (p *Pool) Stop() {
	close(p.work)
	p.wg.Wait()
}
