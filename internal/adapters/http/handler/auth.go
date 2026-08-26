package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/dto"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
	appauth "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
)

// Auth serves POST /auth/login (public) and GET /balance (JWT-guarded).
type Auth struct {
	login    *appauth.Login
	balance  *appauth.Balance
	currency string
}

// NewAuth builds an Auth handler backed by login and balance. currency is
// the configured currency code (BETSLIP_CURRENCY_CODE) echoed in the
// balance response — application/auth.Balance returns only a money.Money,
// never a currency.
func NewAuth(login *appauth.Login, balance *appauth.Balance, currency string) *Auth {
	return &Auth{login: login, balance: balance, currency: currency}
}

// Login handles POST /auth/login (spec: auth-and-balance/Demo User Login
// Issuing JWT).
func (h *Auth) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperror.WriteStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "Debe indicar un correo y una contraseña válidos.")
		return
	}

	result, err := h.login.Execute(c.Request.Context(), appauth.LoginCommand{Email: req.Email, Password: req.Password})
	if err != nil {
		apperror.Write(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.LoginResponse{Token: result.Token, ExpiresIn: int64(result.ExpiresIn.Seconds())})
}

// Balance handles GET /balance (JWT-guarded; spec: auth-and-balance/
// Balance Query).
func (h *Auth) Balance(c *gin.Context) {
	bal, err := h.balance.Execute(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		apperror.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.BalanceResponse{Balance: bal, Currency: h.currency})
}
