package queue

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

const (
	StatusPending      = "pending"
	StatusRunning      = "running"
	StatusCompleted    = "completed"
	StatusDeadLettered = "dead-lettered"
)

type Config struct {
	Workers           int
	ShutdownTimeout   time.Duration
	DefaultMaxRetries int
	DefaultTimeout    time.Duration
}

type taskType struct {
	name              string
	handler           interfaces.TaskHandler
	defaultMaxRetries int
	defaultTimeout    time.Duration
}

type taskEntry struct {
	id          string
	name        string
	payload     json.RawMessage
	status      string
	priority    int
	retryCount  int
	maxRetries  int
	timeout     time.Duration
	createdAt   time.Time
	startedAt   *time.Time
	completedAt *time.Time
	err         string
	delay       time.Duration
	index       int // heap index
}

type deadLetterEntry struct {
	TaskID          string
	Name            string
	OriginalPayload json.RawMessage
	LastError       string
	RetryCount      int
	CreatedAt       time.Time
	DeadLetteredAt  time.Time
}

type priorityQueue []*taskEntry

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	if pq[i].priority != pq[j].priority {
		return pq[i].priority > pq[j].priority
	}
	return pq[i].createdAt.Before(pq[j].createdAt)
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*taskEntry)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[:n-1]
	return item
}

type Service struct {
	config Config
	logger *slog.Logger

	mu        sync.RWMutex
	taskTypes map[string]*taskType
	tasks     map[string]*taskEntry

	pqMu   sync.Mutex
	pq     priorityQueue
	notify chan struct{}

	deadMu      sync.Mutex
	deadLetters []deadLetterEntry

	shuttingDown atomic.Bool
	wg           sync.WaitGroup
	stopCh       chan struct{}
}

func NewService(cfg Config, logger *slog.Logger) *Service {
	pq := make(priorityQueue, 0)
	heap.Init(&pq)
	return &Service{
		config:    cfg,
		logger:    logger,
		taskTypes: make(map[string]*taskType),
		tasks:     make(map[string]*taskEntry),
		pq:        pq,
		notify:    make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
	}
}

func (s *Service) Start() {
	for i := 0; i < s.config.Workers; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
	s.logger.Info("queue workers started", "count", s.config.Workers)
}

func (s *Service) RegisterTask(_ context.Context, name string, handler interfaces.TaskHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.taskTypes[name]; exists {
		return fmt.Errorf("task type %q already registered", name)
	}

	s.taskTypes[name] = &taskType{
		name:              name,
		handler:           handler,
		defaultMaxRetries: s.config.DefaultMaxRetries,
		defaultTimeout:    s.config.DefaultTimeout,
	}

	s.logger.Info("task type registered", "name", name)
	return nil
}

func (s *Service) Enqueue(_ context.Context, name string, payload interface{}, options *interfaces.TaskOptions) (string, error) {
	if s.shuttingDown.Load() {
		return "", fmt.Errorf("queue is shutting down")
	}

	s.mu.RLock()
	tt, exists := s.taskTypes[name]
	s.mu.RUnlock()
	if !exists {
		return "", fmt.Errorf("task type %q is not registered", name)
	}

	var rawPayload json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("serialize payload: %w", err)
		}
		rawPayload = data
	}

	taskID := uuid.Must(uuid.NewV7()).String()

	maxRetries := tt.defaultMaxRetries
	timeout := tt.defaultTimeout
	var delay time.Duration
	priority := 0

	if options != nil {
		delay = options.Delay
		priority = options.Priority
		if options.MaxRetries > 0 {
			maxRetries = options.MaxRetries
		}
		if options.Timeout > 0 {
			timeout = options.Timeout
		}
	}

	task := &taskEntry{
		id:         taskID,
		name:       name,
		payload:    rawPayload,
		status:     StatusPending,
		priority:   priority,
		retryCount: 0,
		maxRetries: maxRetries,
		timeout:    timeout,
		createdAt:  time.Now(),
		delay:      delay,
	}

	s.mu.Lock()
	s.tasks[taskID] = task
	s.mu.Unlock()

	if delay > 0 {
		time.AfterFunc(delay, func() {
			if s.shuttingDown.Load() {
				return
			}
			s.pushTask(task)
		})
	} else {
		s.pushTask(task)
	}

	s.logger.Debug("task enqueued",
		"task_id", taskID,
		"name", name,
		"priority", priority,
		"delay", delay,
	)

	return taskID, nil
}

