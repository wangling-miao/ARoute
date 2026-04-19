package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wangling-miao/aroute/sdk/interfaces"
	_ "modernc.org/sqlite"
)

var _ interfaces.QueueService = (*Service)(nil)

func newTestService(t *testing.T, workers int) *Service {
	t.Helper()
	cfg := Config{
		Workers:           workers,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 3,
		DefaultTimeout:    5 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	return svc
}

func TestRegisterTask(t *testing.T) {
	svc := newTestService(t, 2)
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "test-task", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	err = svc.RegisterTask(ctx, "test-task", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")

	_, err = svc.Enqueue(ctx, "nonexistent", nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestEnqueue_ImmediateExecution(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	var executed atomic.Bool
	err := svc.RegisterTask(ctx, "immediate", func(ctx context.Context, payload interface{}) error {
		executed.Store(true)
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "immediate", map[string]string{"key": "value"}, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, taskID)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusCompleted
	}, 2*time.Second, 50*time.Millisecond)

	assert.True(t, executed.Load())
}

func TestEnqueue_WithDelay(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	var executed atomic.Int32
	err := svc.RegisterTask(ctx, "delayed", func(ctx context.Context, payload interface{}) error {
		executed.Add(1)
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "delayed", nil, &interfaces.TaskOptions{Delay: 200 * time.Millisecond})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	status, _ := svc.GetStatus(ctx, taskID)
	assert.Equal(t, interfaces.TaskStatusPending, status.Status)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusCompleted
	}, 3*time.Second, 50*time.Millisecond)

	assert.Equal(t, int32(1), executed.Load())
}

func TestEnqueue_EmptyPayload(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	var receivedPayload interface{}
	err := svc.RegisterTask(ctx, "nil-payload", func(ctx context.Context, payload interface{}) error {
		receivedPayload = payload
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "nil-payload", nil, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusCompleted
	}, 2*time.Second, 50*time.Millisecond)

	assert.Nil(t, receivedPayload)
}

func TestWorkerPool_Concurrency(t *testing.T) {
	svc := newTestService(t, 2)
	defer svc.Close(context.Background())

	ctx := context.Background()

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	err := svc.RegisterTask(ctx, "concurrent", func(ctx context.Context, payload interface{}) error {
		c := concurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		concurrent.Add(-1)
		return nil
	})
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_, err := svc.Enqueue(ctx, "concurrent", i, nil)
		require.NoError(t, err)
	}

	assert.Eventually(t, func() bool {
		svc.mu.RLock()
		completed := 0
		for _, task := range svc.tasks {
			if task.status == interfaces.TaskStatusCompleted {
				completed++
			}
		}
		svc.mu.RUnlock()
		return completed == 5
	}, 5*time.Second, 50*time.Millisecond)

	assert.LessOrEqual(t, maxConcurrent.Load(), int32(2))
}

func TestWorkerPool_DefaultWorkers(t *testing.T) {
	cfg := Config{
		Workers:           0,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 3,
		DefaultTimeout:    5 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	assert.Equal(t, 0, svc.WorkerCount())
}

func TestPriority_HighBeforeNormal(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	var mu sync.Mutex
	var order []string

	ready := make(chan struct{})
	err := svc.RegisterTask(ctx, "priority-test", func(ctx context.Context, payload interface{}) error {
		<-ready
		mu.Lock()
		var name string
		if raw, ok := payload.(json.RawMessage); ok {
			json.Unmarshal(raw, &name)
		} else if s, ok := payload.(string); ok {
			name = s
		}
		order = append(order, name)
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)

	blockingID, err := svc.Enqueue(ctx, "priority-test", "blocking", nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	lowID, err := svc.Enqueue(ctx, "priority-test", "low", &interfaces.TaskOptions{Priority: -1})
	require.NoError(t, err)
	normalID, err := svc.Enqueue(ctx, "priority-test", "normal", &interfaces.TaskOptions{Priority: 0})
	require.NoError(t, err)
	highID, err := svc.Enqueue(ctx, "priority-test", "high", &interfaces.TaskOptions{Priority: 1})
	require.NoError(t, err)

	_ = lowID
	_ = normalID
	_ = highID

	close(ready)

	assert.Eventually(t, func() bool {
		svc.mu.RLock()
		done := true
		for _, id := range []string{blockingID, lowID, normalID, highID} {
			if svc.tasks[id].status != interfaces.TaskStatusCompleted {
				done = false
				break
			}
		}
		svc.mu.RUnlock()
		return done
	}, 5*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(order), 4)
	assert.Equal(t, "blocking", order[0])
	assert.Equal(t, "high", order[1])
	assert.Equal(t, "normal", order[2])
	assert.Equal(t, "low", order[3])
}

func TestPriority_SamePriority_FIFO(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	var mu sync.Mutex
	var order []string

	ready := make(chan struct{})
	err := svc.RegisterTask(ctx, "fifo-test", func(ctx context.Context, payload interface{}) error {
		<-ready
		mu.Lock()
		var name string
		if raw, ok := payload.(json.RawMessage); ok {
			json.Unmarshal(raw, &name)
		} else if s, ok := payload.(string); ok {
			name = s
		}
		order = append(order, name)
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)

	blockingID, err := svc.Enqueue(ctx, "fifo-test", "blocker", nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	firstID, err := svc.Enqueue(ctx, "fifo-test", "first", nil)
	require.NoError(t, err)
	secondID, err := svc.Enqueue(ctx, "fifo-test", "second", nil)
	require.NoError(t, err)
	thirdID, err := svc.Enqueue(ctx, "fifo-test", "third", nil)
	require.NoError(t, err)

	_ = firstID
	_ = secondID
	_ = thirdID

	close(ready)

	assert.Eventually(t, func() bool {
		svc.mu.RLock()
		done := svc.tasks[blockingID].status == interfaces.TaskStatusCompleted &&
			svc.tasks[firstID].status == interfaces.TaskStatusCompleted &&
			svc.tasks[secondID].status == interfaces.TaskStatusCompleted &&
			svc.tasks[thirdID].status == interfaces.TaskStatusCompleted
		svc.mu.RUnlock()
		return done
	}, 5*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(order), 4)
	assert.Equal(t, "blocker", order[0])
	assert.Equal(t, "first", order[1])
	assert.Equal(t, "second", order[2])
	assert.Equal(t, "third", order[3])
}

func TestPriority_DefaultIsNormal(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "default-prio", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "default-prio", nil, nil)
	require.NoError(t, err)

	svc.mu.RLock()
	task := svc.tasks[taskID]
	svc.mu.RUnlock()
	assert.Equal(t, 0, task.priority)
}

func TestRetry_SuccessOnRetry(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	var attempt atomic.Int32
	err := svc.RegisterTask(ctx, "retry-success", func(ctx context.Context, payload interface{}) error {
		a := attempt.Add(1)
		if a < 2 {
			return fmt.Errorf("attempt %d failed", a)
		}
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "retry-success", nil, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusCompleted
	}, 5*time.Second, 50*time.Millisecond)

	status, _ := svc.GetStatus(ctx, taskID)
	assert.Equal(t, 1, status.RetryCount)
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 2,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "always-fail", func(ctx context.Context, payload interface{}) error {
		return errors.New("always fails")
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "always-fail", nil, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)

	status, _ := svc.GetStatus(ctx, taskID)
	assert.Equal(t, 2, status.RetryCount)
	assert.Equal(t, interfaces.TaskStatusDeadLettered, status.Status)
	assert.NotEmpty(t, status.Error)
}

func TestDeadLetter_EntryContent(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 1,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "dlq-test", func(ctx context.Context, payload interface{}) error {
		return errors.New("expected failure")
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "dlq-test", map[string]string{"foo": "bar"}, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)

	dl, total, err := svc.ListDeadLetters(ctx, 1, 100)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, dl, 1)

	entry := dl[0]
	assert.Equal(t, taskID, entry.TaskID)
	assert.Equal(t, "dlq-test", entry.Name)
	assert.Equal(t, "expected failure", entry.LastError)
	assert.Equal(t, 1, entry.RetryCount)
	assert.False(t, entry.CreatedAt.IsZero())
	assert.False(t, entry.DeadLetteredAt.IsZero())

	var payload map[string]string
	err = json.Unmarshal(entry.OriginalPayload, &payload)
	require.NoError(t, err)
	assert.Equal(t, "bar", payload["foo"])
}

func TestGetStatus(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "status-test", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "status-test", nil, nil)
	require.NoError(t, err)

	status, err := svc.GetStatus(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, taskID, status.ID)
	assert.Equal(t, "status-test", status.Name)
	assert.Contains(t, []string{interfaces.TaskStatusPending, interfaces.TaskStatusRunning, interfaces.TaskStatusCompleted}, status.Status)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusCompleted
	}, 2*time.Second, 50*time.Millisecond)

	status, _ = svc.GetStatus(ctx, taskID)
	assert.Equal(t, interfaces.TaskStatusCompleted, status.Status)
	assert.NotNil(t, status.StartedAt)
	assert.NotNil(t, status.CompletedAt)
}

func TestGetStatus_NotFound(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	_, err := svc.GetStatus(ctx, "nonexistent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGracefulShutdown_WaitForInflight(t *testing.T) {
	svc := newTestService(t, 1)

	ctx := context.Background()

	var completed atomic.Bool
	err := svc.RegisterTask(ctx, "slow-task", func(ctx context.Context, payload interface{}) error {
		time.Sleep(200 * time.Millisecond)
		completed.Store(true)
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "slow-task", nil, nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	err = svc.Close(ctx)
	require.NoError(t, err)

	assert.True(t, completed.Load())

	status, _ := svc.GetStatus(ctx, taskID)
	assert.Equal(t, interfaces.TaskStatusCompleted, status.Status)
}

func TestGracefulShutdown_RejectNewTasks(t *testing.T) {
	svc := newTestService(t, 1)

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "reject-test", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	go svc.Close(ctx)
	time.Sleep(100 * time.Millisecond)

	_, err = svc.Enqueue(ctx, "reject-test", nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutting down")
}

func TestTaskTimeout(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 1,
		DefaultTimeout:    100 * time.Millisecond,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "slow", func(ctx context.Context, payload interface{}) error {
		select {
		case <-time.After(5 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "slow", nil, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && (status.Status == interfaces.TaskStatusPending || status.Status == interfaces.TaskStatusDeadLettered)
	}, 5*time.Second, 100*time.Millisecond)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)
}

func TestConcurrentEnqueue(t *testing.T) {
	svc := newTestService(t, 4)
	defer svc.Close(context.Background())

	ctx := context.Background()

	var processed atomic.Int32
	err := svc.RegisterTask(ctx, "conc-enqueue", func(ctx context.Context, payload interface{}) error {
		processed.Add(1)
		return nil
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	taskIDs := make([]string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id, err := svc.Enqueue(ctx, "conc-enqueue", idx, nil)
			if err == nil {
				taskIDs[idx] = id
			}
		}(i)
	}
	wg.Wait()

	assert.Eventually(t, func() bool {
		return processed.Load() == 20
	}, 5*time.Second, 50*time.Millisecond)

	for _, id := range taskIDs {
		status, err := svc.GetStatus(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, interfaces.TaskStatusCompleted, status.Status)
	}
}

func TestPayloadSerialization_RoundTrip(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	type complexPayload struct {
		Name    string         `json:"name"`
		Count   int            `json:"count"`
		Tags    []string       `json:"tags"`
		Meta    map[string]int `json:"meta"`
		Enabled bool           `json:"enabled"`
	}

	expected := complexPayload{
		Name:    "test-item",
		Count:   42,
		Tags:    []string{"alpha", "beta"},
		Meta:    map[string]int{"priority": 1, "weight": 100},
		Enabled: true,
	}

	var receivedPayload json.RawMessage
	err := svc.RegisterTask(ctx, "round-trip", func(ctx context.Context, payload interface{}) error {
		receivedPayload = payload.(json.RawMessage)
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "round-trip", expected, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusCompleted
	}, 2*time.Second, 50*time.Millisecond)

	var result complexPayload
	err = json.Unmarshal(receivedPayload, &result)
	require.NoError(t, err)
	assert.Equal(t, expected.Name, result.Name)
	assert.Equal(t, expected.Count, result.Count)
	assert.Equal(t, expected.Tags, result.Tags)
	assert.Equal(t, expected.Enabled, result.Enabled)
	for k, v := range expected.Meta {
		assert.Equal(t, v, result.Meta[k])
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   15 * time.Second,
		DefaultMaxRetries: 3,
		DefaultTimeout:    5 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	var attempts []time.Time
	var mu sync.Mutex

	err := svc.RegisterTask(ctx, "backoff-test", func(ctx context.Context, payload interface{}) error {
		mu.Lock()
		attempts = append(attempts, time.Now())
		mu.Unlock()
		return errors.New("fail")
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "backoff-test", nil, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 30*time.Second, 100*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, len(attempts), 3)

	if len(attempts) >= 2 {
		gap := attempts[1].Sub(attempts[0])
		assert.GreaterOrEqual(t, gap, 800*time.Millisecond, "backoff between attempt 1 and 2 should be ~1s")
	}
}

func TestListDeadLetters_Pagination(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 0,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "pag-test", func(ctx context.Context, payload interface{}) error {
		return errors.New("fail")
	})
	require.NoError(t, err)

	var taskIDs []string
	for i := 0; i < 5; i++ {
		id, err := svc.Enqueue(ctx, "pag-test", i, nil)
		require.NoError(t, err)
		taskIDs = append(taskIDs, id)
	}

	assert.Eventually(t, func() bool {
		_, total, _ := svc.ListDeadLetters(ctx, 1, 100)
		return total == 5
	}, 10*time.Second, 100*time.Millisecond)

	page1, total, err := svc.ListDeadLetters(ctx, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, page1, 2)

	page2, total, err := svc.ListDeadLetters(ctx, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, page2, 2)

	page3, total, err := svc.ListDeadLetters(ctx, 3, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, page3, 1)

	page4, total, err := svc.ListDeadLetters(ctx, 4, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Empty(t, page4)

	_ = taskIDs
}

func TestRetryDeadLetter(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 0,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	var attempt atomic.Int32
	err := svc.RegisterTask(ctx, "retry-dl", func(ctx context.Context, payload interface{}) error {
		a := attempt.Add(1)
		if a == 1 {
			return errors.New("first fail")
		}
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "retry-dl", "hello", nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)

	dl, total, err := svc.ListDeadLetters(ctx, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, taskID, dl[0].TaskID)

	err = svc.RetryDeadLetter(ctx, taskID)
	require.NoError(t, err)

	dl, total, err = svc.ListDeadLetters(ctx, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, dl)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusCompleted
	}, 5*time.Second, 100*time.Millisecond)
}

func TestDeleteDeadLetter(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 0,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "delete-dl", func(ctx context.Context, payload interface{}) error {
		return errors.New("always fail")
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "delete-dl", nil, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)

	_, total, err := svc.ListDeadLetters(ctx, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, total)

	err = svc.DeleteDeadLetter(ctx, taskID)
	require.NoError(t, err)

	dl, total, err := svc.ListDeadLetters(ctx, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, dl)

	_, err = svc.GetStatus(ctx, taskID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRetryDeadLetter_NotFound(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RetryDeadLetter(ctx, "nonexistent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDeleteDeadLetter_NotFound(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.DeleteDeadLetter(ctx, "nonexistent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestClose_TimeoutExceeded(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   100 * time.Millisecond,
		DefaultMaxRetries: 0,
		DefaultTimeout:    5 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "slow-blocker", func(ctx context.Context, payload interface{}) error {
		time.Sleep(5 * time.Second)
		return nil
	})
	require.NoError(t, err)

	_, err = svc.Enqueue(ctx, "slow-blocker", nil, nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	err = svc.Close(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestClose_ContextCancelled(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   30 * time.Second,
		DefaultMaxRetries: 0,
		DefaultTimeout:    30 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()

	err := svc.RegisterTask(context.Background(), "ctx-cancel-blocker", func(ctx context.Context, payload interface{}) error {
		time.Sleep(30 * time.Second)
		return nil
	})
	require.NoError(t, err)

	_, err = svc.Enqueue(context.Background(), "ctx-cancel-blocker", nil, nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err = svc.Close(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestExecuteTask_UnregisteredHandler(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 1,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "temp-task", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "temp-task", "data", nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusCompleted
	}, 2*time.Second, 50*time.Millisecond)

	ready := make(chan struct{})
	done := make(chan struct{})
	err = svc.RegisterTask(ctx, "unreg-test", func(ctx context.Context, payload interface{}) error {
		close(ready)
		<-done
		return nil
	})
	require.NoError(t, err)

	err = svc.RegisterTask(ctx, "will-unregister", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	taskID2, err := svc.Enqueue(ctx, "unreg-test", nil, nil)
	require.NoError(t, err)

	<-ready

	taskID3, err := svc.Enqueue(ctx, "will-unregister", nil, nil)
	require.NoError(t, err)

	svc.mu.Lock()
	delete(svc.taskTypes, "will-unregister")
	svc.mu.Unlock()

	close(done)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID3)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 5*time.Second, 100*time.Millisecond)

	status2, _ := svc.GetStatus(ctx, taskID2)
	assert.Equal(t, interfaces.TaskStatusCompleted, status2.Status)
}

func TestEnqueue_InvalidPayload(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "invalid-payload-test", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	_, err = svc.Enqueue(ctx, "invalid-payload-test", make(chan int), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "serialize payload")
}

func TestEnqueue_InvalidPriority(t *testing.T) {
	svc := newTestService(t, 1)
	defer svc.Close(context.Background())

	ctx := context.Background()

	err := svc.RegisterTask(ctx, "invalid-prio-test", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	_, err = svc.Enqueue(ctx, "invalid-prio-test", nil, &interfaces.TaskOptions{Priority: 999})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid priority")

	_, err = svc.Enqueue(ctx, "invalid-prio-test", nil, &interfaces.TaskOptions{Priority: -999})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid priority")
}

func TestInitDB_NilDB(t *testing.T) {
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 3,
		DefaultTimeout:    5 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)

	err := svc.InitDB(context.Background())
	assert.NoError(t, err)
}

// --- SQLite persistence tests ---

// testDBService wraps *sql.DB to implement interfaces.DatabaseService.
type testDBService struct {
	db *sql.DB
}

func (t *testDBService) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.db.QueryContext(ctx, query, args...)
}

func (t *testDBService) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.db.QueryRowContext(ctx, query, args...)
}

func (t *testDBService) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.db.ExecContext(ctx, query, args...)
}

func (t *testDBService) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return t.db.BeginTx(ctx, opts)
}

func (t *testDBService) Ping(ctx context.Context) error {
	return t.db.PingContext(ctx)
}

func (t *testDBService) Close() error { return t.db.Close() }

func (t *testDBService) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.db.PrepareContext(ctx, query)
}

func (t *testDBService) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, nil
}

// newTestServiceWithDB creates a Service backed by a real in-memory SQLite database.
func newTestServiceWithDB(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	testDB := &testDBService{db: db}
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 3,
		DefaultTimeout:    5 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, testDB)
	return svc, db
}

func TestInitDB_CreatesTables(t *testing.T) {
	svc, db := newTestServiceWithDB(t)

	err := svc.InitDB(context.Background())
	require.NoError(t, err)

	rows, err := db.QueryContext(context.Background(),
		"SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	require.NoError(t, rows.Err())

	assert.Contains(t, tables, "queue_tasks")
	assert.Contains(t, tables, "queue_dead_letters")
}

func TestInitDB_LoadsPersistedTasks(t *testing.T) {
	svc, db := newTestServiceWithDB(t)

	// Create tables manually and insert a pending task.
	_, err := db.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS queue_tasks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		payload TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		priority INTEGER NOT NULL DEFAULT 0,
		retry_count INTEGER NOT NULL DEFAULT 0,
		max_retries INTEGER NOT NULL DEFAULT 3,
		timeout_ms INTEGER NOT NULL DEFAULT 60000,
		created_at TEXT NOT NULL,
		started_at TEXT,
		completed_at TEXT,
		error TEXT,
		delay_ms INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)

	_, err = db.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS queue_dead_letters (
		task_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		original_payload TEXT,
		last_error TEXT,
		retry_count INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		dead_lettered_at TEXT NOT NULL
	)`)
	require.NoError(t, err)

	taskID := "persisted-task-001"
	payload := `{"key":"value"}`
	now := time.Now().UTC().Truncate(time.Second)

	_, err = db.ExecContext(context.Background(),
		`INSERT INTO queue_tasks (id, name, payload, status, priority, retry_count, max_retries, timeout_ms, created_at, delay_ms)
		 VALUES (?, ?, ?, 'pending', 1, 0, 3, 60000, ?, 0)`,
		taskID, "test-load", payload, now)
	require.NoError(t, err)

	// Also insert a completed task — should NOT be loaded.
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO queue_tasks (id, name, payload, status, priority, retry_count, max_retries, timeout_ms, created_at, delay_ms)
		 VALUES (?, ?, ?, 'completed', 0, 0, 3, 60000, ?, 0)`,
		"completed-task-999", "no-load", "null", now)
	require.NoError(t, err)

	// InitDB should load only the pending task.
	err = svc.InitDB(context.Background())
	require.NoError(t, err)

	svc.mu.RLock()
	task, exists := svc.tasks[taskID]
	svc.mu.RUnlock()
	require.True(t, exists, "pending task should be loaded into tasks map")
	assert.Equal(t, "test-load", task.name)
	assert.Equal(t, interfaces.TaskStatusPending, task.status)
	assert.Equal(t, 1, task.priority)
	assert.Equal(t, json.RawMessage(`{"key":"value"}`), task.payload)

	// Completed task should not be loaded.
	svc.mu.RLock()
	_, existsCompleted := svc.tasks["completed-task-999"]
	svc.mu.RUnlock()
	assert.False(t, existsCompleted, "completed task should not be loaded")
}

func TestInitDB_LoadsDeadLetters(t *testing.T) {
	svc, db := newTestServiceWithDB(t)

	// Create tables.
	_, err := db.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS queue_tasks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		payload TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		priority INTEGER NOT NULL DEFAULT 0,
		retry_count INTEGER NOT NULL DEFAULT 0,
		max_retries INTEGER NOT NULL DEFAULT 3,
		timeout_ms INTEGER NOT NULL DEFAULT 60000,
		created_at TEXT NOT NULL,
		started_at TEXT,
		completed_at TEXT,
		error TEXT,
		delay_ms INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)

	_, err = db.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS queue_dead_letters (
		task_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		original_payload TEXT,
		last_error TEXT,
		retry_count INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		dead_lettered_at TEXT NOT NULL
	)`)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO queue_dead_letters (task_id, name, original_payload, last_error, retry_count, created_at, dead_lettered_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"dl-task-001", "dead-task", `{"x":1}`, "some error", 2, now, now)
	require.NoError(t, err)

	err = svc.InitDB(context.Background())
	require.NoError(t, err)

	entries, total, err := svc.ListDeadLetters(context.Background(), 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, "dl-task-001", entry.TaskID)
	assert.Equal(t, "dead-task", entry.Name)
	assert.Equal(t, "some error", entry.LastError)
	assert.Equal(t, 2, entry.RetryCount)
}

func TestPersistTask_InsertAndRetrieve(t *testing.T) {
	svc, db := newTestServiceWithDB(t)

	err := svc.InitDB(context.Background())
	require.NoError(t, err)

	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err = svc.RegisterTask(ctx, "persist-test", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "persist-test", map[string]string{"hello": "world"}, nil)
	require.NoError(t, err)

	// Verify the task was persisted to SQLite.
	var name, status string
	var payloadStr string
	err = db.QueryRowContext(ctx,
		"SELECT name, status, payload FROM queue_tasks WHERE id = ?", taskID).
		Scan(&name, &status, &payloadStr)
	require.NoError(t, err, "task should exist in SQLite")
	assert.Equal(t, "persist-test", name)
	assert.Equal(t, interfaces.TaskStatusPending, status)
	assert.Contains(t, payloadStr, "world")

	// Wait for completion — task gets deleted from DB on success.
	assert.Eventually(t, func() bool {
		s, err := svc.GetStatus(ctx, taskID)
		return err == nil && s.Status == interfaces.TaskStatusCompleted
	}, 3*time.Second, 50*time.Millisecond)

	// After completion, the task row should be deleted.
	err = db.QueryRowContext(ctx,
		"SELECT id FROM queue_tasks WHERE id = ?", taskID).Scan(&taskID)
	assert.Equal(t, sql.ErrNoRows, err, "completed task should be removed from DB")
}

func TestUpdateTaskStatus(t *testing.T) {
	svc, db := newTestServiceWithDB(t)

	err := svc.InitDB(context.Background())
	require.NoError(t, err)

	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err = svc.RegisterTask(ctx, "update-status-test", func(ctx context.Context, payload interface{}) error {
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "update-status-test", nil, nil)
	require.NoError(t, err)

	// Wait for the task to complete.
	assert.Eventually(t, func() bool {
		s, err := svc.GetStatus(ctx, taskID)
		return err == nil && s.Status == interfaces.TaskStatusCompleted
	}, 3*time.Second, 50*time.Millisecond)

	// The completed task gets deleteTaskFromDB called on success, so verify
	// it no longer exists. During execution, the status was updated to
	// running → completed. The updateTaskStatus method was called.
	// Since deleteTaskFromDB removes the row on completion, verify absence.
	var dummy string
	err = db.QueryRowContext(ctx,
		"SELECT id FROM queue_tasks WHERE id = ?", taskID).Scan(&dummy)
	assert.Equal(t, sql.ErrNoRows, err, "completed task row should be deleted")
}

func TestPersistDeadLetter_OnMaxRetries(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	testDB := &testDBService{db: db}
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 1,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, testDB)

	err = svc.InitDB(context.Background())
	require.NoError(t, err)

	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err = svc.RegisterTask(ctx, "dlq-persist", func(ctx context.Context, payload interface{}) error {
		return errors.New("persistent failure")
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "dlq-persist", map[string]int{"n": 1}, nil)
	require.NoError(t, err)

	// Wait for dead-lettering.
	assert.Eventually(t, func() bool {
		s, err := svc.GetStatus(ctx, taskID)
		return err == nil && s.Status == interfaces.TaskStatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)

	// Verify dead letter in SQLite.
	var name, lastErr string
	var retryCount int
	err = db.QueryRowContext(ctx,
		"SELECT name, last_error, retry_count FROM queue_dead_letters WHERE task_id = ?",
		taskID).Scan(&name, &lastErr, &retryCount)
	require.NoError(t, err, "dead letter should exist in SQLite")
	assert.Equal(t, "dlq-persist", name)
	assert.Equal(t, "persistent failure", lastErr)
	assert.Equal(t, 1, retryCount)

	// The task row should have been deleted from queue_tasks.
	var dummy string
	err = db.QueryRowContext(ctx,
		"SELECT id FROM queue_tasks WHERE id = ?", taskID).Scan(&dummy)
	assert.Equal(t, sql.ErrNoRows, err, "dead-lettered task should be removed from queue_tasks")
}

func TestDeleteDeadLetter_RemovesFromDB(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	testDB := &testDBService{db: db}
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 0,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, testDB)

	err = svc.InitDB(context.Background())
	require.NoError(t, err)

	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	err = svc.RegisterTask(ctx, "delete-dl-db", func(ctx context.Context, payload interface{}) error {
		return errors.New("always fail")
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "delete-dl-db", nil, nil)
	require.NoError(t, err)

	// Wait for dead-lettering.
	assert.Eventually(t, func() bool {
		s, err := svc.GetStatus(ctx, taskID)
		return err == nil && s.Status == interfaces.TaskStatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)

	// Verify dead letter exists in DB before deletion.
	var count int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM queue_dead_letters WHERE task_id = ?", taskID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	err = svc.DeleteDeadLetter(ctx, taskID)
	require.NoError(t, err)

	// Verify removed from DB.
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM queue_dead_letters WHERE task_id = ?", taskID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify task also removed from queue_tasks.
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM queue_tasks WHERE id = ?", taskID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestRetryDeadLetter_RemovesFromDB(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	testDB := &testDBService{db: db}
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 0,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, testDB)

	err = svc.InitDB(context.Background())
	require.NoError(t, err)

	svc.Start()
	defer svc.Close(context.Background())

	ctx := context.Background()

	var attempt atomic.Int32
	err = svc.RegisterTask(ctx, "retry-dl-db", func(ctx context.Context, payload interface{}) error {
		a := attempt.Add(1)
		if a == 1 {
			return errors.New("first fail")
		}
		return nil
	})
	require.NoError(t, err)

	taskID, err := svc.Enqueue(ctx, "retry-dl-db", "hello", nil)
	require.NoError(t, err)

	// Wait for dead-lettering.
	assert.Eventually(t, func() bool {
		s, err := svc.GetStatus(ctx, taskID)
		return err == nil && s.Status == interfaces.TaskStatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)

	// Verify dead letter exists in DB.
	var dlCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM queue_dead_letters WHERE task_id = ?", taskID).Scan(&dlCount)
	require.NoError(t, err)
	assert.Equal(t, 1, dlCount)

	// Retry the dead letter.
	err = svc.RetryDeadLetter(ctx, taskID)
	require.NoError(t, err)

	// Verify dead letter removed from queue_dead_letters.
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM queue_dead_letters WHERE task_id = ?", taskID).Scan(&dlCount)
	require.NoError(t, err)
	assert.Equal(t, 0, dlCount, "dead letter should be removed from DB after retry")

	// Verify new task inserted into queue_tasks (it will be picked up and completed).
	assert.Eventually(t, func() bool {
		s, err := svc.GetStatus(ctx, taskID)
		return err == nil && s.Status == interfaces.TaskStatusCompleted
	}, 5*time.Second, 100*time.Millisecond)

	// After completion, task row should be deleted from DB.
	var taskCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM queue_tasks WHERE id = ?", taskID).Scan(&taskCount)
	require.NoError(t, err)
	assert.Equal(t, 0, taskCount, "completed retried task should be removed from DB")
}
