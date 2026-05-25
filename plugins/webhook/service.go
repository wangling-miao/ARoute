package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type Config struct {
	DeliveryTimeout          time.Duration
	MaxRetries               int
	MaxConsecutiveFailures   int
	DeliveryLogRetentionDays int
}

type Service struct {
	config     Config
	logger     *slog.Logger
	mu         sync.RWMutex
	webhooks   map[string]*interfaces.Webhook
	deliveries map[string][]*interfaces.WebhookDelivery
	httpClient *http.Client
	eventBus   core.EventBus
}

func NewService(cfg Config, logger *slog.Logger) *Service {
	return &Service{
		config:     cfg,
		logger:     logger,
		webhooks:   make(map[string]*interfaces.Webhook),
		deliveries: make(map[string][]*interfaces.WebhookDelivery),
		httpClient: newSafeWebhookClient(cfg.DeliveryTimeout),
	}
}

func newSafeWebhookClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve webhook host %q: %w", host, err)
			}
			for _, resolved := range ips {
				if err := checkIP(resolved.IP); err != nil {
					return nil, err
				}
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no IP addresses found for hostname %q", host)
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return validateWebhookURL(req.URL.String())
		},
	}
}

// SetEventBus sets the EventBus used to emit webhook lifecycle events.
// May be nil — event emission is skipped when no bus is configured.
func (s *Service) SetEventBus(bus core.EventBus) {
	s.eventBus = bus
}

// validateWebhookURL validates that a URL is safe for webhook delivery,
// rejecting private/reserved addresses to prevent SSRF attacks.
func validateWebhookURL(rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s (only http and https are allowed)", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("URL must have a hostname")
	}

	if ip := net.ParseIP(host); ip != nil {
		if err := checkIP(ip); err != nil {
			return err
		}
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname %q: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses found for hostname %q", host)
	}
	for _, ip := range ips {
		if err := checkIP(ip); err != nil {
			return err
		}
	}
	return nil
}

func checkIP(ip net.IP) error {
	if ip.IsLoopback() {
		return fmt.Errorf("webhook URL must not point to a loopback address")
	}
	if ip.IsPrivate() {
		return fmt.Errorf("webhook URL must not point to a private address")
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("webhook URL must not point to a link-local address")
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("webhook URL must not point to an unspecified address")
	}
	return nil
}

func (s *Service) Create(ctx context.Context, rawURL string, events []string, secret string) (*interfaces.Webhook, error) {
	if err := validateWebhookURL(rawURL); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("events must not be empty")
	}
	if len(secret) < 8 {
		return nil, fmt.Errorf("secret must be at least 8 characters")
	}

	now := time.Now()
	wh := &interfaces.Webhook{
		ID:        uuid.Must(uuid.NewV7()).String(),
		URL:       rawURL,
		Events:    events,
		Secret:    secret,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	s.webhooks[wh.ID] = wh
	s.mu.Unlock()

	s.logger.Info("webhook created",
		"id", wh.ID,
		"url", wh.URL,
		"events", wh.Events,
	)
	return wh, nil
}

func (s *Service) Get(ctx context.Context, id string) (*interfaces.Webhook, error) {
	s.mu.RLock()
	wh, ok := s.webhooks[id]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("webhook %q not found", id)
	}
	return wh, nil
}

func (s *Service) List(ctx context.Context) []*interfaces.Webhook {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*interfaces.Webhook, 0, len(s.webhooks))
	for _, wh := range s.webhooks {
		result = append(result, wh)
	}
	return result
}

func (s *Service) Update(ctx context.Context, id string, rawURL string, events []string) (*interfaces.Webhook, error) {
	if err := validateWebhookURL(rawURL); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("events must not be empty")
	}

	s.mu.Lock()
	wh, ok := s.webhooks[id]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("webhook %q not found", id)
	}
	wh.URL = rawURL
	wh.Events = events
	wh.UpdatedAt = time.Now()
	s.mu.Unlock()

	s.logger.Info("webhook updated", "id", id)
	return wh, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	_, ok := s.webhooks[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("webhook %q not found", id)
	}
	delete(s.webhooks, id)
	delete(s.deliveries, id)
	s.mu.Unlock()

	s.logger.Info("webhook deleted", "id", id)
	return nil
}

func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) error {
	s.mu.Lock()
	wh, ok := s.webhooks[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("webhook %q not found", id)
	}
	wh.Enabled = enabled
	wh.UpdatedAt = time.Now()
	if enabled {
		wh.ConsecutiveFailures = 0
		wh.DisabledReason = ""
	}
	s.mu.Unlock()

	s.logger.Info("webhook enabled toggled", "id", id, "enabled", enabled)
	return nil
}

