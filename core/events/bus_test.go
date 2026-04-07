package events_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangling-miao/aroute/core/events"
)

func TestNewEventBus(t *testing.T) {
	eb := events.NewEventBus()
	if eb == nil {
		t.Fatal("NewEventBus returned nil")
	}
}

func TestSubscribeFilter_WithPriority(t *testing.T) {
	eb := events.NewEventBus()

	var executed []string
	handlerA := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "A")
		return event, nil
	}
	handlerB := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "B")
		return event, nil
	}

	idA := eb.SubscribeFilter("test.event", 10, handlerA)
	idB := eb.SubscribeFilter("test.event", 20, handlerB)

	if idA == "" {
		t.Error("SubscribeFilter should return non-empty handler ID")
	}
	if idB == "" {
		t.Error("SubscribeFilter should return non-empty handler ID")
	}
	if idA == idB {
		t.Error("Handler IDs should be unique")
	}

	event := &events.Event{Topic: "test.event", Data: map[string]interface{}{"key": "value"}}
	_, err := eb.DispatchFilter(context.Background(), event)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	if len(executed) != 2 {
		t.Fatalf("expected 2 handlers executed, got %d", len(executed))
	}
	// Lower priority should execute first
	if executed[0] != "A" || executed[1] != "B" {
		t.Errorf("expected execution order [A, B], got %v", executed)
	}
}

func TestSubscribeFilter_SamePriority(t *testing.T) {
	eb := events.NewEventBus()

	var executed []string
	for i := 0; i < 3; i++ {
		idx := i
		handler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
			executed = append(executed, string(rune('A'+idx)))
			return event, nil
		}
		eb.SubscribeFilter("test.event", 10, handler)
	}

	event := &events.Event{Topic: "test.event"}
	_, err := eb.DispatchFilter(context.Background(), event)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	// Same priority should execute in FIFO order
	if len(executed) != 3 {
		t.Errorf("expected 3 handlers, got %d", len(executed))
	}
	if executed[0] != "A" || executed[1] != "B" || executed[2] != "C" {
		t.Errorf("expected FIFO order [A, B, C], got %v", executed)
	}
}

func TestDispatchFilter_ChainAbort(t *testing.T) {
	eb := events.NewEventBus()

	var executed []string
	errHandler := errors.New("handler B error")

	handlerA := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "A")
		event.Data["fromA"] = true
		return event, nil
	}

	handlerB := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "B")
		return nil, errHandler
	}

	handlerC := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "C")
		return event, nil
	}

	eb.SubscribeFilter("test.event", 10, handlerA)
	eb.SubscribeFilter("test.event", 20, handlerB)
	eb.SubscribeFilter("test.event", 30, handlerC)

	event := &events.Event{Topic: "test.event", Data: map[string]interface{}{}}
	result, err := eb.DispatchFilter(context.Background(), event)

	if err != errHandler {
		t.Errorf("expected error from handler B, got %v", err)
	}
	if result != nil {
		t.Error("result should be nil when error occurs")
	}
	if len(executed) != 2 {
		t.Errorf("expected 2 handlers executed before abort, got %d", len(executed))
	}
	if executed[0] != "A" || executed[1] != "B" {
		t.Errorf("expected [A, B], got %v", executed)
	}
}

func TestDispatchFilter_EventModification(t *testing.T) {
	eb := events.NewEventBus()

	handlerA := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		event.Data["step1"] = "done"
		event.Data["counter"] = 1
		return event, nil
	}

	handlerB := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		if counter, ok := event.Data["counter"].(int); ok {
			event.Data["counter"] = counter + 1
		}
		event.Data["step2"] = "done"
		return event, nil
	}

	eb.SubscribeFilter("test.event", 10, handlerA)
	eb.SubscribeFilter("test.event", 20, handlerB)

	event := &events.Event{Topic: "test.event", Data: map[string]interface{}{}}
	result, err := eb.DispatchFilter(context.Background(), event)

	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	if result.Data["step1"] != "done" {
		t.Error("handler A should have modified event")
	}
	if result.Data["step2"] != "done" {
		t.Error("handler B should have modified event")
	}
	if result.Data["counter"] != 2 {
		t.Errorf("expected counter=2, got %v", result.Data["counter"])
	}
}

