package augplus

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is the standard AugPlus API response format.
type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

// User represents a user object in AugPlus API.
type User struct {
	ID    string `json:"id"`
	Token string `json:"token"`
	Email string `json:"email,omitempty"`
	VIP   *VIP   `json:"vip,omitempty"`
}

// VIP represents VIP information.
type VIP struct {
	Product   string `json:"product"`
	Score     int    `json:"score"`
	ScoreUsed int    `json:"score_used"`
}

// PoolAccount represents an account from the pool.
type PoolAccount struct {
	Token       string `json:"token,omitempty"`
	Host        string `json:"host,omitempty"`
	Email       string `json:"email,omitempty"`
	AccessToken string `json:"access_token,omitempty"`
}

// PoolItem represents a pool in the list.
type PoolItem struct {
	PoolID string `json:"pool_id"`
	Name   string `json:"name"`
}

// success returns a successful response.
func success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
		Data: data,
	})
}

// fail returns an error response.
func fail(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, Response{
		Code: -1,
		Msg:  msg,
	})
}
