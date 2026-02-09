package augplus

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// PoolGainRequest is the request body for pool gain.
type PoolGainRequest struct {
	Product string `json:"product"`
	PoolID  string `json:"pool_id"`
	Version int    `json:"version"`
}

// PoolListRequest is the request body for pool list.
type PoolListRequest struct {
	Product string `json:"product"`
}

// poolGain handles POST /api/pools/gain
// This is the core endpoint that returns CLIProxyAPI credentials.
func (m *Module) poolGain(c *gin.Context) {
	var req PoolGainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "invalid request body")
		return
	}

	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	// Build the host address from config
	host := "127.0.0.1"
	port := 8317
	if cfg != nil {
		if cfg.Host != "" && cfg.Host != "0.0.0.0" {
			host = cfg.Host
		}
		if cfg.Port > 0 {
			port = cfg.Port
		}
	}

	// Get API key from config or use default
	apiKey := "my-ampcode-key"
	if cfg != nil && len(cfg.APIKeys) > 0 {
		apiKey = cfg.APIKeys[0]
	}

	// Return credentials based on product type
	if req.Product == "windsurf" {
		success(c, PoolAccount{
			AccessToken: apiKey,
			Email:       "local@cliproxyapi.local",
		})
		return
	}

	// Default: augment product
	success(c, PoolAccount{
		Token: apiKey,
		Host:  fmt.Sprintf("%s:%d", host, port),
		Email: "local@cliproxyapi.local",
	})
}

// poolList handles POST /api/pools/gain_list
// Returns available pools.
func (m *Module) poolList(c *gin.Context) {
	var req PoolListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "invalid request body")
		return
	}

	pools := []PoolItem{
		{PoolID: "local", Name: "本地 CLIProxyAPI"},
	}

	success(c, gin.H{"list": pools})
}
