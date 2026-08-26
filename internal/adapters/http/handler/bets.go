package handler

import (
	"math"
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
		n, err := strconv.Atoi(raw)
		// n must fit the int32 range: BetRepository.ListByUser casts limit
		// straight into DynamoDB's QueryInput.Limit (*int32). A non-numeric
		// value, a negative value, or a value beyond math.MaxInt32 (which
		// previously either produced a DynamoDB ValidationException — a 500
		// — or silently wrapped to an unintended small limit) is rejected
		// here instead (finding W-limit).
		if err != nil || n < 0 || n > math.MaxInt32 {
			apperror.WriteStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "El parámetro limit debe ser un entero no negativo válido.")
			return
		}
		limit = n
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