func TestDispatchFilter_NoHandlers(t *testing.T) {
	eb := events.NewEventBus()

	event := &events.Event{Topic: "nonexistent.event", Data: map[string]interface{}{}}
	result, err := eb.DispatchFilter(context.Background(), event)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != event {
		t.Error("result should be the original event when no handlers")
	}
}

func TestDispatchFilter_SingleHandler(t *testing.T) {
	eb := events.NewEventBus()

	handler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		event.Data["processed"] = true
		return event, nil
	}

	eb.SubscribeFilter("test.event", 10, handler)

	event := &events.Event{Topic: "test.event", Data: map[string]interface{}{}}
	result, err := eb.DispatchFilter(context.Background(), event)

	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}
	if result.Data["processed"] != true {
		t.Error("handler should have processed event")
	}
}

func TestSubscribeBroadcast_Basic(t *testing.T) {
	eb := events.NewEventBus()

	var received []string
	var mu sync.Mutex
	var done sync.WaitGroup

	handler := func(ctx context.Context, event events.Event) {
		defer done.Done()
		mu.Lock()
		received = append(received, event.Topic)
		mu.Unlock()
	}

	id := eb.SubscribeBroadcast("test.event", handler)
	if id == "" {
		t.Error("SubscribeBroadcast should return non-empty handler ID")
	}

	done.Add(1)
	event := events.Event{Topic: "test.event", Data: map[string]interface{}{"key": "value"}}
	eb.Emit(context.Background(), event)

	// Wait for handler to complete
	done.Wait()

	mu.Lock()
	if len(received) != 1 {
		t.Errorf("expected 1 event received, got %d", len(received))
	}
	mu.Unlock()
}

func TestSubscribeBroadcast_MultipleHandlers(t *testing.T) {
	eb := events.NewEventBus()

	var counter atomic.Int32
	var done sync.WaitGroup

	handler := func(ctx context.Context, event events.Event) {
		defer done.Done()
		counter.Add(1)
	}

	eb.SubscribeBroadcast("test.event", handler)
	eb.SubscribeBroadcast("test.event", handler)
	eb.SubscribeBroadcast("test.event", handler)

	done.Add(3)
	event := events.Event{Topic: "test.event"}
	eb.Emit(context.Background(), event)

	// Wait for all handlers to complete
	done.Wait()

	if counter.Load() != 3 {
		t.Errorf("expected all 3 handlers to execute, got %d", counter.Load())
	}
}

func TestSubscribeBroadcast_HandlerPanic(t *testing.T) {
	eb := events.NewEventBus()

	var executed atomic.Int32
	var done sync.WaitGroup

	handlerA := func(ctx context.Context, event events.Event) {
		defer done.Done()
		executed.Add(1)
		panic("handler A panicked")
	}

	handlerB := func(ctx context.Context, event events.Event) {
		defer done.Done()
		executed.Add(1)
	}

	eb.SubscribeBroadcast("test.event", handlerA)
	eb.SubscribeBroadcast("test.event", handlerB)

	done.Add(2)
	event := events.Event{Topic: "test.event"}
	eb.Emit(context.Background(), event)

	// Wait for all handlers to complete
	done.Wait()

	// Both handlers should execute despite panic
	if executed.Load() != 2 {
		t.Errorf("expected both handlers to execute (2), got %d", executed.Load())
	}
}

func TestSubscribeBroadcast_EventCopy(t *testing.T) {
	eb := events.NewEventBus()

	var mu sync.Mutex
	var receivedEvent events.Event
	var done sync.WaitGroup

	handler := func(ctx context.Context, event events.Event) {
		defer done.Done()
		mu.Lock()
		receivedEvent = event
		mu.Unlock()
		// Modify the copy
		event.Data["modified"] = true
	}

	eb.SubscribeBroadcast("test.event", handler)

	done.Add(1)
	event := events.Event{Topic: "test.event", Data: map[string]interface{}{"original": true}}
	eb.Emit(context.Background(), event)

	// Wait for handler to complete
	done.Wait()

	mu.Lock()
	defer mu.Unlock()

	// Original event should not have "modified" key
	if _, ok := event.Data["modified"]; ok {
		t.Error("original event should not be modified by broadcast handler")
	}
	// Received event should have the original data
	if receivedEvent.Data["original"] != true {
		t.Error("handler should receive copy with original data")
	}
}

