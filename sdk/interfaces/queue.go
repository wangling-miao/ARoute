package interfaces

import (
	"context"
	"encoding/json"
	"time"
)

// Priority constants for task scheduling.
const (
	PriorityHigh   = 1
	PriorityNormal = 0
	PriorityLow    = -1
)

// Task statuses.
const (
	TaskStatusPending      = "pending"
	TaskStatusRunning      = "running"
	TaskStatusCompleted    = "completed"
	TaskStatusFailed       = "failed"
	TaskStatusDeadLettered = "dead-lettered"
)

// QueueService defines asynchronous task processing operations.
// It implements a lightweight goroutine-based worker pool for
// background task execution with retry and dead-letter support.
type QueueService interface {
	// RegisterTask registers a task handler for a named task type.
	// Only one handler can be registered per task type.
	RegisterTask(ctx context.Context, name string, handler TaskHandler) error

	// Enqueue adds a task to the queue for asynchronous processing.
	// Returns a task ID that can be used to cancel or check status.
	Enqueue(ctx context.Context, name string, payload interface{}, options *TaskOptions) (string, error)

	// GetStatus retrieves the current status of a queued task.
	GetStatus(ctx context.Context, taskID string) (*TaskStatus, error)

	// Close gracefully shuts down the worker pool, waiting for in-flight tasks.
	Close(ctx context.Context) error

	// ListDeadLetters returns a paginated list of dead-lettered tasks.
	// Returns the entries and total count for pagination.
	ListDeadLetters(ctx context.Context, page, pageSize int) ([]*DeadLetterEntry, int, error)

	// RetryDeadLetter re-enqueues a dead-lettered task with retry count reset to 0.
	// Removes the task from the dead letter queue.
	RetryDeadLetter(ctx context.Context, taskID string) error

	// DeleteDeadLetter permanently removes a task from the dead letter queue.
	DeleteDeadLetter(ctx context.Context, taskID string) error

	// WorkerCount returns the number of workers in the pool.
	WorkerCount() int
}

// TaskHandler is the function signature for task handlers.
// It receives the task payload and context, and returns an error if processing fails.
type TaskHandler func(ctx context.Context, payload interface{}) error

// TaskOptions contains task configuration options.
type TaskOptions struct {
	// Delay is the time to wait before processing the task.
	// Set to 0 for immediate processing.
	Delay time.Duration `json:"delay,omitempty"`

	// Priority is the task priority (0 = normal, 1 = high, -1 = low).
	Priority int `json:"priority,omitempty"`

	// MaxRetries is the maximum number of retry attempts on failure (default: 3).
	MaxRetries int `json:"max_retries,omitempty"`

	// Timeout is the maximum time allowed for task execution.
	// Set to 0 for no timeout (not recommended).
	Timeout time.Duration `json:"timeout,omitempty"`
}

// TaskStatus represents the current state of a task.
type TaskStatus struct {
	// ID is the unique task identifier.
	ID string `json:"id"`

	// Name is the task type name.
	Name string `json:"name"`

	// Status is the current task status (pending, running, completed, failed).
	Status string `json:"status"`

	// CreatedAt is when the task was enqueued.
	CreatedAt time.Time `json:"created_at"`

	// StartedAt is when the task started processing (nullable).
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the task finished (nullable).
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Error contains the error message if the task failed.
	Error string `json:"error,omitempty"`

	// RetryCount is the number of times this task has been retried.
	RetryCount int `json:"retry_count"`
}

// DeadLetterEntry represents a task that has exhausted all retry attempts.
type DeadLetterEntry struct {
	TaskID          string          `json:"task_id"`
	Name            string          `json:"name"`
	OriginalPayload json.RawMessage `json:"original_payload"`
	LastError       string          `json:"last_error"`
	RetryCount      int             `json:"retry_count"`
	CreatedAt       time.Time       `json:"created_at"`
	DeadLetteredAt  time.Time       `json:"dead_lettered_at"`
}
