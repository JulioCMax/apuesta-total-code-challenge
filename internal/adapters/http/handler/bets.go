package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/dto"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
	appauth "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
)

// Bets serves GET /bets (JWT-guarded; spec: bet-history/List Caller's Own
// Bets).
type Bets struct {
	history *appauth.History
}

// NewBets builds a Bets handler backed by history.
func NewBets(history *appauth.History) *Bets {
	return &Bets{history: history}
}

// List handles GET /bets?limit=&cursor=. The caller's user ID comes
// exclusively from the verified JWT (middleware.UserID) — never from a
// query parameter — so a caller can never request another user's history.
func (h *Bets) List(c *gin.Context) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}

	result, err := h.history.Execute(c.Request.Context(), appauth.HistoryCommand{
		UserID: middleware.UserID(c),
		Limit:  limit,
		Cursor: c.Query("cursor"),
	})
	if err != nil {
		apperror.Write(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.BetsResponseFromDomain(result.Bets, result.NextCursor))
}