func (s *Service) UpdateSecret(ctx context.Context, id string, secret string) error {
	s.mu.Lock()
	wh, ok := s.webhooks[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("webhook %q not found", id)
	}
	wh.Secret = secret
	wh.UpdatedAt = time.Now()
	s.mu.Unlock()

	s.logger.Info("webhook secret updated", "id", id)
	return nil
}

// HandleEvent satisfies the EventBus BroadcastHandler signature.
func (s *Service) HandleEvent(ctx context.Context, event events.Event) {
	s.mu.RLock()
	var matched []*interfaces.Webhook
	for _, wh := range s.webhooks {
		if !wh.Enabled {
			continue
		}
		for _, pattern := range wh.Events {
			if matchEventPattern(event.Topic, pattern) {
				matched = append(matched, wh)
				break
			}
		}
	}
	s.mu.RUnlock()

	if len(matched) == 0 {
		return
	}

	// Capture receive time once so all webhooks get the same timestamp.
	receivedAt := time.Now()
	var wg sync.WaitGroup
	for _, wh := range matched {
		wg.Add(1)
		go func(w *interfaces.Webhook) {
			defer wg.Done()
			s.deliverWithRetry(ctx, w, event.Topic, event.Data, receivedAt)
		}(wh)
	}
	wg.Wait()
}

func (s *Service) DeliverEvent(ctx context.Context, event interfaces.WebhookEvent) {
	topic := event.Topic
	data := event.Data
	ts := event.Timestamp

	s.mu.RLock()
	var matched []*interfaces.Webhook
	for _, wh := range s.webhooks {
		if !wh.Enabled {
			continue
		}
		for _, pattern := range wh.Events {
			if matchEventPattern(topic, pattern) {
				matched = append(matched, wh)
				break
			}
		}
	}
	s.mu.RUnlock()

	if len(matched) == 0 {
		return
	}

	receivedAt := ts
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}
	var wg sync.WaitGroup
	for _, wh := range matched {
		wg.Add(1)
		go func(w *interfaces.Webhook) {
			defer wg.Done()
			s.deliverWithRetry(ctx, w, topic, data, receivedAt)
		}(wh)
	}
	wg.Wait()
}

func (s *Service) deliverWithRetry(ctx context.Context, wh *interfaces.Webhook, eventType string, data map[string]any, receivedAt time.Time) {
	s.mu.RLock()
	secret := wh.Secret
	s.mu.RUnlock()

	maxAttempts := s.config.MaxRetries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		delivery, ok := s.deliver(ctx, wh, eventType, data, attempt, secret, receivedAt)
		if ok {
			s.mu.Lock()
			wh.ConsecutiveFailures = 0
			wh.UpdatedAt = time.Now()
			s.mu.Unlock()
			return
		}

		isPermanent := delivery.StatusCode >= 400 && delivery.StatusCode < 500 && delivery.StatusCode != 429
		if isPermanent || attempt >= maxAttempts {
			break
		}

		backoff := time.Duration(1<<uint(attempt-1)) * time.Second
		s.logger.Debug("webhook delivery retry scheduled",
			"webhook_id", wh.ID,
			"event", eventType,
			"attempt", attempt,
			"backoff", backoff,
		)

		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}

	s.mu.Lock()
	wh.ConsecutiveFailures++
	wh.UpdatedAt = time.Now()
	autoDisabled := false
	if wh.ConsecutiveFailures >= s.config.MaxConsecutiveFailures {
		wh.Enabled = false
		wh.DisabledReason = "consecutive_failures"
		autoDisabled = true
		s.logger.Warn("webhook auto-disabled due to consecutive failures",
			"webhook_id", wh.ID,
			"consecutive_failures", wh.ConsecutiveFailures,
		)
	}
	s.mu.Unlock()

	if autoDisabled && s.eventBus != nil {
		s.eventBus.Emit(context.Background(), events.Event{
			Topic: "webhook.auto_disabled",
			Data: map[string]interface{}{
				"webhook_id":           wh.ID,
				"url":                  wh.URL,
				"consecutive_failures": wh.ConsecutiveFailures,
				"reason":               "consecutive_failures",
			},
		})
	}
}