func (s *Service) GetStatus(_ context.Context, taskID string) (*interfaces.TaskStatus, error) {
	s.mu.RLock()
	task, exists := s.tasks[taskID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("task %q not found", taskID)
	}

	return &interfaces.TaskStatus{
		ID:          task.id,
		Name:        task.name,
		Status:      task.status,
		CreatedAt:   task.createdAt,
		StartedAt:   task.startedAt,
		CompletedAt: task.completedAt,
		Error:       task.err,
		RetryCount:  task.retryCount,
	}, nil
}

func (s *Service) Close(ctx context.Context) error {
	s.shuttingDown.Store(true)
	close(s.stopCh)

	s.signalWorkers()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("all workers stopped gracefully")
		return nil
	case <-time.After(s.config.ShutdownTimeout):
		s.logger.Warn("shutdown timeout exceeded, workers may be cancelled")
		return fmt.Errorf("shutdown timeout exceeded after %s", s.config.ShutdownTimeout)
	case <-ctx.Done():
		s.logger.Warn("context cancelled during shutdown")
		return ctx.Err()
	}
}

func (s *Service) DeadLetters() []deadLetterEntry {
	s.deadMu.Lock()
	defer s.deadMu.Unlock()
	result := make([]deadLetterEntry, len(s.deadLetters))
	copy(result, s.deadLetters)
	return result
}

func (s *Service) WorkerCount() int {
	return s.config.Workers
}

func (s *Service) pushTask(task *taskEntry) {
	s.pqMu.Lock()
	heap.Push(&s.pq, task)
	s.pqMu.Unlock()

	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *Service) popTask() *taskEntry {
	s.pqMu.Lock()
	defer s.pqMu.Unlock()

	if s.pq.Len() == 0 {
		return nil
	}
	return heap.Pop(&s.pq).(*taskEntry)
}

func (s *Service) signalWorkers() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *Service) worker(id int) {
	defer s.wg.Done()

	s.logger.Debug("worker started", "worker_id", id)

	for {
		task := s.popTask()
		if task != nil {
			s.executeTask(task)
			continue
		}

		select {
		case <-s.notify:
			continue
		case <-s.stopCh:
			// drain remaining tasks
			for {
				t := s.popTask()
				if t == nil {
					return
				}
				s.executeTask(t)
			}
		}
	}
}

func (s *Service) executeTask(task *taskEntry) {
	s.mu.Lock()
	task.status = StatusRunning
	now := time.Now()
	task.startedAt = &now
	s.mu.Unlock()

	timeout := task.timeout
	if timeout <= 0 {
		timeout = s.config.DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	s.mu.RLock()
	tt := s.taskTypes[task.name]
	s.mu.RUnlock()

	if tt == nil {
		s.mu.Lock()
		task.status = StatusDeadLettered
		task.err = fmt.Sprintf("task type %q not found", task.name)
		now := time.Now()
		task.completedAt = &now
		s.mu.Unlock()
		return
	}

	var payload interface{}
	if task.payload != nil {
		payload = task.payload
	}

	handlerErr := tt.handler(ctx, payload)

	s.mu.Lock()
	defer s.mu.Unlock()

	if handlerErr == nil {
		task.status = StatusCompleted
		now := time.Now()
		task.completedAt = &now
		s.logger.Debug("task completed",
			"task_id", task.id,
			"name", task.name,
		)
		return
	}

	task.retryCount++
	task.err = handlerErr.Error()

	if task.retryCount < task.maxRetries {
		task.status = StatusPending
		backoff := time.Duration(1) * time.Second * time.Duration(1<<uint(task.retryCount-1))

		s.logger.Debug("task failed, re-enqueueing",
			"task_id", task.id,
			"name", task.name,
			"retry_count", task.retryCount,
			"backoff", backoff,
			"error", handlerErr,
		)

		time.AfterFunc(backoff, func() {
			if s.shuttingDown.Load() {
				return
			}
			s.pushTask(task)
		})
	} else {
		task.status = StatusDeadLettered
		now := time.Now()
		task.completedAt = &now

		s.deadMu.Lock()
		s.deadLetters = append(s.deadLetters, deadLetterEntry{
			TaskID:          task.id,
			Name:            task.name,
			OriginalPayload: task.payload,
			LastError:       handlerErr.Error(),
			RetryCount:      task.retryCount,
			CreatedAt:       task.createdAt,
			DeadLetteredAt:  now,
		})
		s.deadMu.Unlock()

		s.logger.Warn("task dead-lettered",
			"task_id", task.id,
			"name", task.name,
			"retry_count", task.retryCount,
			"error", handlerErr,
		)
	}
}
