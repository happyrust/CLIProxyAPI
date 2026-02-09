package augplus

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
)

// CardLoginRequest is the request body for card login.
type CardLoginRequest struct {
	Card  string `json:"card"`
	Email string `json:"email"`
	Agent string `json:"agent"`
}

// generateToken generates a random token.
func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "local_" + hex.EncodeToString(b)
}

// cardLogin handles POST /api/users/card-login
// This endpoint accepts any card and returns a local user.
func (m *Module) cardLogin(c *gin.Context) {
	var req CardLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "invalid request body")
		return
	}

	if req.Card == "" {
		fail(c, "card is required")
		return
	}

	// Generate a local user with unlimited credits
	user := User{
		ID:    "local_user_" + time.Now().Format("20060102150405"),
		Token: generateToken(),
		Email: req.Email,
		VIP: &VIP{
			Product:   "augment",
			Score:     999999,
			ScoreUsed: 0,
		},
	}

	success(c, user)
}

// whoami handles POST /api/users/whoami
// Returns the current user information.
func (m *Module) whoami(c *gin.Context) {
	// Return a local user with unlimited credits
	user := User{
		ID:    "local_user",
		Token: c.GetHeader("X-Auth-Token"),
		Email: "local@cliproxyapi.local",
		VIP: &VIP{
			Product:   "augment",
			Score:     999999,
			ScoreUsed: 0,
		},
	}

	success(c, user)
}

// logout handles POST /api/users/logout
func (m *Module) logout(c *gin.Context) {
	success(c, nil)
}

// getVips handles POST /api/users/vips
// Returns the VIP list for the current user.
func (m *Module) getVips(c *gin.Context) {
	vips := []VIP{
		{Product: "augment", Score: 999999, ScoreUsed: 0},
		{Product: "windsurf", Score: 999999, ScoreUsed: 0},
		{Product: "augment-proxy", Score: 999999, ScoreUsed: 0},
	}

	success(c, gin.H{"list": vips})
}
