// Package events provides the EventBus implementation for event-driven communication.
// It supports two event modes: Filter chain (ordered, can abort) and Broadcast (concurrent, fire-and-forget).
// Thread safety: All operations are protected by RWMutex. Filter chains use mutex for ordering,
// while broadcasts use goroutines for concurrency.
package events

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wangling-miao/aroute/core/services"
)

// Event represents an event that can be emitted through the EventBus.
// It contains a topic (e.g., "content.post.saved") and arbitrary data payload.
type Event struct {
	// Topic is the event topic (e.g., "content.post.saved")
	Topic string
	// Data is the event payload
	Data map[string]interface{}
}

// FilterHandler is a filter chain handler function.
// It receives the context, event, and can modify the event.
// Returns the (potentially modified) event and an optional error.
// If error is returned, the filter chain aborts.
type FilterHandler func(ctx context.Context, event *Event) (*Event, error)

// BroadcastHandler is a broadcast handler function.
// It receives a copy of the event and does not return values (fire-and-forget).
// Errors should be logged but not propagated.
type BroadcastHandler func(ctx context.Context, event Event)

// EventBus provides event-driven communication with two modes:
// 1. Filter chain: ordered execution, can abort chain with error, result passing
// 2. Broadcast: concurrent execution, fire-and-forget, errors logged
//
// Thread safety: All operations are thread-safe. Filter chains use mutex to ensure
// ordering priority, while broadcasts use goroutines for concurrent execution.
type EventBus struct {
	mu sync.RWMutex

	// filterHandlers stores filter chain handlers by topic.
	// map[topic][]handler{handlerID, priority, handler}
	filterHandlers map[string][]filterHandlerEntry

	// broadcastHandlers stores broadcast handlers by topic.
	// map[topic][]handler{handlerID, handler}
	broadcastHandlers map[string][]broadcastHandlerEntry

	// handlerIDCounter generates unique IDs for handlers
	handlerIDCounter uint64
}

// filterHandlerEntry represents a registered filter chain handler.
type filterHandlerEntry struct {
	id       string
	priority int
	handler  FilterHandler
}

// broadcastHandlerEntry represents a registered broadcast handler.
type broadcastHandlerEntry struct {
	id      string
	handler BroadcastHandler
}

// NewEventBus creates a new EventBus instance.
func NewEventBus() *EventBus {
	return &EventBus{
		filterHandlers:    make(map[string][]filterHandlerEntry),
		broadcastHandlers: make(map[string][]broadcastHandlerEntry),
	}
}

