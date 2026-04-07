// Package verification implements email OTP verification for the onboarding flow.
//
// Design notes:
//   - The 6-digit code is stored as its bcrypt hash, never plaintext.
//     A DB dump cannot be used to bypass verification.
//   - Each token is bound to BOTH an email AND an onboarding session ID.
//     A token issued for one session cannot be used to verify another.
//   - Tokens have a max-attempts counter to defeat brute force.
//   - On success the token is consumed (consumed_at is set), making it
//     single-use.
//   - Tokens expire after 10 minutes.
//
// The package depends on `internal/notification` to actually send the
// email. The Sender is injected so unit tests can use NoopSender.
package verification

import "time"

// Token represents one outstanding verification attempt.
type Token struct {
	ID          string     `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	SessionID   string     `gorm:"column:session_id;type:uuid;not null;index"               json:"session_id"`
	Email       string     `gorm:"column:email;type:varchar(320);not null;index"            json:"email"`
	CodeHash    string     `gorm:"column:code_hash;type:varchar(128);not null"              json:"-"`
	Purpose     string     `gorm:"column:purpose;type:varchar(40);not null;default:'email_verification'" json:"purpose"`
	Attempts    int16      `gorm:"column:attempts;not null;default:0"                       json:"attempts"`
	MaxAttempts int16      `gorm:"column:max_attempts;not null;default:5"                   json:"max_attempts"`
	ExpiresAt   time.Time  `gorm:"column:expires_at;not null"                               json:"expires_at"`
	ConsumedAt  *time.Time `gorm:"column:consumed_at"                                       json:"consumed_at,omitempty"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
}

// TableName overrides GORM's default pluralization.
func (Token) TableName() string { return "verification_tokens" }

const (
	PurposeEmailVerification = "email_verification"
	// TokenLifetime is how long a verification code is valid after issue.
	TokenLifetime = 10 * time.Minute
	// CodeLength is the number of digits in the OTP.
	CodeLength = 6
	// MaxAttemptsDefault is the per-token brute-force limit.
	MaxAttemptsDefault = 5
)
