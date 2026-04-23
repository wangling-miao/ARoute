package registry

import "fmt"

// checkTrustLevel enforces trust boundary rules:
//   - L1 (Native) can only consume L1 providers
//   - L2 (gRPC) can consume L1 and L2 providers
//   - L3 (WASM) can consume L1, L2, and L3 providers
//
// The key invariant: higher isolation can depend on lower isolation,
// but lower isolation cannot depend on higher isolation.
// This prevents trusted code from becoming dependent on untrusted code.
func checkTrustLevel(consumer, provider TrustLevel) error {
	// TrustLevel values: L1=0 (highest trust), L2=1, L3=2 (lowest trust)
	// Rule: more trusted code cannot depend on less trusted code.
	// consumer < provider means consumer is MORE trusted than provider → violation.
	if consumer < provider {
		return fmt.Errorf(
			"trust boundary violation: %s consumer cannot depend on %s provider (trusted code cannot depend on less trusted code)",
			consumer, provider,
		)
	}
	return nil
}
