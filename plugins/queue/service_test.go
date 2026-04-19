package queue

import (
	"context"
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
	svc := NewService(cfg, logger)
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
		return err == nil && status.Status == StatusCompleted
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
	assert.Equal(t, StatusPending, status.Status)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == StatusCompleted
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
		return err == nil && status.Status == StatusCompleted
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
			if task.status == StatusCompleted {
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
	svc := NewService(cfg, logger)
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
			if svc.tasks[id].status != StatusCompleted {
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
		done := svc.tasks[blockingID].status == StatusCompleted &&
			svc.tasks[firstID].status == StatusCompleted &&
			svc.tasks[secondID].status == StatusCompleted &&
			svc.tasks[thirdID].status == StatusCompleted
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
		return err == nil && status.Status == StatusCompleted
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
	svc := NewService(cfg, logger)
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
		return err == nil && status.Status == StatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)

	status, _ := svc.GetStatus(ctx, taskID)
	assert.Equal(t, 2, status.RetryCount)
	assert.Equal(t, StatusDeadLettered, status.Status)
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
	svc := NewService(cfg, logger)
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
		return err == nil && status.Status == StatusDeadLettered
	}, 10*time.Second, 100*time.Millisecond)

	dl := svc.DeadLetters()
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
	assert.Contains(t, []string{StatusPending, StatusRunning, StatusCompleted}, status.Status)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == StatusCompleted
	}, 2*time.Second, 50*time.Millisecond)

	status, _ = svc.GetStatus(ctx, taskID)
	assert.Equal(t, StatusCompleted, status.Status)
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
	assert.Equal(t, StatusCompleted, status.Status)
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
	svc := NewService(cfg, logger)
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
		return err == nil && (status.Status == StatusPending || status.Status == StatusDeadLettered)
	}, 5*time.Second, 100*time.Millisecond)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(ctx, taskID)
		return err == nil && status.Status == StatusDeadLettered
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
		assert.Equal(t, StatusCompleted, status.Status)
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
		return err == nil && status.Status == StatusCompleted
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
	svc := NewService(cfg, logger)
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
		return err == nil && status.Status == StatusDeadLettered
	}, 30*time.Second, 100*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, len(attempts), 3)

	if len(attempts) >= 2 {
		gap := attempts[1].Sub(attempts[0])
		assert.GreaterOrEqual(t, gap, 800*time.Millisecond, "backoff between attempt 1 and 2 should be ~1s")
	}
}
