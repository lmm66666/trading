package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"github.com/gin-gonic/gin"
)

// NewUpdaterRouter deliberately exposes only the internal refresh capability.
func NewUpdaterRouter(kernel KernelServices, token string) (*gin.Engine, error) {
	if err := ValidateUpdaterToken(token); err != nil {
		return nil, err
	}
	expected := sha256.Sum256([]byte("Bearer " + token))
	r := gin.New()
	r.Use(gin.Recovery())
	h := &StockHandler{kernel: kernel}
	r.POST(updaterRefreshPath, func(c *gin.Context) {
		provided := sha256.Sum256([]byte(c.GetHeader("Authorization")))
		if len(c.Request.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
			respondError(c, 401, "UNAUTHORIZED")
			return
		}
		h.MarketRefresh(c)
	})
	return r, nil
}