func TestWildcard_SingleSegment(t *testing.T) {
	eb := events.NewEventBus()

	var filterExecuted atomic.Bool
	var broadcastExecuted atomic.Bool
	var done sync.WaitGroup

	filterHandler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		filterExecuted.Store(true)
		return event, nil
	}

	broadcastHandler := func(ctx context.Context, event events.Event) {
		defer done.Done()
		broadcastExecuted.Store(true)
	}

	eb.SubscribeFilter("content.post.*", 10, filterHandler)
	eb.SubscribeBroadcast("content.post.*", broadcastHandler)

	// Should match
	eventFilter := &events.Event{Topic: "content.post.saved"}
	_, err := eb.DispatchFilter(context.Background(), eventFilter)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	done.Add(1)
	eventBroadcast := events.Event{Topic: "content.post.deleted"}
	eb.Emit(context.Background(), eventBroadcast)
	done.Wait()

	if !filterExecuted.Load() {
		t.Error("filter handler should have executed for content.post.saved")
	}
	if !broadcastExecuted.Load() {
		t.Error("broadcast handler should have executed for content.post.deleted")
	}

	// Should NOT match (two segments after post)
	filterExecuted.Store(false)
	broadcastExecuted.Store(false)

	eventNoMatch := &events.Event{Topic: "content.post.field.updated"}
	_, err = eb.DispatchFilter(context.Background(), eventNoMatch)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	if filterExecuted.Load() {
		t.Error("filter handler should NOT have executed for content.post.field.updated")
	}
}

func TestWildcard_DoubleSegment(t *testing.T) {
	eb := events.NewEventBus()

	var executed atomic.Bool

	handler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed.Store(true)
		return event, nil
	}

	eb.SubscribeFilter("content.**", 10, handler)

	// Should match all content.* events
	testCases := []string{
		"content.post.saved",
		"content.page.deleted",
		"content.post.field.updated",
	}

	for _, topic := range testCases {
		executed.Store(false)
		event := &events.Event{Topic: topic}
		_, err := eb.DispatchFilter(context.Background(), event)
		if err != nil {
			t.Fatalf("DispatchFilter failed for %s: %v", topic, err)
		}
		if !executed.Load() {
			t.Errorf("handler should have executed for %s", topic)
		}
	}
}

func TestWildcard_MatchEverything(t *testing.T) {
	eb := events.NewEventBus()

	var executed atomic.Int32

	handler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed.Add(1)
		return event, nil
	}

	eb.SubscribeFilter("**", 10, handler)

	// ** matches everything
	testCases := []string{
		"any.event",
		"deeply.nested.event.here",
		"single",
	}

	for _, topic := range testCases {
		event := &events.Event{Topic: topic}
		_, err := eb.DispatchFilter(context.Background(), event)
		if err != nil {
			t.Fatalf("DispatchFilter failed for %s: %v", topic, err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			if executed.Load() == int32(len(testCases)) {
				return
			}
		}
	}()
	wg.Wait()

	if executed.Load() != int32(len(testCases)) {
		t.Errorf("expected %d executions, got %d", len(testCases), executed.Load())
	}
}

func TestWildcard_ExactMatch(t *testing.T) {
	eb := events.NewEventBus()

	var executed atomic.Bool

	handler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed.Store(true)
		return event, nil
	}

	eb.SubscribeFilter("content.post.saved", 10, handler)

	// Should match exact topic
	event := &events.Event{Topic: "content.post.saved"}
	_, err := eb.DispatchFilter(context.Background(), event)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	if !executed.Load() {
		t.Error("handler should execute for exact match")
	}

	// Should NOT match different topic
	executed.Store(false)
	event2 := &events.Event{Topic: "content.post.saved.extra"}
	_, err = eb.DispatchFilter(context.Background(), event2)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	if executed.Load() {
		t.Error("handler should NOT execute for non-matching topic")
	}
}

