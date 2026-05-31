package registry

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/wangling-miao/aroute/core/trust"
	bolt "go.etcd.io/bbolt"
)

// ApplyTrustDecision persists the latest policy decision for a plugin route.
func (r *BoltUnifiedRegistry) ApplyTrustDecision(plugin string, decision trust.Decision) error {
	if plugin == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrUnifiedRegistryClosed
	}

	ref := RouteRef{Type: RouteTypePlugin, Domain: defaultDomain, Name: plugin}
	return r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(routeBucketName(RouteTypePlugin)))
		if bucket == nil {
			return &RouteError{RouteRef: ref, Op: "trust", Msg: "route not found"}
		}

		key := routeKey(defaultDomain, plugin)
		data := bucket.Get(key)
		if data == nil {
			return &RouteError{RouteRef: ref, Op: "trust", Msg: "route not found"}
		}

		var route Route
		if err := json.Unmarshal(data, &route); err != nil {
			return fmt.Errorf("deserialize route: %w", err)
		}

		route.RiskScore = decision.RiskScore
		route.TrustState = TrustState(decision.State)
		route.State = routeStateFromTrust(decision.State, route.State)
		route.LastDecision = &TrustDecision{
			Action:         string(decision.Action),
			Reason:         decision.Reason,
			RiskScore:      decision.RiskScore,
			PolicyRevision: decision.PolicyRevision,
			At:             decision.At,
		}
		route.PolicyRevision = decision.PolicyRevision
		route.UpdatedAt = time.Now()
		if decision.State == trust.StateDisabled {
			route.Enabled = false
		}

		newData, err := json.Marshal(&route)
		if err != nil {
			return fmt.Errorf("serialize route: %w", err)
		}
		return bucket.Put(key, newData)
	})
}

func routeStateFromTrust(state trust.State, previous RouteState) RouteState {
	switch state {
	case trust.StateGuarded:
		return RouteStateGuarded
	case trust.StatePendingReview:
		return RouteStatePendingReview
	case trust.StateQuarantined:
		return RouteStateQuarantined
	case trust.StateDisabled:
		return RouteStateInactive
	default:
		if previous == RouteStateGuarded || previous == RouteStatePendingReview || previous == RouteStateQuarantined {
			return RouteStateActive
		}
		return previous
	}
}