func (s *Service) deliver(ctx context.Context, wh *interfaces.Webhook, eventType string, data map[string]any, attempt int, secret string, receivedAt time.Time) (*interfaces.WebhookDelivery, bool) {
	payload := map[string]any{
		"event":     eventType,
		"timestamp": receivedAt.UTC().Format(time.RFC3339),
		"data":      data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		d := s.recordDelivery(wh.ID, eventType, attempt, 0, 0, false, fmt.Sprintf("marshal payload: %v", err))
		return d, false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	reqCtx, cancel := context.WithTimeout(ctx, s.config.DeliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		d := s.recordDelivery(wh.ID, eventType, attempt, 0, 0, false, fmt.Sprintf("create request: %v", err))
		return d, false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", "sha256="+sig)

	start := time.Now()
	resp, err := s.httpClient.Do(req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		d := s.recordDelivery(wh.ID, eventType, attempt, 0, elapsed, false, err.Error())
		return d, false
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	var errMsg string
	if !success {
		errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	d := s.recordDelivery(wh.ID, eventType, attempt, resp.StatusCode, elapsed, success, errMsg)
	return d, success
}

func (s *Service) recordDelivery(webhookID string, event string, attempt int, statusCode int, responseTime int64, success bool, errMsg string) *interfaces.WebhookDelivery {
	d := &interfaces.WebhookDelivery{
		ID:           uuid.Must(uuid.NewV7()).String(),
		WebhookID:    webhookID,
		Event:        event,
		Attempt:      attempt,
		StatusCode:   statusCode,
		ResponseTime: responseTime,
		Success:      success,
		Error:        errMsg,
		CreatedAt:    time.Now(),
	}

	s.mu.Lock()
	s.deliveries[webhookID] = append(s.deliveries[webhookID], d)
	s.mu.Unlock()

	return d
}

func (s *Service) GetDeliveries(ctx context.Context, webhookID string, limit int, offset int) ([]*interfaces.WebhookDelivery, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := s.deliveries[webhookID]
	total := len(all)

	if offset >= total {
		return nil, total
	}

	// Return in reverse chronological order
	end := total - offset
	start := max(end-limit, 0)

	result := make([]*interfaces.WebhookDelivery, 0, end-start)
	for i := end - 1; i >= start; i-- {
		result = append(result, all[i])
	}
	return result, total
}

// PruneOldDeliveries removes delivery log entries older than
// Config.DeliveryLogRetentionDays for all webhooks.
func (s *Service) PruneOldDeliveries(ctx context.Context) {
	retentionDays := s.config.DeliveryLogRetentionDays
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	s.mu.Lock()
	defer s.mu.Unlock()

	for whID, entries := range s.deliveries {
		pruned := make([]*interfaces.WebhookDelivery, 0, len(entries))
		for _, d := range entries {
			if !d.CreatedAt.Before(cutoff) {
				pruned = append(pruned, d)
			}
		}
		if len(pruned) == 0 {
			delete(s.deliveries, whID)
		} else {
			s.deliveries[whID] = pruned
		}
	}
}

func (s *Service) TestDelivery(ctx context.Context, webhookID string) (*interfaces.WebhookDelivery, error) {
	s.mu.RLock()
	wh, ok := s.webhooks[webhookID]
	if !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("webhook %q not found", webhookID)
	}
	secret := wh.Secret
	targetURL := wh.URL
	s.mu.RUnlock()

	payload := map[string]any{
		"event":     "webhook.test",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      map[string]string{"message": "Test delivery"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal test payload: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	reqCtx, cancel := context.WithTimeout(ctx, s.config.DeliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", "sha256="+sig)

	start := time.Now()
	resp, err := s.httpClient.Do(req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		d := s.recordDelivery(webhookID, "webhook.test", 1, 0, elapsed, false, err.Error())
		return d, nil
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	var errMsg string
	if !success {
		errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	d := s.recordDelivery(webhookID, "webhook.test", 1, resp.StatusCode, elapsed, success, errMsg)
	return d, nil
}

func (s *Service) Close() error {
	s.httpClient.CloseIdleConnections()
	s.logger.Info("webhook service closed")
	return nil
}

func matchEventPattern(topic string, pattern string) bool {
	if pattern == "**" {
		return true
	}
	if pattern == topic {
		return true
	}

	topicParts := strings.Split(topic, ".")
	patternParts := strings.Split(pattern, ".")

	return matchEventParts(topicParts, patternParts, 0)
}

func matchEventParts(topicParts, patternParts []string, depth int) bool {
	if depth > 20 {
		return false
	}
	for len(patternParts) > 0 {
		if len(topicParts) == 0 {
			for _, p := range patternParts {
				if p != "**" {
					return false
				}
			}
			return true
		}

		if patternParts[0] == "**" {
			if matchEventParts(topicParts, patternParts[1:], depth+1) {
				return true
			}
			return matchEventParts(topicParts[1:], patternParts, depth+1)
		}

		if patternParts[0] == "*" {
			topicParts = topicParts[1:]
			patternParts = patternParts[1:]
			continue
		}

		if patternParts[0] != topicParts[0] {
			return false
		}

		topicParts = topicParts[1:]
		patternParts = patternParts[1:]
	}

	return len(topicParts) == 0
}
