package queue

import (
	"container/heap"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/sdk/interfaces"
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
	index       int
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
	old[n-1] = nil
	item.index = -1
	*pq = old[:n-1]
	return item
}

type Service struct {
	config Config
	logger *slog.Logger
	db     interfaces.DatabaseService

	mu        sync.RWMutex
	taskTypes map[string]*taskType
	tasks     map[string]*taskEntry

	pqMu   sync.Mutex
	pq     priorityQueue
	notify chan struct{}

	deadMu      sync.Mutex
	deadLetters []*interfaces.DeadLetterEntry

	shuttingDown atomic.Bool
	wg           sync.WaitGroup
	stopCh       chan struct{}
}

func NewService(cfg Config, logger *slog.Logger, db interfaces.DatabaseService) *Service {
	pq := make(priorityQueue, 0)
	heap.Init(&pq)
	return &Service{
		config:      cfg,
		logger:      logger,
		db:          db,
		taskTypes:   make(map[string]*taskType),
		tasks:       make(map[string]*taskEntry),
		pq:          pq,
		notify:      make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
		deadLetters: make([]*interfaces.DeadLetterEntry, 0),
	}
}

func (s *Service) InitDB(ctx context.Context) error {
	if s.db == nil {
		return nil
	}

	_, err := s.db.Exec(ctx, `CREATE TABLE IF NOT EXISTS queue_tasks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		payload TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		priority INTEGER NOT NULL DEFAULT 0,
		retry_count INTEGER NOT NULL DEFAULT 0,
		max_retries INTEGER NOT NULL DEFAULT 3,
		timeout_ms INTEGER NOT NULL DEFAULT 60000,
		created_at DATETIME NOT NULL,
		started_at DATETIME,
		completed_at DATETIME,
		error TEXT,
		delay_ms INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return fmt.Errorf("create queue_tasks table: %w", err)
	}

	_, err = s.db.Exec(ctx, `CREATE TABLE IF NOT EXISTS queue_dead_letters (
		task_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		original_payload TEXT,
		last_error TEXT,
		retry_count INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		dead_lettered_at DATETIME NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create queue_dead_letters table: %w", err)
	}

	return s.loadPersistedTasks(ctx)
}

func (s *Service) loadPersistedTasks(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT id, name, payload, priority, retry_count, max_retries, timeout_ms, created_at, delay_ms FROM queue_tasks WHERE status = 'pending'`)
	if err != nil {
		return fmt.Errorf("load pending tasks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		var payloadStr sql.NullString
		var priority, retryCount, maxRetries, timeoutMs, delayMs int
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &payloadStr, &priority, &retryCount, &maxRetries, &timeoutMs, &createdAt, &delayMs); err != nil {
			return fmt.Errorf("scan pending task: %w", err)
		}

		var payload json.RawMessage
		if payloadStr.Valid {
			payload = json.RawMessage(payloadStr.String)
		}

		task := &taskEntry{
			id:         id,
			name:       name,
			payload:    payload,
			status:     interfaces.TaskStatusPending,
			priority:   priority,
			retryCount: retryCount,
			maxRetries: maxRetries,
			timeout:    time.Duration(timeoutMs) * time.Millisecond,
			createdAt:  createdAt,
			delay:      time.Duration(delayMs) * time.Millisecond,
		}
		s.tasks[id] = task
		s.pushTask(task)
	}

	rows2, err := s.db.Query(ctx, `SELECT task_id, name, original_payload, last_error, retry_count, created_at, dead_lettered_at FROM queue_dead_letters`)
	if err != nil {
		return fmt.Errorf("load dead letters: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var taskID, name string
		var payloadStr, lastErr sql.NullString
		var retryCount int
		var createdAt, deadLetteredAt time.Time

		if err := rows2.Scan(&taskID, &name, &payloadStr, &lastErr, &retryCount, &createdAt, &deadLetteredAt); err != nil {
			return fmt.Errorf("scan dead letter: %w", err)
		}

		var payload json.RawMessage
		if payloadStr.Valid {
			payload = json.RawMessage(payloadStr.String)
		}

		entry := &interfaces.DeadLetterEntry{
			TaskID:          taskID,
			Name:            name,
			OriginalPayload: payload,
			LastError:       lastErr.String,
			RetryCount:      retryCount,
			CreatedAt:       createdAt,
			DeadLetteredAt:  deadLetteredAt,
		}
		s.deadLetters = append(s.deadLetters, entry)
	}

	return nil
}

func (s *Service) persistTask(ctx context.Context, task *taskEntry) {
	if s.db == nil {
		return
	}
	timeoutMs := task.timeout.Milliseconds()
	delayMs := task.delay.Milliseconds()
	payloadStr := string(task.payload)

	_, err := s.db.Exec(ctx,
		`INSERT OR REPLACE INTO queue_tasks (id, name, payload, status, priority, retry_count, max_retries, timeout_ms, created_at, delay_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.id, task.name, payloadStr, task.status, task.priority, task.retryCount, task.maxRetries, timeoutMs, task.createdAt, delayMs)
	if err != nil {
		s.logger.Error("persist task failed", "task_id", task.id, "error", err)
	}
}

func (s *Service) updateTaskStatus(ctx context.Context, task *taskEntry) {
	if s.db == nil {
		return
	}
	var startedAt, completedAt interface{}
	if task.startedAt != nil {
		startedAt = *task.startedAt
	}
	if task.completedAt != nil {
		completedAt = *task.completedAt
	}
	_, err := s.db.Exec(ctx,
		`UPDATE queue_tasks SET status=?, retry_count=?, error=?, started_at=?, completed_at=? WHERE id=?`,
		task.status, task.retryCount, task.err, startedAt, completedAt, task.id)
	if err != nil {
		s.logger.Error("update task status failed", "task_id", task.id, "error", err)
	}
}

func (s *Service) persistDeadLetter(ctx context.Context, entry *interfaces.DeadLetterEntry) {
	if s.db == nil {
		return
	}
	_, err := s.db.Exec(ctx,
		`INSERT OR REPLACE INTO queue_dead_letters (task_id, name, original_payload, last_error, retry_count, created_at, dead_lettered_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.TaskID, entry.Name, string(entry.OriginalPayload), entry.LastError, entry.RetryCount, entry.CreatedAt, entry.DeadLetteredAt)
	if err != nil {
		s.logger.Error("persist dead letter failed", "task_id", entry.TaskID, "error", err)
	}
}

func (s *Service) removeDeadLetterFromDB(ctx context.Context, taskID string) {
	if s.db == nil {
		return
	}
	_, err := s.db.Exec(ctx, `DELETE FROM queue_dead_letters WHERE task_id = ?`, taskID)
	if err != nil {
		s.logger.Error("remove dead letter from db failed", "task_id", taskID, "error", err)
	}
}

func (s *Service) deleteTaskFromDB(ctx context.Context, taskID string) {
	if s.db == nil {
		return
	}
	_, err := s.db.Exec(ctx, `DELETE FROM queue_tasks WHERE id = ?`, taskID)
	if err != nil {
		s.logger.Error("delete task from db failed", "task_id", taskID, "error", err)
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

func (s *Service) Enqueue(ctx context.Context, name string, payload interface{}, options *interfaces.TaskOptions) (string, error) {
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
	priority := interfaces.PriorityNormal

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

	if priority < interfaces.PriorityLow || priority > interfaces.PriorityHigh {
		return "", fmt.Errorf("invalid priority %d: must be %d (low), %d (normal), or %d (high)",
			priority, interfaces.PriorityLow, interfaces.PriorityNormal, interfaces.PriorityHigh)
	}

	task := &taskEntry{
		id:         taskID,
		name:       name,
		payload:    rawPayload,
		status:     interfaces.TaskStatusPending,
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

	s.persistTask(ctx, task)

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

func (s *Service) ListDeadLetters(_ context.Context, page, pageSize int) ([]*interfaces.DeadLetterEntry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	s.deadMu.Lock()
	defer s.deadMu.Unlock()

	total := len(s.deadLetters)
	start := (page - 1) * pageSize
	if start >= total {
		return []*interfaces.DeadLetterEntry{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	result := make([]*interfaces.DeadLetterEntry, end-start)
	copy(result, s.deadLetters[start:end])
	return result, total, nil
}

func (s *Service) RetryDeadLetter(ctx context.Context, taskID string) error {
	s.deadMu.Lock()
	var entry *interfaces.DeadLetterEntry
	var idx int
	for i, dl := range s.deadLetters {
		if dl.TaskID == taskID {
			entry = dl
			idx = i
			break
		}
	}
	if entry == nil {
		s.deadMu.Unlock()
		return fmt.Errorf("dead letter task %q not found", taskID)
	}
	s.deadLetters = append(s.deadLetters[:idx], s.deadLetters[idx+1:]...)
	s.deadMu.Unlock()

	s.removeDeadLetterFromDB(ctx, taskID)

	s.mu.Lock()
	task := &taskEntry{
		id:         entry.TaskID,
		name:       entry.Name,
		payload:    entry.OriginalPayload,
		status:     interfaces.TaskStatusPending,
		priority:   interfaces.PriorityNormal,
		retryCount: 0,
		maxRetries: s.config.DefaultMaxRetries,
		timeout:    s.config.DefaultTimeout,
		createdAt:  time.Now(),
	}
	s.tasks[task.id] = task
	s.mu.Unlock()

	s.persistTask(ctx, task)
	s.pushTask(task)

	s.logger.Info("dead letter task retried", "task_id", taskID)
	return nil
}

func (s *Service) DeleteDeadLetter(ctx context.Context, taskID string) error {
	s.deadMu.Lock()
	var idx int
	found := false
	for i, dl := range s.deadLetters {
		if dl.TaskID == taskID {
			idx = i
			found = true
			break
		}
	}
	if !found {
		s.deadMu.Unlock()
		return fmt.Errorf("dead letter task %q not found", taskID)
	}
	s.deadLetters = append(s.deadLetters[:idx], s.deadLetters[idx+1:]...)
	s.deadMu.Unlock()

	s.removeDeadLetterFromDB(ctx, taskID)

	s.mu.Lock()
	delete(s.tasks, taskID)
	s.mu.Unlock()

	s.deleteTaskFromDB(ctx, taskID)

	s.logger.Info("dead letter task deleted", "task_id", taskID)
	return nil
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
			return
		}
	}
}

func (s *Service) executeTask(task *taskEntry) {
	s.mu.Lock()
	task.status = interfaces.TaskStatusRunning
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
		task.status = interfaces.TaskStatusDeadLettered
		task.err = fmt.Sprintf("task type %q not found", task.name)
		now := time.Now()
		task.completedAt = &now
		s.mu.Unlock()

		s.updateTaskStatus(ctx, task)

		s.deadMu.Lock()
		entry := &interfaces.DeadLetterEntry{
			TaskID:          task.id,
			Name:            task.name,
			OriginalPayload: task.payload,
			LastError:       task.err,
			RetryCount:      task.retryCount,
			CreatedAt:       task.createdAt,
			DeadLetteredAt:  *task.completedAt,
		}
		s.deadLetters = append(s.deadLetters, entry)
		s.deadMu.Unlock()

		s.persistDeadLetter(ctx, entry)
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
		task.status = interfaces.TaskStatusCompleted
		now := time.Now()
		task.completedAt = &now
		s.logger.Debug("task completed",
			"task_id", task.id,
			"name", task.name,
		)
		s.updateTaskStatus(ctx, task)
		s.deleteTaskFromDB(ctx, task.id)
		return
	}

	task.retryCount++
	task.err = handlerErr.Error()
	task.status = interfaces.TaskStatusFailed
	s.updateTaskStatus(ctx, task)

	if task.retryCount < task.maxRetries {
		task.status = interfaces.TaskStatusPending
		backoff := time.Second * time.Duration(1<<uint(task.retryCount-1))

		s.logger.Debug("task failed, re-enqueueing",
			"task_id", task.id,
			"name", task.name,
			"retry_count", task.retryCount,
			"backoff", backoff,
			"error", handlerErr,
		)

		s.updateTaskStatus(ctx, task)

		time.AfterFunc(backoff, func() {
			if s.shuttingDown.Load() {
				return
			}
			s.pushTask(task)
		})
	} else {
		task.status = interfaces.TaskStatusDeadLettered
		now := time.Now()
		task.completedAt = &now

		s.deadMu.Lock()
		entry := &interfaces.DeadLetterEntry{
			TaskID:          task.id,
			Name:            task.name,
			OriginalPayload: task.payload,
			LastError:       handlerErr.Error(),
			RetryCount:      task.retryCount,
			CreatedAt:       task.createdAt,
			DeadLetteredAt:  now,
		}
		s.deadLetters = append(s.deadLetters, entry)
		s.deadMu.Unlock()

		s.updateTaskStatus(ctx, task)
		s.persistDeadLetter(ctx, entry)
		s.deleteTaskFromDB(ctx, task.id)

		s.logger.Warn("task dead-lettered",
			"task_id", task.id,
			"name", task.name,
			"retry_count", task.retryCount,
			"error", handlerErr,
		)
	}
}
