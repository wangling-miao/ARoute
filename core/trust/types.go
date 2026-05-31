package trust

import "time"

type State string

const (
	StateAllow         State = "allow"
	StateGuarded       State = "guarded"
	StatePendingReview State = "pending_review"
	StateQuarantined   State = "quarantined"
	StateDisabled      State = "disabled"
)

type Action string

const (
	ActionAllow      Action = "allow"
	ActionGuard      Action = "guard"
	ActionReview     Action = "review"
	ActionQuarantine Action = "quarantine"
	ActionDisable    Action = "disable"
	ActionDeny       Action = "deny"
)

type RiskEventType string

const (
	EventCapabilityDenied   RiskEventType = "capability_denied"
	EventSignatureMismatch  RiskEventType = "signature_mismatch"
	EventDigestMismatch     RiskEventType = "digest_mismatch"
	EventL2ProcessExit      RiskEventType = "l2_process_exit"
	EventWasmTimeout        RiskEventType = "wasm_timeout"
	EventEventFlood         RiskEventType = "event_flood"
	EventDependencyRisk     RiskEventType = "dependency_risk"
	EventCapabilityExpanded RiskEventType = "capability_expanded"
	EventMetricAnomaly      RiskEventType = "metric_anomaly"
)

type RiskEvent struct {
	Plugin     string            `json:"plugin"`
	Type       RiskEventType     `json:"type"`
	Severity   int               `json:"severity"`
	Message    string            `json:"message,omitempty"`
	Capability string            `json:"capability,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	At         time.Time         `json:"at"`
}

type StaticEvidence struct {
	Plugin             string   `json:"plugin"`
	Engine             string   `json:"engine"`
	DeclaredTrust      string   `json:"declared_trust"`
	EffectiveTrust     string   `json:"effective_trust"`
	Capabilities       []string `json:"capabilities,omitempty"`
	DigestMismatch     bool     `json:"digest_mismatch,omitempty"`
	SignatureMismatch  bool     `json:"signature_mismatch,omitempty"`
	CapabilityExpanded bool     `json:"capability_expanded,omitempty"`
}

type Decision struct {
	Plugin         string      `json:"plugin"`
	Action         Action      `json:"action"`
	State          State       `json:"state"`
	RiskScore      int         `json:"risk_score"`
	Reason         string      `json:"reason"`
	Events         []RiskEvent `json:"events,omitempty"`
	PolicyRevision string      `json:"policy_revision"`
	At             time.Time   `json:"at"`
}

type Grant struct {
	Plugin       string   `json:"plugin"`
	Engine       string   `json:"engine"`
	TrustLevel   string   `json:"trust_level"`
	Capabilities []string `json:"capabilities"`
}

type AuthorizationRequest struct {
	Plugin     string            `json:"plugin"`
	Engine     string            `json:"engine"`
	Capability string            `json:"capability"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type AuthorizationResult struct {
	Allowed  bool     `json:"allowed"`
	Decision Decision `json:"decision"`
}
