// Package augplus provides AugPlus extension compatible API endpoints.
// This module allows the AugPlus VS Code extension to use CLIProxyAPI
// as a backend without credit deduction.
package augplus

import (
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/api/modules"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api/handlers"
	log "github.com/sirupsen/logrus"
)

// Module implements the AugPlus compatible API endpoints.
type Module struct {
	mu         sync.RWMutex
	cfg        *config.Config
	registered bool
}

// New creates a new AugPlus module instance.
func New() *Module {
	return &Module{}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "augplus"
}

// Register sets up the AugPlus compatible routes.
func (m *Module) Register(ctx modules.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.registered {
		return nil
	}

	m.cfg = ctx.Config
	m.registerRoutes(ctx.Engine, ctx.BaseHandler)
	m.registered = true

	log.Info("AugPlus compatible API module registered")
	return nil
}

// OnConfigUpdated handles configuration changes.
func (m *Module) OnConfigUpdated(cfg *config.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	return nil
}

// registerRoutes sets up all AugPlus compatible API routes.
func (m *Module) registerRoutes(engine *gin.Engine, _ *handlers.BaseAPIHandler) {
	// User endpoints
	engine.POST("/api/users/card-login", m.cardLogin)
	engine.POST("/api/users/whoami", m.whoami)
	engine.POST("/api/users/logout", m.logout)
	engine.POST("/api/users/vips", m.getVips)

	// Pool endpoints
	engine.POST("/api/pools/gain", m.poolGain)
	engine.POST("/api/pools/gain_list", m.poolList)

	// Proxy endpoint
	engine.POST("/api/v1/get-proxy", m.getProxy)

	// VIP merge endpoint
	engine.POST("/api/vips/merge", m.vipMerge)
}
