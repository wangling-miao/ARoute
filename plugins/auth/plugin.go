// Package auth provides the authentication and authorization plugin for Aroute CMS.
// It implements JWT-based authentication, RBAC permissions, API token management,
// and rate limiting for authentication endpoints.
package auth

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"net"
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
	trustedProxies      []*net.IPNet
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
	authCfg, err := p.readConfig(config)
	if err != nil {
		return fmt.Errorf("read auth config: %w", err)
	}

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

	// 10. Register HTTP auth routes.
	var registrar interfaces.RouteRegistrar
	if err := ctx.Services().Get(&registrar); err != nil {
		logger.Warn("Route registrar not available, auth HTTP endpoints not registered", "error", err)
	} else {
		p.registerRoutes(registrar)
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

	// Drain in-flight background goroutines (e.g. async API token last-used updates).
	if p.service != nil {
		p.service.bgWg.Wait()
	}

	p.running = false
	logger.Info("Auth plugin stopped successfully")

	return nil
}

// readConfig reads auth plugin configuration from the ConfigProvider.
func (p *Plugin) readConfig(config core.ConfigProvider) (authConfig, error) {
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

	// Parse trusted proxies as a comma-separated list of CIDR strings.
	tpStr := config.GetString("auth.trusted_proxies")
	if tpStr != "" {
		for _, cidr := range strings.Split(tpStr, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				p.ctx.Logger().Warn("invalid trusted proxy CIDR, skipping", "cidr", cidr, "error", err)
				continue
			}
			cfg.trustedProxies = append(cfg.trustedProxies, network)
		}
	}

	// Apply defaults.
	if cfg.jwtSecret == "" {
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return authConfig{}, fmt.Errorf("generate jwt secret: %w", err)
		}
		cfg.jwtSecret = hex.EncodeToString(secretBytes)
		p.ctx.Logger().Warn("jwt_secret not configured; using random secret (will not persist across restarts)")
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
		passBytes := make([]byte, 16)
		if _, err := rand.Read(passBytes); err != nil {
			return authConfig{}, fmt.Errorf("generate admin password: %w", err)
		}
		cfg.adminPassword = hex.EncodeToString(passBytes)
		p.ctx.Logger().Warn("admin password not configured; using random password (check logs)", "admin_email", cfg.adminEmail)
	}

	return cfg, nil
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

func (p *Plugin) registerRoutes(registrar interfaces.RouteRegistrar) {
	registrar.HandleFunc("POST /api/v1/auth/login", p.handleLogin)
	registrar.HandleFunc("POST /api/v1/auth/refresh", p.handleRefresh)
	registrar.HandleFunc("GET /api/v1/auth/me", p.handleGetCurrentUser)

	registrar.HandleFunc("GET /api/v1/users", p.handleListUsers)
	registrar.HandleFunc("POST /api/v1/users", p.handleCreateUser)
	registrar.HandleFunc("PUT /api/v1/users/{id}", p.handleUpdateUser)
	registrar.HandleFunc("DELETE /api/v1/users/{id}", p.handleDeleteUser)

	registrar.HandleFunc("GET /api/v1/roles", p.handleListRoles)
	registrar.HandleFunc("PUT /api/v1/roles/{id}", p.handleUpdateRole)

	registrar.HandleFunc("GET /api/v1/api-tokens", p.handleListAPITokens)
	registrar.HandleFunc("POST /api/v1/api-tokens", p.handleCreateAPIToken)
	registrar.HandleFunc("DELETE /api/v1/api-tokens/{id}", p.handleRevokeAPIToken)
}