func TestUnsubscribe(t *testing.T) {
	eb := events.NewEventBus()

	var executed atomic.Bool

	handler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed.Store(true)
		return event, nil
	}

	id := eb.SubscribeFilter("test.event", 10, handler)

	// Handler should work before unsubscribe
	event := &events.Event{Topic: "test.event"}
	_, err := eb.DispatchFilter(context.Background(), event)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	if !executed.Load() {
		t.Error("handler should execute before unsubscribe")
	}

	// Unsubscribe
	eb.Unsubscribe(id)

	// Handler should not work after unsubscribe
	executed.Store(false)
	_, err = eb.DispatchFilter(context.Background(), event)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	if executed.Load() {
		t.Error("handler should NOT execute after unsubscribe")
	}
}

func TestUnsubscribe_Broadcast(t *testing.T) {
	eb := events.NewEventBus()

	var executed atomic.Bool
	var done sync.WaitGroup

	handler := func(ctx context.Context, event events.Event) {
		defer done.Done()
		executed.Store(true)
	}

	id := eb.SubscribeBroadcast("test.event", handler)

	// Handler should work before unsubscribe
	done.Add(1)
	event := events.Event{Topic: "test.event"}
	eb.Emit(context.Background(), event)
	done.Wait()

	if !executed.Load() {
		t.Error("handler should execute before unsubscribe")
	}

	// Unsubscribe
	eb.Unsubscribe(id)

	// Handler should not work after unsubscribe
	// We can't use WaitGroup here because the handler won't be called
	// Just verify that executed is still false after a brief moment
	executed.Store(false)
	eb.Emit(context.Background(), event)

	// Give a brief moment for any potential goroutine to start
	// This is a fire-and-forget model, so we just check that the counter doesn't increment
	// In a real scenario, the handler should not execute
	time.Sleep(10 * time.Millisecond)

	// executed should still be false because handler was unsubscribed
	if executed.Load() {
		t.Error("handler should NOT execute after unsubscribe")
	}
}

func TestUnsubscribe_NonExistent(t *testing.T) {
	eb := events.NewEventBus()

	// Should not panic or error
	eb.Unsubscribe("non-existent-id")
	eb.Unsubscribe("")
}

func TestConcurrent_Subscription(t *testing.T) {
	eb := events.NewEventBus()

	var wg sync.WaitGroup
	var subscribeCount atomic.Int32

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				handler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
					return event, nil
				}
				eb.SubscribeFilter("test.event", 10, handler)
				subscribeCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if subscribeCount.Load() != 500 {
		t.Errorf("expected 500 subscriptions, got %d", subscribeCount.Load())
	}
}

func TestConcurrent_EmissionAndSubscription(t *testing.T) {
	eb := events.NewEventBus()

	var wg sync.WaitGroup
	var emitCount atomic.Int32

	// Start emitter
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				event := &events.Event{Topic: "test.event"}
				eb.DispatchFilter(context.Background(), event)
				emitCount.Add(1)
			}
		}()
	}

	// Start subscriber
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				handler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
					return event, nil
				}
				eb.SubscribeFilter("test.event", 10, handler)
			}
		}()
	}

	wg.Wait()

	// Should complete without data race (verified by Go race detector)
}

func TestConcurrent_EmissionOnSameTopic(t *testing.T) {
	eb := events.NewEventBus()

	var counter atomic.Int32

	handler := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		counter.Add(1)
		return event, nil
	}

	eb.SubscribeFilter("test.event", 10, handler)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				event := &events.Event{Topic: "test.event"}
				_, err := eb.DispatchFilter(context.Background(), event)
				if err != nil {
					t.Errorf("DispatchFilter failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// Each emission should process its filter chain independently
	// Total executions should be 10 * 100 = 1000
	if counter.Load() != 1000 {
		t.Errorf("expected 1000 handler executions, got %d", counter.Load())
	}
}

func TestPriority_NegativePriorities(t *testing.T) {
	eb := events.NewEventBus()

	var executed []string

	handlerA := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "A")
		return event, nil
	}

	handlerB := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "B")
		return event, nil
	}

	handlerC := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "C")
		return event, nil
	}

	// Negative priorities should execute first
	eb.SubscribeFilter("test.event", 0, handlerC)
	eb.SubscribeFilter("test.event", -10, handlerA)
	eb.SubscribeFilter("test.event", 10, handlerB)

	event := &events.Event{Topic: "test.event"}
	_, err := eb.DispatchFilter(context.Background(), event)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	// Order should be A (-10), C (0), B (10)
	if executed[0] != "A" || executed[1] != "C" || executed[2] != "B" {
		t.Errorf("expected order [A, C, B], got %v", executed)
	}
}

