package interfaces

import (
	"context"
	"time"
)

// WebhookService defines webhook management and event delivery operations.
// It provides CRUD for webhook registrations, event-driven HTTP delivery
// with HMAC-SHA256 signing, retry with exponential backoff, and delivery logging.
type WebhookService interface {
	// Create registers a new webhook endpoint.
	// Returns the created webhook with a unique ID.
	Create(url string, events []string, secret string) (*Webhook, error)

	// Get retrieves a webhook by ID.
	// Returns an error if the webhook is not found.
	Get(id string) (*Webhook, error)

	// List returns all registered webhooks.
	List() []*Webhook

	// Update modifies a webhook's URL and event subscriptions.
	Update(id string, url string, events []string) (*Webhook, error)

	// Delete removes a webhook and its delivery history.
	Delete(id string) error

	// SetEnabled enables or disables a webhook.
	// Re-enabling resets the consecutive failure counter.
	SetEnabled(id string, enabled bool) error

	// UpdateSecret rotates the webhook's shared secret for HMAC signing.
	// In-flight deliveries continue using the old secret.
	UpdateSecret(id string, secret string) error

	// DeliverEvent delivers an event to all matching, enabled webhooks.
	DeliverEvent(ctx context.Context, event WebhookEvent)

	// GetDeliveries returns paginated delivery logs for a webhook.
	// Results are in reverse chronological order.
	// Returns the deliveries and total count for pagination.
	GetDeliveries(webhookID string, limit int, offset int) ([]*WebhookDelivery, int)

	// TestDelivery sends a test payload to a webhook endpoint.
	// Works even for disabled webhooks.
	TestDelivery(ctx context.Context, webhookID string) (*WebhookDelivery, error)

	// PruneOldDeliveries removes delivery logs older than the retention period.
	PruneOldDeliveries()

	// Close shuts down the service, closing idle HTTP connections.
	Close() error
}

// Webhook represents a registered webhook endpoint.
type Webhook struct {
	// ID is the unique webhook identifier.
	ID string `json:"id"`

	// URL is the target endpoint URL for HTTP POST delivery.
	URL string `json:"url"`

	// Events is the list of event patterns this webhook subscribes to.
	// Supports wildcards: "content.*", "content.**", "**".
	Events []string `json:"events"`

	// Secret is the shared secret for HMAC-SHA256 payload signing.
	// Not returned in API responses.
	Secret string `json:"-"`

	// Enabled indicates whether this webhook is active.
	Enabled bool `json:"enabled"`

	// ConsecutiveFailures is the count of consecutive delivery failures.
	ConsecutiveFailures int `json:"consecutive_failures"`

	// DisabledReason describes why the webhook was auto-disabled.
	// Empty string if not disabled or manually disabled.
	DisabledReason string `json:"disabled_reason,omitempty"`

	// CreatedAt is when the webhook was registered.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the webhook was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// WebhookDelivery represents a single delivery attempt log entry.
type WebhookDelivery struct {
	// ID is the unique delivery log identifier.
	ID string `json:"id"`

	// WebhookID is the webhook this delivery belongs to.
	WebhookID string `json:"webhook_id"`

	// Event is the event type that triggered this delivery.
	Event string `json:"event"`

	// Attempt is the attempt number (1 = initial, 2+ = retries).
	Attempt int `json:"attempt"`

	// StatusCode is the HTTP response status code (0 for network errors).
	StatusCode int `json:"status_code"`

	// ResponseTime is the round-trip time in milliseconds.
	ResponseTime int64 `json:"response_time"`

	// Success indicates whether the delivery was successful (2xx response).
	Success bool `json:"success"`

	// Error contains the error description for failed deliveries.
	Error string `json:"error,omitempty"`

	// CreatedAt is when this delivery attempt was made.
	CreatedAt time.Time `json:"created_at"`
}

// WebhookEvent represents an event to be delivered via webhook.
// This mirrors the EventBus Event structure for interface decoupling.
type WebhookEvent struct {
	// Topic is the event type (e.g., "content.created").
	Topic string

	// Timestamp is when the event was originally published.
	// If zero, the delivery time is used as fallback.
	Timestamp time.Time

	// Data is the event payload.
	Data map[string]any
}
