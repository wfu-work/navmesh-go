package services

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
)

type NotificationTask func(context.Context)

type NotificationRunner struct {
	mu       sync.Mutex
	queue    chan NotificationTask
	started  bool
	stopping bool
	wg       sync.WaitGroup
}

type NotificationRunnerStats struct {
	Started     bool `json:"started"`
	Stopping    bool `json:"stopping"`
	QueueLength int  `json:"queueLength"`
	QueueCap    int  `json:"queueCap"`
}

func (r *NotificationRunner) Stats() NotificationRunnerStats {
	if r == nil {
		return NotificationRunnerStats{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return NotificationRunnerStats{
		Started:     r.started,
		Stopping:    r.stopping,
		QueueLength: len(r.queue),
		QueueCap:    cap(r.queue),
	}
}

func NewNotificationRunner() *NotificationRunner {
	return &NotificationRunner{}
}

func (r *NotificationRunner) Start(workers, queueSize int) {
	if workers <= 0 {
		workers = 4
	}
	if queueSize <= 0 {
		queueSize = 1024
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.stopping {
		return
	}
	r.queue = make(chan NotificationTask, queueSize)
	r.started = true
	for range workers {
		r.wg.Add(1)
		go r.worker(r.queue)
	}
}

func (r *NotificationRunner) Submit(task NotificationTask) bool {
	if task == nil {
		return false
	}
	r.mu.Lock()
	if r.stopping {
		r.mu.Unlock()
		return false
	}
	if !r.started {
		r.mu.Unlock()
		r.runTask(task)
		return true
	}
	queue := r.queue
	select {
	case queue <- task:
		r.mu.Unlock()
		return true
	default:
		r.mu.Unlock()
		return false
	}
}

func (r *NotificationRunner) Stop(ctx context.Context) error {
	r.mu.Lock()
	if !r.started {
		r.stopping = true
		r.mu.Unlock()
		return nil
	}
	r.started = false
	r.stopping = true
	close(r.queue)
	r.mu.Unlock()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *NotificationRunner) worker(queue <-chan NotificationTask) {
	defer r.wg.Done()
	for task := range queue {
		r.runTask(task)
	}
}

func (r *NotificationRunner) runTask(task NotificationTask) {
	defer func() {
		if recovered := recover(); recovered != nil && global.NAV_LOG != nil {
			global.NAV_LOG.Error("notification task panicked", zap.Any("panic", recovered), zap.ByteString("stack", debug.Stack()))
		}
	}()
	task(context.Background())
}

var DefaultNotificationRunner = NewNotificationRunner()
