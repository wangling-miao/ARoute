// Package example demonstrates how to build an Aroute CMS plugin using the SDK.
//
// This plugin showcases the three core patterns:
//   - Service access: retrieving the DatabaseService via SDK helpers
//   - Event subscription: listening to content.created events
//   - Route registration: adding custom HTTP endpoints
//
// To build:
//
//	go build -o example-plugin .
//
// See README.md for detailed usage instructions.
package example

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	sdk "github.com/wangling-miao/aroute/sdk/go"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// ExamplePlugin demonstrates a complete Aroute CMS plugin built with the SDK.
// It embeds sdk.BasePlugin for default lifecycle behavior and overrides
// Init to register services, subscribe to events, and add custom routes.
type ExamplePlugin struct {
	*sdk.BasePlugin

	// db holds a reference to the database service (set during Init).
	db interfaces.DatabaseService

	// handlerID stores the event subscription ID for cleanup during Stop.
	handlerID string
}

// New creates a new ExamplePlugin instance.
// It loads the plugin manifest from the embedded manifest data.
func New() *ExamplePlugin {
	return &ExamplePlugin{
		BasePlugin: sdk.MustNewBasePlugin("example-plugin", "1.0.0"),
	}
}

// Init initializes the plugin by:
//  1. Storing the Core context
//  2. Obtaining the DatabaseService from the service container
//  3. Subscribing to content.created events on the EventBus
//  4. Registering a custom HTTP route at /api/plugins/example/hello
//
// This method overrides the BasePlugin no-op Init.
func (p *ExamplePlugin) Init(ctx core.CoreContext) error {
	// Call parent Init to store the context
	if err := p.BasePlugin.Init(ctx); err != nil {
		return fmt.Errorf("example-plugin: base init failed: %w", err)
	}

	logger := ctx.Logger()
	logger.Info("Initializing example plugin")

	// Pattern 1: Access a service from the container
	db, err := sdk.GetDB(ctx.Services())
	if err != nil {
		logger.Warn("Database service not available, running without database", "error", err)
	} else {
		p.db = db
		logger.Info("Database service connected")
	}

	// Pattern 2: Subscribe to events on the EventBus
	p.handlerID = sdk.SubscribeEvent(ctx, "content.*.created", func(ctx context.Context, event events.Event) {
		logger.Info("Content created event received",
			"topic", event.Topic,
			"content_type", event.Data["content_type"],
			"id", event.Data["id"],
		)
	})
	logger.Info("Subscribed to content creation events", "handlerID", p.handlerID)

	// Pattern 3: Register custom HTTP routes
	router, err := sdk.GetRouter(ctx.Services())
	if err != nil {
		logger.Warn("Route registrar not available, skipping route registration", "error", err)
	} else {
		router.HandleFunc("/api/plugins/example/hello", p.handleHello)
			router.HandleFunc("/api/plugins/example/info", p.handleInfo)
		logger.Info("Registered custom routes at /api/plugins/example/*")
	}

	return nil
}

// Start is called after all plugins are initialized.
// Use this for startup logic that depends on other plugins being ready.
func (p *ExamplePlugin) Start() error {
	if p.Logger() != nil {
		p.Logger().Info("Example plugin started", "sdk_version", sdk.Version())
	}
	return nil
}

// Stop cleans up the plugin by unsubscribing from events.
// Called during graceful shutdown in reverse dependency order.
func (p *ExamplePlugin) Stop() error {
	if p.Context() != nil && p.handlerID != "" {
		p.Context().Events().Unsubscribe(p.handlerID)
	}
	if p.Logger() != nil {
		p.Logger().Info("Example plugin stopped")
	}
	return nil
}

// handleHello returns a greeting message.
// Demonstrates a simple custom API endpoint.
func (p *ExamplePlugin) handleHello(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Hello from example plugin",
		"plugin":  p.Name(),
		"version": p.Version(),
	})
}

// handleInfo returns plugin and SDK metadata.
// Demonstrates accessing SDK version and plugin manifest data.
func (p *ExamplePlugin) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	info := map[string]interface{}{
		"plugin": map[string]string{
			"name":        p.Name(),
			"version":     p.Version(),
			"description": p.Description(),
			"author":      p.Author(),
		},
		"sdk_version": sdk.Version(),
	}
	json.NewEncoder(w).Encode(info)
}

// Compile-time interface check.
var _ core.Plugin = (*ExamplePlugin)(nil)
