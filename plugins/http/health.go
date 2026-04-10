// Package http provides health check endpoint functionality.
// It implements a /healthz endpoint for monitoring and load balancer checks.
package http

import (
	"encoding/json"
	"net/http"
	"time"
)

// HealthStatus represents the health check response.
type HealthStatus struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

// setupHealthCheck registers the /healthz endpoint.
// It returns a JSON response with status, timestamp, and version.
func (p *Plugin) setupHealthCheck() {
	p.router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		status := HealthStatus{
			Status:    "healthy",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Version:   p.Version(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(status)
	})
}
