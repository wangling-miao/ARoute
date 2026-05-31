package trust

import (
	"context"
	"testing"
)

func TestEvaluatorThresholds(t *testing.T) {
	evaluator := NewEvaluator()

	allow := evaluator.Evaluate(StaticEvidence{Plugin: "p"}, nil)
	if allow.Action != ActionAllow || allow.State != StateAllow {
		t.Fatalf("allow decision = %s/%s", allow.Action, allow.State)
	}

	guard := evaluator.Evaluate(StaticEvidence{Plugin: "p"}, []RiskEvent{{Severity: 35, Message: "denied"}})
	if guard.Action != ActionGuard || guard.State != StateGuarded {
		t.Fatalf("guard decision = %s/%s", guard.Action, guard.State)
	}

	quarantine := evaluator.Evaluate(StaticEvidence{Plugin: "p"}, []RiskEvent{{Severity: 65, Message: "exit"}})
	if quarantine.Action != ActionQuarantine || quarantine.State != StateQuarantined {
		t.Fatalf("quarantine decision = %s/%s", quarantine.Action, quarantine.State)
	}

	disabled := evaluator.Evaluate(StaticEvidence{Plugin: "p"}, []RiskEvent{{Severity: 90, Message: "tamper"}})
	if disabled.Action != ActionDisable || disabled.State != StateDisabled {
		t.Fatalf("disable decision = %s/%s", disabled.Action, disabled.State)
	}
}

func TestEvaluatorCapabilityExpansionForcesReview(t *testing.T) {
	decision := NewEvaluator().Evaluate(StaticEvidence{
		Plugin:             "p",
		CapabilityExpanded: true,
	}, nil)
	if decision.Action != ActionReview || decision.State != StatePendingReview {
		t.Fatalf("decision = %s/%s", decision.Action, decision.State)
	}
}

func TestBrokerAuthorize(t *testing.T) {
	ledger := NewMemoryLedger()
	broker := NewBroker(ledger, NewEvaluator(), nil, nil)
	broker.RegisterGrant(Grant{
		Plugin:       "wasm-plugin",
		Engine:       "wasm",
		TrustLevel:   "L3",
		Capabilities: []string{"event:publish:content.*"},
	})

	allowed := broker.Authorize(context.Background(), AuthorizationRequest{
		Plugin:     "wasm-plugin",
		Engine:     "wasm",
		Capability: "event:publish:content.created",
	})
	if !allowed.Allowed {
		t.Fatalf("expected authorization to be allowed")
	}

	denied := broker.Authorize(context.Background(), AuthorizationRequest{
		Plugin:     "wasm-plugin",
		Engine:     "wasm",
		Capability: "service:content.write",
	})
	if denied.Allowed {
		t.Fatalf("expected authorization to be denied")
	}
	if denied.Decision.State != StateGuarded {
		t.Fatalf("denied state = %s, want guarded", denied.Decision.State)
	}

	history, err := ledger.List("wasm-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
}
