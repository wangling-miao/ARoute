package license

import (
	"sync"
	"testing"
	"time"
)

func TestValidatorConcurrentAccess(t *testing.T) {
	license := &License{
		ID:        "test-enterprise",
		Tier:      TierEnterprise,
		Features:  []string{"plugin:l2-grpc", "plugin:custom-ddl"},
		IssuedAt:  time.Now(),
		IssuedTo:  "Test Corp",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	v := NewValidator(license, nil)

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = v.Tier()
			_ = v.IsFeatureAllowed("plugin:l2-grpc")
			_ = v.IsExpired()
			_ = v.License()
			_ = v.LicenseInfo()
		}()
	}

	// Concurrent Validate calls (writer)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = v.Validate()
		}()
	}

	// Mixed concurrent reads + writes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%5 == 0 {
				_ = v.Validate()
			} else {
				_ = v.IsFeatureAllowed("plugin:l2-grpc")
				_ = v.Tier()
			}
		}(i)
	}

	wg.Wait()
}
