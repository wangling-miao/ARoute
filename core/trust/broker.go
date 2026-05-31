package trust

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/wangling-miao/aroute/core/events"
)

type EventEmitter interface {
	Emit(ctx context.Context, event events.Event)
}

type RegistryStore interface {
	ApplyTrustDecision(plugin string, decision Decision) error
}

type Service interface {
	RegisterGrant(grant Grant)
	Grant(plugin string) (Grant, bool)
	Authorize(ctx context.Context, req AuthorizationRequest) AuthorizationResult
	RecordRisk(ctx context.Context, plugin string, event RiskEvent) Decision
	RecordDecision(ctx context.Context, decision Decision) Decision
	History(plugin string) ([]Decision, error)
}

type Broker struct {
	mu        sync.RWMutex
	grants    map[string]Grant
	ledger    Ledger
	evaluator *Evaluator
	events    EventEmitter
	store     RegistryStore
	StrictL1  bool
}

func NewBroker(ledger Ledger, evaluator *Evaluator, emitter EventEmitter, store RegistryStore) *Broker {
	if ledger == nil {
		ledger = NewMemoryLedger()
	}
	if evaluator == nil {
		evaluator = NewEvaluator()
	}
	return &Broker{
		grants:    make(map[string]Grant),
		ledger:    ledger,
		evaluator: evaluator,
		events:    emitter,
		store:     store,
	}
}

func (b *Broker) RegisterGrant(grant Grant) {
	if grant.Plugin == "" {
		return
	}
	if grant.TrustLevel == "" {
		grant.TrustLevel = NormalizeTrustLevel(grant.Engine, "")
	}
	b.mu.Lock()
	b.grants[grant.Plugin] = grant
	b.mu.Unlock()
}

func (b *Broker) Grant(plugin string) (Grant, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	grant, ok := b.grants[plugin]
	return grant, ok
}

func (b *Broker) Authorize(ctx context.Context, req AuthorizationRequest) AuthorizationResult {
	grant, ok := b.Grant(req.Plugin)
	if !ok {
		grant = Grant{
			Plugin:     req.Plugin,
			Engine:     req.Engine,
			TrustLevel: NormalizeTrustLevel(req.Engine, ""),
		}
	}
	if req.Engine == "" {
		req.Engine = grant.Engine
	}

	if !b.StrictL1 && (req.Engine == "native" || req.Engine == "l1") {
		decision := b.record(ctx, StaticEvidence{
			Plugin:         req.Plugin,
			Engine:         req.Engine,
			DeclaredTrust:  "L1",
			EffectiveTrust: "L1",
		}, nil, "l1 audit allow")
		return AuthorizationResult{Allowed: true, Decision: decision}
	}

	if hasCapability(grant.Capabilities, req.Capability) {
		decision := b.record(ctx, StaticEvidence{
			Plugin:         req.Plugin,
			Engine:         req.Engine,
			DeclaredTrust:  grant.TrustLevel,
			EffectiveTrust: grant.TrustLevel,
			Capabilities:   grant.Capabilities,
		}, nil, "capability allow")
		return AuthorizationResult{Allowed: true, Decision: decision}
	}

	event := RiskEvent{
		Plugin:     req.Plugin,
		Type:       EventCapabilityDenied,
		Severity:   35,
		Message:    "capability denied: " + req.Capability,
		Capability: req.Capability,
		Metadata:   req.Metadata,
	}
	decision := b.record(ctx, StaticEvidence{
		Plugin:         req.Plugin,
		Engine:         req.Engine,
		DeclaredTrust:  grant.TrustLevel,
		EffectiveTrust: grant.TrustLevel,
		Capabilities:   grant.Capabilities,
	}, []RiskEvent{event}, "capability denied")
	return AuthorizationResult{Allowed: false, Decision: decision}
}

func (b *Broker) RecordRisk(ctx context.Context, plugin string, event RiskEvent) Decision {
	grant, _ := b.Grant(plugin)
	if event.Plugin == "" {
		event.Plugin = plugin
	}
	return b.record(ctx, StaticEvidence{
		Plugin:         plugin,
		Engine:         grant.Engine,
		DeclaredTrust:  grant.TrustLevel,
		EffectiveTrust: grant.TrustLevel,
		Capabilities:   grant.Capabilities,
	}, []RiskEvent{event}, event.Message)
}

func (b *Broker) RecordDecision(ctx context.Context, decision Decision) Decision {
	if decision.Plugin == "" {
		return decision
	}
	if decision.PolicyRevision == "" {
		decision.PolicyRevision = b.evaluator.PolicyRevision
	}
	if decision.At.IsZero() {
		decision.At = time.Now()
	}
	if b.ledger != nil {
		_ = b.ledger.Append(decision)
	}
	if b.store != nil {
		_ = b.store.ApplyTrustDecision(decision.Plugin, decision)
	}
	if b.events != nil {
		b.events.Emit(ctx, eventsPkg(decision))
	}
	return decision
}

func (b *Broker) History(plugin string) ([]Decision, error) {
	if b == nil || b.ledger == nil {
		return []Decision{}, nil
	}
	return b.ledger.List(plugin)
}

func (b *Broker) record(ctx context.Context, static StaticEvidence, events []RiskEvent, fallbackReason string) Decision {
	decision := b.evaluator.Evaluate(static, events)
	if decision.Reason == "policy allow" && fallbackReason != "" {
		decision.Reason = fallbackReason
	}
	if b.ledger != nil {
		_ = b.ledger.Append(decision)
	}
	if b.store != nil {
		_ = b.store.ApplyTrustDecision(decision.Plugin, decision)
	}
	if b.events != nil {
		b.events.Emit(ctx, eventsPkg(decision))
	}
	return decision
}

func eventsPkg(decision Decision) events.Event {
	return events.Event{
		Topic: "trust.plugin." + decision.Plugin + ".decision",
		Data: map[string]interface{}{
			"plugin":          decision.Plugin,
			"action":          string(decision.Action),
			"state":           string(decision.State),
			"risk_score":      decision.RiskScore,
			"reason":          decision.Reason,
			"policy_revision": decision.PolicyRevision,
			"at":              decision.At,
		},
	}
}

func hasCapability(grants []string, capability string) bool {
	for _, grant := range grants {
		if grant == capability || grant == "*" {
			return true
		}
		if strings.HasSuffix(grant, "*") && strings.HasPrefix(capability, strings.TrimSuffix(grant, "*")) {
			return true
		}
	}
	return false
}