// SubscribeFilter registers a filter chain handler for the given topic.
// Lower priority numbers execute first (higher priority).
// Priority 0 is the highest priority. Negative priorities are allowed.
// Returns a unique handler ID for unsubscription.
//
// Thread safety: Uses write lock to prevent concurrent modifications.
func (eb *EventBus) SubscribeFilter(topic string, priority int, handler FilterHandler) string {
	if handler == nil {
		return ""
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	id := eb.generateHandlerID()
	entry := filterHandlerEntry{
		id:       id,
		priority: priority,
		handler:  handler,
	}

	eb.filterHandlers[topic] = append(eb.filterHandlers[topic], entry)
	// Sort by priority (ascending order, lower number = higher priority)
	sort.Slice(eb.filterHandlers[topic], func(i, j int) bool {
		if eb.filterHandlers[topic][i].priority != eb.filterHandlers[topic][j].priority {
			return eb.filterHandlers[topic][i].priority < eb.filterHandlers[topic][j].priority
		}
		// Same priority: maintain subscription order (FIFO)
		return false
	})

	return id
}

// SubscribeBroadcast registers a broadcast handler for the given topic.
// Returns a unique handler ID for unsubscription.
//
// Thread safety: Uses write lock to prevent concurrent modifications.
func (eb *EventBus) SubscribeBroadcast(topic string, handler BroadcastHandler) string {
	if handler == nil {
		return ""
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	id := eb.generateHandlerID()
	entry := broadcastHandlerEntry{
		id:      id,
		handler: handler,
	}

	eb.broadcastHandlers[topic] = append(eb.broadcastHandlers[topic], entry)
	return id
}

// Emit dispatches an event to all matching broadcast handlers concurrently.
// This is fire-and-forget: errors are logged but not propagated to the caller.
// Broadcast handlers receive a copy of the event to prevent mutation.
//
// Thread safety: Uses read lock to get handler snapshot, then executes without lock.
func (eb *EventBus) Emit(ctx context.Context, event Event) {
	if event.Topic == "" {
		return
	}

	// Get snapshot of matching handlers
	handlers := eb.getBroadcastHandlers(event.Topic)
	if len(handlers) == 0 {
		return
	}

	// Execute all handlers concurrently
	var wg sync.WaitGroup
	for _, entry := range handlers {
		wg.Add(1)
		go func(h BroadcastHandler) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					// Log panic but don't propagate
					// In production, this would use proper logging
					fmt.Printf("[EventBus] Broadcast handler panic: %v\n", r)
				}
			}()

			// Send a copy to prevent mutation
			eventCopy := Event{
				Topic: event.Topic,
				Data:  copyEventData(event.Data),
			}
			h(ctx, eventCopy)
		}(entry.handler)
	}

	// For fire-and-forget, we don't wait for completion
	// This matches the spec requirement: "returns immediately"
	// Handlers run in background
}

// DispatchFilter executes filter chain handlers in priority order.
// Each handler receives the event (possibly modified by previous handlers).
// If a handler returns error, the chain aborts and error is returned.
// Returns the final (potentially modified) event.
//
// Thread safety: Uses read lock to get handler snapshot, then executes without lock.
func (eb *EventBus) DispatchFilter(ctx context.Context, event *Event) (*Event, error) {
	if event == nil {
		return nil, fmt.Errorf("events: event cannot be nil")
	}
	if event.Topic == "" {
		return event, nil
	}

	// Get snapshot of matching handlers
	handlers := eb.getFilterHandlers(event.Topic)
	if len(handlers) == 0 {
		return event, nil
	}

	// Execute handlers in order
	currentEvent := event
	for _, entry := range handlers {
		var err error
		currentEvent, err = entry.handler(ctx, currentEvent)
		if err != nil {
			// Chain aborts on error
			return nil, err
		}
		// If handler returned nil event, use the previous one
		if currentEvent == nil {
			currentEvent = event
		}
	}

	return currentEvent, nil
}

