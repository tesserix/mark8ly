package giftcard

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// IssueInput holds the fields needed to issue a new gift card.
type IssueInput struct {
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	InitialBalance decimal.Decimal
	CurrencyCode   string
	SenderName     *string
	SenderEmail    *string
	RecipientName  *string
	RecipientEmail *string
	Message        *string
	ExpiresAt      *time.Time
}

// BalanceResult is the response for a balance check.
type BalanceResult struct {
	Code           string          `json:"code"`
	CurrentBalance decimal.Decimal `json:"current_balance"`
	CurrencyCode   string          `json:"currency_code"`
	Status         GiftCardStatus  `json:"status"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
}

// ThemeLoader resolves the gift-card email theme for a given store
// (store name + branding). Narrow interface so the giftcard package
// doesn't depend on the branding package internals. Nil is allowed —
// the mailer falls back to editorial defaults when it is.
type ThemeLoader interface {
	LoadTheme(ctx context.Context, storeID uuid.UUID) (GiftCardEmailTheme, string, error)
}

// Service contains the business logic for gift cards.
type Service struct {
	db          *gorm.DB
	repo        Repository
	mailer      Mailer      // optional — nil disables delivery emails
	themeLoader ThemeLoader // optional — nil falls back to default theme
	logger      *slog.Logger
}

// NewService constructs a gift card Service.
//
// Deprecated shape kept for back-compat with tests; production wiring
// should call NewServiceWithMailer so delivery emails fire.
func NewService(db *gorm.DB, repo Repository, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, logger: logger}
}

// NewServiceWithMailer constructs a Service with delivery email wired.
// Mailer / themeLoader may be nil independently — missing mailer
// disables sending; missing themeLoader uses editorial defaults.
func NewServiceWithMailer(db *gorm.DB, repo Repository, mailer Mailer, themeLoader ThemeLoader, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, mailer: mailer, themeLoader: themeLoader, logger: logger}
}

// Unit runs fn inside a database transaction.
func (s *Service) Unit(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// Issue creates a new gift card with a cryptographically random code.
func (s *Service) Issue(ctx context.Context, in IssueInput) (*GiftCard, error) {
	if in.InitialBalance.LessThanOrEqual(decimal.Zero) {
		return nil, apperrors.ValidationFailed("initial_balance", "must be greater than zero")
	}
	if in.CurrencyCode == "" {
		return nil, apperrors.ValidationFailed("currency_code", "required")
	}

	code, err := GenerateCode()
	if err != nil {
		return nil, fmt.Errorf("generate gift card code: %w", err)
	}

	now := time.Now()
	gc := GiftCard{
		TenantID:       in.TenantID,
		StoreID:        in.StoreID,
		Code:           code,
		InitialBalance: in.InitialBalance,
		CurrentBalance: in.InitialBalance,
		CurrencyCode:   strings.ToUpper(in.CurrencyCode),
		Status:         StatusActive,
		SenderName:     in.SenderName,
		SenderEmail:    in.SenderEmail,
		RecipientName:  in.RecipientName,
		RecipientEmail: in.RecipientEmail,
		Message:        in.Message,
		PurchasedAt:    &now,
		ExpiresAt:      in.ExpiresAt,
	}

	initialTxn := Transaction{
		TenantID:     in.TenantID,
		Type:         TxnPurchase,
		Amount:       in.InitialBalance,
		BalanceAfter: in.InitialBalance,
	}

	err = s.Unit(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateInTx(tx, &gc, &initialTxn)
	})
	if err != nil {
		return nil, err
	}

	// Fire-and-log the delivery email. A send failure must NOT roll back
	// the gift card itself — the merchant can always resend from the
	// detail page. Errors land in the log with campaign-style context.
	s.sendDeliveryIfPossible(ctx, &gc)

	return &gc, nil
}

// sendDeliveryIfPossible dispatches the delivery email when a mailer is
// wired and the card has a recipient email. No-op otherwise. Errors are
// logged but never returned.
func (s *Service) sendDeliveryIfPossible(ctx context.Context, gc *GiftCard) {
	if s.mailer == nil || gc == nil || gc.RecipientEmail == nil || *gc.RecipientEmail == "" {
		return
	}

	theme := GiftCardEmailTheme{}
	var storefrontURL string
	if s.themeLoader != nil {
		loaded, url, err := s.themeLoader.LoadTheme(ctx, gc.StoreID)
		if err != nil {
			s.logger.Warn("giftcard: theme load failed — using defaults",
				"card_id", gc.ID, "store_id", gc.StoreID, "err", err)
		} else {
			theme = loaded
			storefrontURL = url
		}
	}

	if err := s.mailer.SendDelivery(ctx, DeliveryInput{
		Recipient:     *gc.RecipientEmail,
		Card:          gc,
		Theme:         theme,
		StorefrontURL: storefrontURL,
	}); err != nil {
		s.logger.Error("giftcard: delivery email failed",
			"card_id", gc.ID, "recipient", *gc.RecipientEmail, "err", err)
	}
}

// GetByID returns a single gift card with transactions.
func (s *Service) GetByID(ctx context.Context, storeID, id uuid.UUID) (*GiftCard, []Transaction, error) {
	gc, err := s.repo.GetByID(ctx, s.db, id, storeID)
	if err != nil {
		return nil, nil, err
	}
	txns, err := s.repo.ListTransactions(ctx, s.db, gc.ID)
	if err != nil {
		return gc, nil, err
	}
	return gc, txns, nil
}

// normalizeCode accepts any reasonable user-entered format and returns
// the canonical 26-char uppercase code we stored. Customers copy codes
// from email (formatted with dashes, e.g. XXXX-XXXX-XXXX-...), from
// chat (might include spaces or dashes), or paste straight from the
// database (no dashes). Strip both, uppercase, and move on.
func normalizeCode(code string) string {
	s := strings.ToUpper(strings.TrimSpace(code))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// GetByCode returns a gift card by its code within a store.
// Amendment LOW FIX 8: expose GetByCode as a public method.
func (s *Service) GetByCode(ctx context.Context, storeID uuid.UUID, code string) (*GiftCard, error) {
	return s.repo.GetByCode(ctx, s.db, storeID, normalizeCode(code))
}

// CheckBalance looks up a gift card by code and returns the balance.
// Returns domain errors for not-found, expired, or disabled cards.
func (s *Service) CheckBalance(ctx context.Context, storeID uuid.UUID, code string) (*BalanceResult, error) {
	gc, err := s.repo.GetByCode(ctx, s.db, storeID, normalizeCode(code))
	if err != nil {
		return nil, err
	}

	if gc.Status == StatusDisabled {
		return nil, apperrors.New(apperrors.CodeGiftCardNotFound, "gift card not found")
	}
	if gc.ExpiresAt != nil && gc.ExpiresAt.Before(time.Now()) {
		return nil, apperrors.New(apperrors.CodeGiftCardExpired, "gift card has expired")
	}

	return &BalanceResult{
		Code:           gc.Code,
		CurrentBalance: gc.CurrentBalance,
		CurrencyCode:   gc.CurrencyCode,
		Status:         gc.Status,
		ExpiresAt:      gc.ExpiresAt,
	}, nil
}

// ListByStore returns paginated gift cards.
func (s *Service) ListByStore(ctx context.Context, storeID, tenantID uuid.UUID, status *GiftCardStatus, page, pageSize int) ([]GiftCard, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.ListByStore(ctx, s.db, storeID, tenantID, status, page, pageSize)
}

// Debit atomically deducts amount from the gift card inside the given tx.
// This is called from checkout — the tx is owned by the checkout handler.
// Amendment CRITICAL FIX 1: uses the caller's tx, does NOT open its own.
func (s *Service) Debit(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID uuid.UUID, tenantID uuid.UUID) (decimal.Decimal, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, apperrors.ValidationFailed("amount", "must be greater than zero")
	}
	return s.repo.DebitInTx(tx, cardID, amount, orderID, tenantID)
}

// GenerateCode produces a 26-character uppercase base32 code using
// crypto/rand with 128-bit entropy (16 random bytes).
// Amendment CRITICAL FIX 2: use 16 bytes (128 bits), not 10.
// Format: XXXXXX-XXXXXX-XXXXXX-XXXXXX-XX (stored without dashes).
func GenerateCode() (string, error) {
	b := make([]byte, 16) // 16 bytes = 128 bits
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	// Take first 26 characters, uppercase.
	if len(raw) < 26 {
		return "", fmt.Errorf("base32 output too short: %d", len(raw))
	}
	return strings.ToUpper(raw[:26]), nil
}

// FormatCodeDisplay formats a 26-char code with dashes for display:
// XXXXXX-XXXXXX-XXXXXX-XXXXXX-XX
func FormatCodeDisplay(code string) string {
	if len(code) != 26 {
		return code
	}
	return code[0:4] + "-" + code[4:8] + "-" + code[8:12] + "-" + code[12:16] + "-" + code[16:20] + "-" + code[20:24] + "-" + code[24:26]
}