func TestPriority_ZeroIsHighest(t *testing.T) {
	eb := events.NewEventBus()

	var executed []string

	handlerA := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "A")
		return event, nil
	}

	handlerB := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "B")
		return event, nil
	}

	eb.SubscribeFilter("test.event", 0, handlerA)
	eb.SubscribeFilter("test.event", 1, handlerB)

	event := &events.Event{Topic: "test.event"}
	_, err := eb.DispatchFilter(context.Background(), event)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	// Priority 0 should execute before priority 1
	if executed[0] != "A" || executed[1] != "B" {
		t.Errorf("expected [A, B], got %v", executed)
	}
}

func TestNegativePriorityBeforeZero(t *testing.T) {
	eb := events.NewEventBus()

	var executed []string

	handlerA := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "A")
		return event, nil
	}

	handlerB := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "B")
		return event, nil
	}

	eb.SubscribeFilter("test.event", -10, handlerA)
	eb.SubscribeFilter("test.event", 0, handlerB)

	event := &events.Event{Topic: "test.event"}
	_, err := eb.DispatchFilter(context.Background(), event)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	// Negative priority should execute before 0
	if executed[0] != "A" || executed[1] != "B" {
		t.Errorf("expected [A, B], got %v", executed)
	}
}

func TestFilterChain_FirstHandlerAborts(t *testing.T) {
	eb := events.NewEventBus()

	errExpected := errors.New("first handler error")

	var executed []string

	handlerA := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "A")
		return nil, errExpected
	}

	handlerB := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		executed = append(executed, "B")
		return event, nil
	}

	eb.SubscribeFilter("test.event", 10, handlerA)
	eb.SubscribeFilter("test.event", 20, handlerB)

	event := &events.Event{Topic: "test.event"}
	_, err := eb.DispatchFilter(context.Background(), event)

	if err != errExpected {
		t.Errorf("expected error from first handler, got %v", err)
	}

	// Only first handler should have executed
	if len(executed) != 1 {
		t.Errorf("expected only 1 handler executed, got %d", len(executed))
	}
	if executed[0] != "A" {
		t.Errorf("expected [A], got %v", executed)
	}
}

func TestFilterChain_PartiallyModifiedEvent(t *testing.T) {
	eb := events.NewEventBus()

	handlerA := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		event.Data["modified"] = "by A"
		return event, nil
	}

	handlerB := func(ctx context.Context, event *events.Event) (*events.Event, error) {
		return nil, errors.New("handler B error")
	}

	eb.SubscribeFilter("test.event", 10, handlerA)
	eb.SubscribeFilter("test.event", 20, handlerB)

	event := &events.Event{Topic: "test.event", Data: map[string]interface{}{}}
	result, err := eb.DispatchFilter(context.Background(), event)

	if err == nil {
		t.Error("expected error from handler B")
	}
	if result != nil {
		t.Error("result should be nil when error occurs")
	}
}

func TestEmptyMethod(t *testing.T) {
	eb := events.NewEventBus()

	// Test with nil handler
	eb.SubscribeFilter("test.event", 10, nil)
	eb.SubscribeBroadcast("test.event", nil)

	event := &events.Event{Topic: "test.event"}
	_, err := eb.DispatchFilter(context.Background(), event)
	if err != nil {
		t.Fatalf("DispatchFilter failed: %v", err)
	}

	eb.Emit(context.Background(), *event)
}
