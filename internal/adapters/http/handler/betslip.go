package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/dto"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
	appauth "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
)

// messageInsufficientFunds is the Spanish, user-facing message for a
// rejected placement (design.md's PlaceBet Transaction "Responses"
// example).
const messageInsufficientFunds = "Saldo insuficiente para realizar la apuesta."

// idempotencyKeyHeader is the header POST /betslip/place reads its
// idempotency key from (spec: bet-slip-placement/Idempotent Placement via
// Idempotency-Key).
const idempotencyKeyHeader = "Idempotency-Key"

// idempotentReplayHeader marks a response that returned a previously
// recorded outcome rather than performing a fresh placement (design.md's
// PlaceBet Transaction "Responses").
const idempotentReplayHeader = "Idempotent-Replay"

// BetSlip serves POST /betslip/calculate (public) and POST /betslip/place
// (JWT-guarded).
type BetSlip struct {
	calculate *appbetslip.Calculate
	place     *appbetslip.Place
	balance   *appauth.Balance
}

// NewBetSlip builds a BetSlip handler. balance is used only by Place, to
// read the caller's post-placement balance (BetRepository.Place returns
// only the stored bet, never an updated balance).
func NewBetSlip(calculate *appbetslip.Calculate, place *appbetslip.Place, balance *appauth.Balance) *BetSlip {
	return &BetSlip{calculate: calculate, place: place, balance: balance}
}

// Calculate handles POST /betslip/calculate (spec: bet-slip-calculation/
// Calculate Endpoint Response Shape).
func (h *BetSlip) Calculate(c *gin.Context) {
	var req dto.BetSlipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperror.WriteStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "Debe indicar al menos una selección y un monto de apuesta válido.")
		return
	}

	stake, err := req.StakeMoney()
	if err != nil {
		apperror.WriteStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "El monto de la apuesta no es válido.")
		return
	}

	result, err := h.calculate.Execute(c.Request.Context(), appbetslip.CalculateCommand{
		SelectionIDs: req.SelectionIDs,
		Stake:        stake,
	})
	if err != nil {
		apperror.Write(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.CalculateResponseFromDomain(result))
}

// Place handles POST /betslip/place (JWT-guarded; spec: bet-slip-
// placement). It always reads the caller's fresh balance afterward
// (accepted, replayed-accepted, or rejected) because a rejected placement's
// 409 envelope also carries the current balance (D15's Responses example).
func (h *BetSlip) Place(c *gin.Context) {
	var req dto.BetSlipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperror.WriteStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "Debe indicar al menos una selección y un monto de apuesta válido.")
		return
	}

	stake, err := req.StakeMoney()
	if err != nil {
		apperror.WriteStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "El monto de la apuesta no es válido.")
		return
	}

	userID := middleware.UserID(c)

	result, err := h.place.Execute(c.Request.Context(), appbetslip.PlaceCommand{
		UserID:         userID,
		SelectionIDs:   req.SelectionIDs,
		Stake:          stake,
		IdempotencyKey: c.GetHeader(idempotencyKeyHeader),
	})
	if err != nil {
		apperror.Write(c, err)
		return
	}

	if result.Replayed {
		c.Header(idempotentReplayHeader, "true")
	}

	balanceAfter, balErr := h.balance.Execute(c.Request.Context(), userID)
	if balErr != nil {
		apperror.Write(c, balErr)
		return
	}

	// A replay of a previously rejected key resolves with err == nil (D16 —
	// the recorded outcome is returned untouched, never re-evaluated), so
	// the rejected-envelope branch must be driven by the bet's own status,
	// not by the presence of an error.
	if result.Bet.Status == domainbetslip.BetStatusRejected {
		apperror.WriteDetails(c, http.StatusConflict, "INSUFFICIENT_FUNDS", messageInsufficientFunds, map[string]any{
			"betId":           result.Bet.ID,
			"status":          string(result.Bet.Status),
			"rejectionReason": result.Bet.RejectionReason,
			"balance":         balanceAfter,
			"required":        result.Bet.Stake,
		})
		return
	}

	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	c.JSON(status, dto.PlaceResponseFromDomain(result.Bet, balanceAfter))
}
