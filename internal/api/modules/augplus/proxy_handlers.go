package augplus

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// getProxy handles POST /api/v1/get-proxy
// Returns the proxy URL for augment-proxy product.
func (m *Module) getProxy(c *gin.Context) {
	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	// Build the proxy URL from config
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

	proxyURL := fmt.Sprintf("http://%s:%d/", host, port)

	success(c, gin.H{"proxy": proxyURL})
}

// vipMerge handles POST /api/vips/merge
// Merges VIP records (no-op for local backend).
func (m *Module) vipMerge(c *gin.Context) {
	success(c, gin.H{"merged": true})
}
