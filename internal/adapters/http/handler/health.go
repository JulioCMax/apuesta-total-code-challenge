package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthResponse is the JSON body of GET /health.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health handles GET /health: an unlimited, public liveness probe carrying
// the app's version (design.md's HTTP Layer route table).
func Health(version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthResponse{Status: "ok", Version: version})
	}
}
