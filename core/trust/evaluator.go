package trust

import (
	"fmt"
	"strings"
	"time"
)

type Thresholds struct {
	Guard      int
	Quarantine int
	Disable    int
}

type Evaluator struct {
	PolicyRevision string
	Thresholds     Thresholds
}

func NewEvaluator() *Evaluator {
	return &Evaluator{
		PolicyRevision: "builtin:v1",
		Thresholds: Thresholds{
			Guard:      30,
			Quarantine: 60,
			Disable:    80,
		},
	}
}

func (e *Evaluator) Evaluate(static StaticEvidence, events []RiskEvent) Decision {
	if e == nil {
		e = NewEvaluator()
	}

	score := 0
	var reasons []string

	if static.DigestMismatch {
		score += 55
		reasons = append(reasons, "digest mismatch")
		events = append(events, RiskEvent{
			Plugin:   static.Plugin,
			Type:     EventDigestMismatch,
			Severity: 55,
			Message:  "declared digest does not match plugin artifact",
			At:       time.Now(),
		})
	}
	if static.SignatureMismatch {
		score += 55
		reasons = append(reasons, "signature mismatch")
		events = append(events, RiskEvent{
			Plugin:   static.Plugin,
			Type:     EventSignatureMismatch,
			Severity: 55,
			Message:  "signature verification failed",
			At:       time.Now(),
		})
	}
	if static.CapabilityExpanded {
		score += 35
		reasons = append(reasons, "capability expansion requires review")
		events = append(events, RiskEvent{
			Plugin:   static.Plugin,
			Type:     EventCapabilityExpanded,
			Severity: 0,
			Message:  "hot-swap requested additional capabilities",
			At:       time.Now(),
		})
	}

	for i := range events {
		if events[i].At.IsZero() {
			events[i].At = time.Now()
		}
		score += events[i].Severity
		if events[i].Message != "" {
			reasons = append(reasons, events[i].Message)
		}
	}

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	action, state := e.actionFor(score)
	if static.CapabilityExpanded && score < e.Thresholds.Quarantine {
		action = ActionReview
		state = StatePendingReview
	}

	reason := "policy allow"
	if len(reasons) > 0 {
		reason = strings.Join(dedupe(reasons), "; ")
	}

	return Decision{
		Plugin:         static.Plugin,
		Action:         action,
		State:          state,
		RiskScore:      score,
		Reason:         reason,
		Events:         events,
		PolicyRevision: e.PolicyRevision,
		At:             time.Now(),
	}
}

func (e *Evaluator) actionFor(score int) (Action, State) {
	switch {
	case score >= e.Thresholds.Disable:
		return ActionDisable, StateDisabled
	case score >= e.Thresholds.Quarantine:
		return ActionQuarantine, StateQuarantined
	case score >= e.Thresholds.Guard:
		return ActionGuard, StateGuarded
	default:
		return ActionAllow, StateAllow
	}
}

func dedupe(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func NormalizeTrustLevel(engine, declared string) string {
	switch strings.ToUpper(declared) {
	case "L1", "L2", "L3":
		return strings.ToUpper(declared)
	}
	switch engine {
	case "grpc", "l2":
		return "L2"
	case "wasm", "l3":
		return "L3"
	default:
		return "L1"
	}
}

func ValidateStateTransition(from, to State) error {
	if from == "" || from == to {
		return nil
	}
	switch from {
	case StateAllow:
		return nil
	case StateGuarded:
		if to == StateAllow || to == StatePendingReview {
			return nil
		}
	case StatePendingReview:
		if to == StateAllow || to == StateGuarded || to == StateQuarantined {
			return nil
		}
	case StateQuarantined:
		if to == StateGuarded || to == StateDisabled {
			return nil
		}
	case StateDisabled:
		if to == StatePendingReview {
			return nil
		}
	}
	return fmt.Errorf("trust: invalid state transition from %s to %s", from, to)
}
