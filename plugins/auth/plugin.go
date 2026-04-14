// Package auth provides the authentication and authorization plugin for Aroute CMS.
// It implements JWT-based authentication, RBAC permissions, API token management,
// and rate limiting for authentication endpoints.
package auth

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

// authConfig holds the configuration for the auth plugin.
type authConfig struct {
	jwtSecret           string
	jwtAlgorithm        string
	jwtPrivateKeyPath   string // PEM file path for RS256/RS512 private key.
	jwtPublicKeyPath    string // PEM file path for RS256/RS512 public key.
	accessTokenTTL      time.Duration
	refreshTokenTTL     time.Duration
	rotateRefreshTokens bool
	rateLimitAttempts   int
	rateLimitWindow     time.Duration
	bcryptCost          int
	adminEmail          string
	adminPassword       string
}

// Plugin implements the core.Plugin interface for authentication functionality.
type Plugin struct {
	*core.BasePlugin

	mu        sync.RWMutex
	ctx       core.CoreContext
	service   *Service
	rateLimit *RateLimiter
	running   bool
}

// New creates a new auth plugin instance.
func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("auth plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

// Init initializes the auth plugin.
// It retrieves the DatabaseService, creates tables, initializes JWT/RBAC/rate-limiting,
// seeds default roles and the admin user, and registers AuthService.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()
	config := ctx.Config()

	logger.Info("Initializing auth plugin")

	// 1. Get DatabaseService from ServiceContainer.
	var dbSvc interfaces.DatabaseService
	if err := ctx.Services().Get(&dbSvc); err != nil {
		return fmt.Errorf("database service not available: %w", err)
	}

	// 2. Create Store and database tables.
	store := NewStore(dbSvc)
	if err := store.CreateTables(ctx.Context()); err != nil {
		return fmt.Errorf("create auth tables: %w", err)
	}
	logger.Info("Auth tables created or verified")

	// 3. Read auth configuration.
	authCfg := p.readConfig(config)

	// 4. Create JWTManager.
	jwtManager, err := NewJWTManager(authCfg)
	if err != nil {
		return fmt.Errorf("create JWT manager: %w", err)
	}
	logger.Info("JWT manager configured",
		"algorithm", authCfg.jwtAlgorithm,
		"access_ttl", authCfg.accessTokenTTL,
	)

	// 5. Create RBACManager and initialize default roles.
	rbacManager := NewRBACManager(store, logger)
	if err := rbacManager.InitializeDefaultRoles(ctx.Context()); err != nil {
		return fmt.Errorf("initialize default roles: %w", err)
	}
	logger.Info("Default roles and permissions initialized")

	// 6. Create RateLimiter.
	rateLimiter := NewRateLimiter(authCfg.rateLimitAttempts, authCfg.rateLimitWindow)
	p.rateLimit = rateLimiter
	logger.Info("Rate limiter configured",
		"max_attempts", authCfg.rateLimitAttempts,
		"window", authCfg.rateLimitWindow,
	)

	// 7. Create Service.
	svc := NewService(store, jwtManager, rbacManager, rateLimiter, logger, config, authCfg)
	p.service = svc

	// 8. Create default admin user if no users exist.
	if err := svc.CreateDefaultAdmin(ctx.Context()); err != nil {
		return fmt.Errorf("create default admin: %w", err)
	}

	// 9. Register AuthService via ServiceContainer.
	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.AuthService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register AuthService: %w", err)
	}

	logger.Info("Auth plugin initialized successfully",
		"service_registered", "auth.service",
	)

	return nil
}

// Start starts the auth plugin.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	logger := p.ctx.Logger()
	logger.Info("Auth plugin started successfully")
	p.running = true

	return nil
}

// Stop gracefully shuts down the auth plugin.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	logger := p.ctx.Logger()
	logger.Info("Stopping auth plugin")

	// Stop rate limiter background goroutine.
	if p.rateLimit != nil {
		p.rateLimit.Stop()
	}

	p.running = false
	logger.Info("Auth plugin stopped successfully")

	return nil
}

// readConfig reads auth plugin configuration from the ConfigProvider.
func (p *Plugin) readConfig(config core.ConfigProvider) authConfig {
	cfg := authConfig{
		jwtSecret:           config.GetString("auth.jwt_secret"),
		jwtAlgorithm:        config.GetString("auth.jwt_algorithm"),
		jwtPrivateKeyPath:   config.GetString("auth.jwt_private_key_path"),
		jwtPublicKeyPath:    config.GetString("auth.jwt_public_key_path"),
		rotateRefreshTokens: config.GetBool("auth.rotate_refresh_tokens"),
		rateLimitAttempts:   config.GetInt("auth.rate_limit.max_attempts"),
		bcryptCost:          config.GetInt("auth.bcrypt_cost"),
		adminEmail:          config.GetString("auth.admin.email"),
		adminPassword:       config.GetString("auth.admin.password"),
	}

	// Parse access token TTL as duration string (e.g. "15m").
	accessTTLStr := config.GetString("auth.access_token_ttl")
	if accessTTLStr != "" {
		if d, err := time.ParseDuration(accessTTLStr); err == nil {
			cfg.accessTokenTTL = d
		}
	}

	// Parse refresh token TTL as duration string (e.g. "7d").
	// Go's ParseDuration doesn't support "d" suffix, so convert days to hours.
	refreshTTLStr := config.GetString("auth.refresh_token_ttl")
	if refreshTTLStr != "" {
		cfg.refreshTokenTTL = parseDurationWithDays(refreshTTLStr)
	}

	// Parse rate limit window as duration string (e.g. "1m").
	windowStr := config.GetString("auth.rate_limit.window")
	if windowStr != "" {
		if d, err := time.ParseDuration(windowStr); err == nil {
			cfg.rateLimitWindow = d
		}
	}

	// Apply defaults.
	if cfg.jwtSecret == "" {
		cfg.jwtSecret = "aroute-default-secret-change-in-production"
	}
	if cfg.jwtAlgorithm == "" {
		cfg.jwtAlgorithm = "HS256"
	}
	if cfg.accessTokenTTL == 0 {
		cfg.accessTokenTTL = 15 * time.Minute
	}
	if cfg.refreshTokenTTL == 0 {
		cfg.refreshTokenTTL = 7 * 24 * time.Hour // 7 days.
	}
	if cfg.rateLimitAttempts == 0 {
		cfg.rateLimitAttempts = 5
	}
	if cfg.rateLimitWindow == 0 {
		cfg.rateLimitWindow = 1 * time.Minute
	}
	if cfg.bcryptCost == 0 {
		cfg.bcryptCost = 10
	}
	if cfg.adminEmail == "" {
		cfg.adminEmail = "admin@localhost"
	}
	if cfg.adminPassword == "" {
		cfg.adminPassword = "changeme"
	}

	return cfg
}

// parseDurationWithDays parses a duration string that may contain a "d" suffix
// for days, converting it to hours. For example, "7d" becomes "168h".
func parseDurationWithDays(s string) time.Duration {
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0
		}
		return time.Duration(days) * 24 * time.Hour
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}
