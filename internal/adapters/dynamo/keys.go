// Package dynamo is the DynamoDB adapter: UserRepository and BetRepository
// on a single table (design.md's DynamoDB Single-Table Design), with all
// placement atomicity concentrated in one TransactWriteItems call (D8).
package dynamo

import (
	"fmt"
	"strings"
)

// profileSKValue is the fixed sort key of every user profile item.
const profileSKValue = "PROFILE"

// betSKPrefix is the sort-key prefix every bet item shares, so
// begins_with(SK, betSKPrefix) queries a user's full bet history in one
// partition, ULID-ordered.
const betSKPrefix = "BET#"

// idempotencySKPrefix is the sort-key prefix every idempotency record
// shares.
const idempotencySKPrefix = "IDEMP#"

// UserPK returns the shared partition key for every item type belonging to
// userID (user profile, bets, idempotency records).
func UserPK(userID string) string {
	return fmt.Sprintf("USER#%s", userID)
}

// ProfileSK returns the fixed sort key of a user's profile item.
func ProfileSK() string {
	return profileSKValue
}

// BetSK returns the sort key of the bet identified by betID.
func BetSK(betID string) string {
	return betSKPrefix + betID
}

// IdempotencySK returns the sort key of the idempotency record for key.
func IdempotencySK(key string) string {
	return idempotencySKPrefix + key
}

// EmailGSI1PK returns the EmailIndex GSI partition key for email, always
// lowercased so lookups are case-insensitive regardless of how the caller
// capitalized it at login.
func EmailGSI1PK(email string) string {
	return "EMAIL#" + strings.ToLower(email)
}