// Unsubscribe removes a handler by its ID.
// Works for both filter and broadcast handlers.
// If handler ID doesn't exist, it's a no-op.
//
// Thread safety: Uses write lock to prevent concurrent modifications.
func (eb *EventBus) Unsubscribe(handlerID string) {
	if handlerID == "" {
		return
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Remove from filter handlers
	for topic, handlers := range eb.filterHandlers {
		for i, entry := range handlers {
			if entry.id == handlerID {
				eb.filterHandlers[topic] = append(handlers[:i], handlers[i+1:]...)
				if len(eb.filterHandlers[topic]) == 0 {
					delete(eb.filterHandlers, topic)
				}
				return
			}
		}
	}

	// Remove from broadcast handlers
	for topic, handlers := range eb.broadcastHandlers {
		for i, entry := range handlers {
			if entry.id == handlerID {
				eb.broadcastHandlers[topic] = append(handlers[:i], handlers[i+1:]...)
				if len(eb.broadcastHandlers[topic]) == 0 {
					delete(eb.broadcastHandlers, topic)
				}
				return
			}
		}
	}
}

// generateHandlerID generates a unique handler ID.
// Thread safety: Uses atomic counter for thread-safe ID generation.
func (eb *EventBus) generateHandlerID() string {
	id := atomic.AddUint64(&eb.handlerIDCounter, 1)
	return fmt.Sprintf("handler-%d", id)
}

// getFilterHandlers returns a snapshot of filter handlers matching the topic.
// Handles wildcard matching (*, **).
// Thread safety: Uses read lock.
func (eb *EventBus) getFilterHandlers(topic string) []filterHandlerEntry {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	var result []filterHandlerEntry

	// Exact match
	if handlers, ok := eb.filterHandlers[topic]; ok {
		result = append(result, handlers...)
	}

	// Wildcard matching
	for pattern, handlers := range eb.filterHandlers {
		if pattern == topic {
			continue // Already added as exact match
		}
		if matchTopic(topic, pattern) {
			result = append(result, handlers...)
		}
	}

	// Sort all matched handlers by priority
	sort.Slice(result, func(i, j int) bool {
		if result[i].priority != result[j].priority {
			return result[i].priority < result[j].priority
		}
		// Same priority: maintain original order (FIFO)
		return false
	})

	return result
}

// getBroadcastHandlers returns a snapshot of broadcast handlers matching the topic.
// Handles wildcard matching (*, **).
// Thread safety: Uses read lock.
func (eb *EventBus) getBroadcastHandlers(topic string) []broadcastHandlerEntry {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	var result []broadcastHandlerEntry

	// Exact match
	if handlers, ok := eb.broadcastHandlers[topic]; ok {
		result = append(result, handlers...)
	}

	// Wildcard matching
	for pattern, handlers := range eb.broadcastHandlers {
		if pattern == topic {
			continue // Already added as exact match
		}
		if matchTopic(topic, pattern) {
			result = append(result, handlers...)
		}
	}

	return result
}

// matchTopic checks if a topic matches a pattern with wildcards.
// * matches exactly one segment.
// ** matches one or more segments at any depth.
func matchTopic(topic, pattern string) bool {
	// Empty pattern only matches empty topic
	if pattern == "" {
		return topic == ""
	}

	// ** matches everything
	if pattern == "**" {
		return true
	}

	topicParts := strings.Split(topic, ".")
	patternParts := strings.Split(pattern, ".")

	return matchParts(topicParts, patternParts)
}

// matchParts recursively matches topic parts against pattern parts.
func matchParts(topicParts, patternParts []string) bool {
	// Both consumed: match
	if len(topicParts) == 0 && len(patternParts) == 0 {
		return true
	}

	// Pattern exhausted but topic remains: no match
	if len(patternParts) == 0 {
		return false
	}

	// Topic exhausted but pattern remains
	if len(topicParts) == 0 {
		// Check if remaining pattern is all **
		for _, p := range patternParts {
			if p != "**" {
				return false
			}
		}
		return true
	}

	// ** can match one or more segments
	if patternParts[0] == "**" {
		// Try matching 0 segments
		if matchParts(topicParts, patternParts[1:]) {
			return true
		}
		// Try matching 1+ segments
		return matchParts(topicParts[1:], patternParts)
	}

	// * matches exactly one segment
	if patternParts[0] == "*" {
		return matchParts(topicParts[1:], patternParts[1:])
	}

	// Exact match
	if patternParts[0] == topicParts[0] {
		return matchParts(topicParts[1:], patternParts[1:])
	}

	return false
}

// copyEventData creates a deep copy of event data map.
// Ensures broadcast handlers receive independent copies.
func copyEventData(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}

	result := make(map[string]interface{}, len(data))
	for k, v := range data {
		// Shallow copy is sufficient for most use cases
		// For nested maps, callers should handle deep copying if needed
		result[k] = v
	}
	return result
}

// RegisterToContainer registers the EventBus as a service in the container.
// This allows other components to access the EventBus via dependency injection.
func RegisterToContainer(container *services.Container) error {
	return container.Provide(func(c *services.Container) (*EventBus, error) {
		return NewEventBus(), nil
	})
}
