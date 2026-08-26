package account

import "errors"

// ErrInvalidCredentials is returned when login fails. It intentionally
// carries no detail about whether the email exists, so handlers never leak
// account existence to a caller.
var ErrInvalidCredentials = errors.New("account: invalid credentials")
