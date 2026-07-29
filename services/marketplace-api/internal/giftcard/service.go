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

// PurchaseInput is what the storefront purchase endpoint collects from
// the buyer: amount, currency (forced to store currency by the handler),
// recipient + sender display info + personal message, and the buyer's
// own name/email (who is paying).
type PurchaseInput struct {
	TenantID         uuid.UUID
	StoreID          uuid.UUID
	InitialBalance   decimal.Decimal
	CurrencyCode     string
	SenderName       *string
	SenderEmail      *string
	RecipientName    *string
	RecipientEmail   *string
	Message          *string
	ExpiresAt        *time.Time
	PurchaserName    string
	PurchaserEmail   string
	PaymentProvider  string // "stripe" (only supported provider for hosted checkout right now)
	SuccessURL       string // where the provider redirects after payment success
	CancelURL        string // where the provider redirects on cancel/decline
}

// PurchaseResult is what the storefront purchase endpoint returns.
type PurchaseResult struct {
	GiftCardID        uuid.UUID
	CheckoutSessionID string
	CheckoutURL       string
	Provider          string
}

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

// GatewayResolver resolves a payment gateway by provider name for the
// calling store. The storefront purchase flow uses it to obtain the
// store-scoped Stripe gateway without coupling this package to the
// full payment.Service or its repository.
type GatewayResolver interface {
	ResolveCheckoutGateway(ctx context.Context, storeID uuid.UUID, provider string) (CheckoutCapableGateway, error)
}

// CheckoutCapableGateway is the minimum surface giftcard needs from a
// payment provider to create a hosted checkout session. Concrete
// implementations (e.g. payment.StripeGateway) satisfy this.
type CheckoutCapableGateway interface {
	ProviderName() string
	CreateCheckoutSession(ctx context.Context, in CheckoutSessionInput) (*CheckoutSessionOutput, error)
}

// CheckoutSessionInput mirrors payment.CreateCheckoutSessionInput. We
// redeclare it here so the giftcard package doesn't import `payment`.
type CheckoutSessionInput struct {
	ReferenceID   string
	Amount        decimal.Decimal
	CurrencyCode  string
	CustomerEmail string
	Description   string
	Name          string
	SuccessURL    string
	CancelURL     string
	Metadata      map[string]string
}

// CheckoutSessionOutput mirrors payment.CheckoutSession.
type CheckoutSessionOutput struct {
	ID  string
	URL string
}

// Service contains the business logic for gift cards.
type Service struct {
	db              *gorm.DB
	repo            Repository
	mailer          Mailer          // optional — nil disables delivery emails
	themeLoader     ThemeLoader     // optional — nil falls back to default theme
	gatewayResolver GatewayResolver // optional — nil disables storefront purchase
	logger          *slog.Logger
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

// WithGatewayResolver attaches a GatewayResolver used by the storefront
// Purchase flow. Fluent setter so we don't break existing callers.
func (s *Service) WithGatewayResolver(gr GatewayResolver) *Service {
	s.gatewayResolver = gr
	return s
}

// Unit runs fn inside a database transaction.
func (s *Service) Unit(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// Purchase creates a pending gift card + a hosted checkout session for
// the storefront purchase flow. The card is NOT spendable until the
// provider webhook calls ActivateByCheckoutSession on payment success.
//
// Returns the checkout URL the storefront should redirect the buyer to.
func (s *Service) Purchase(ctx context.Context, in PurchaseInput) (*PurchaseResult, error) {
	if in.InitialBalance.LessThanOrEqual(decimal.Zero) {
		return nil, apperrors.ValidationFailed("initial_balance", "must be greater than zero")
	}
	if in.CurrencyCode == "" {
		return nil, apperrors.ValidationFailed("currency_code", "required")
	}
	if in.PurchaserEmail == "" {
		return nil, apperrors.ValidationFailed("purchaser_email", "required")
	}
	if s.gatewayResolver == nil {
		return nil, fmt.Errorf("giftcard: purchase disabled — gateway resolver not wired")
	}
	if in.SuccessURL == "" || in.CancelURL == "" {
		return nil, apperrors.ValidationFailed("success_url/cancel_url", "required")
	}

	provider := in.PaymentProvider
	if provider == "" {
		provider = "stripe"
	}

	gateway, err := s.gatewayResolver.ResolveCheckoutGateway(ctx, in.StoreID, provider)
	if err != nil {
		return nil, fmt.Errorf("giftcard purchase: resolve gateway: %w", err)
	}

	code, err := GenerateCode()
	if err != nil {
		return nil, fmt.Errorf("giftcard purchase: generate code: %w", err)
	}

	gc := newPendingCard(in, code, provider, time.Now())
	// No ledger row at purchase time: nothing has been paid, so nothing
	// may be recorded as value. ActivateAfterPayment writes the
	// `purchase` row when the payment settles.
	err = s.Unit(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateInTx(tx, &gc, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("giftcard purchase: persist: %w", err)
	}

	session, err := gateway.CreateCheckoutSession(ctx, CheckoutSessionInput{
		ReferenceID:   gc.ID.String(),
		Amount:        in.InitialBalance,
		CurrencyCode:  in.CurrencyCode,
		CustomerEmail: in.PurchaserEmail,
		Name:          fmt.Sprintf("Gift card — %s", in.CurrencyCode),
		Description:   "Gift card purchase",
		SuccessURL:    in.SuccessURL,
		CancelURL:     in.CancelURL,
		Metadata: map[string]string{
			"gift_card_id": gc.ID.String(),
			"store_id":     in.StoreID.String(),
		},
	})
	if err != nil {
		// Card already persisted with status=pending. Log and let the
		// caller try again later; admin can clean up orphaned pending
		// cards.
		s.logger.Error("giftcard purchase: create checkout session failed",
			"card_id", gc.ID, "err", err)
		return nil, fmt.Errorf("giftcard purchase: create checkout session: %w", err)
	}

	// Persist the provider-side session id so the webhook can correlate.
	if err := s.db.WithContext(ctx).Model(&GiftCard{}).
		Where("id = ?", gc.ID).
		Update("checkout_session_id", session.ID).Error; err != nil {
		s.logger.Error("giftcard purchase: persist checkout session id", "card_id", gc.ID, "err", err)
	}

	return &PurchaseResult{
		GiftCardID:        gc.ID,
		CheckoutSessionID: session.ID,
		CheckoutURL:       session.URL,
		Provider:          gateway.ProviderName(),
	}, nil
}

// newPendingCard builds the row a storefront purchase persists before the
// buyer has paid anything.
//
// SECURITY: CurrentBalance is deliberately zero. The card carries no value until
// ActivateAfterPayment funds it from InitialBalance, in the same UPDATE
// that flips pending → active. Setting it to InitialBalance here would
// make an unpaid card spendable — the buyer can read their own pending
// card's code from GET /account/gift-cards, and the debit would clear.
// Do not "restore" this to InitialBalance.
func newPendingCard(in PurchaseInput, code, provider string, now time.Time) GiftCard {
	pendingStatus := PaymentStatusPending
	gc := GiftCard{
		TenantID:               in.TenantID,
		StoreID:                in.StoreID,
		Code:                   code,
		InitialBalance:         in.InitialBalance,
		CurrentBalance:         decimal.Zero,
		CurrencyCode:           strings.ToUpper(in.CurrencyCode),
		Status:                 StatusPending,
		SenderName:             in.SenderName,
		SenderEmail:            in.SenderEmail,
		RecipientName:          in.RecipientName,
		RecipientEmail:         in.RecipientEmail,
		Message:                in.Message,
		PurchasedAt:            &now,
		ExpiresAt:              in.ExpiresAt,
		PaymentStatus:          &pendingStatus,
		PaymentProvider:        &provider,
		PurchasedViaStorefront: true,
		PurchasedByEmail:       &in.PurchaserEmail,
	}
	if in.PurchaserName != "" {
		gc.PurchasedByName = &in.PurchaserName
	}
	return gc
}

// ActivateByCheckoutSession transitions a pending gift card to active
// when the provider reports a successful payment. Called from the
// payment webhook with the hosted-checkout session id. Idempotent:
// returns (card, true, nil) on a first-time flip, (card, false, nil) if
// the card is already active. Also triggers the delivery email on
// first-time flip.
func (s *Service) ActivateByCheckoutSession(ctx context.Context, sessionID, paymentIntentID string) (*GiftCard, bool, error) {
	gc, err := s.repo.GetByCheckoutSessionID(ctx, s.db, sessionID)
	if err != nil {
		return nil, false, err
	}
	return s.activatePending(ctx, gc, paymentIntentID)
}

// ActivateByPaymentIntent handles the case where the webhook arrives
// with a payment_intent event instead of a checkout session event.
func (s *Service) ActivateByPaymentIntent(ctx context.Context, paymentIntentID string) (*GiftCard, bool, error) {
	gc, err := s.repo.GetByPaymentIntentID(ctx, s.db, paymentIntentID)
	if err != nil {
		return nil, false, err
	}
	return s.activatePending(ctx, gc, paymentIntentID)
}

// MarkPurchaseFailed flags the card's payment as failed. Status stays
// pending so admin can see the attempt in the "pending payment" filter.
//
// A declined card is not redeemable: it was never funded
// (current_balance = 0), DebitInTx only touches `active` rows, and
// CheckBalance reports `pending` as not-found. Leaving the status alone
// therefore costs nothing but merchant-visible bookkeeping.
func (s *Service) MarkPurchaseFailed(ctx context.Context, sessionOrIntentID string) error {
	gc, err := s.repo.GetByCheckoutSessionID(ctx, s.db, sessionOrIntentID)
	if err != nil {
		gc, err = s.repo.GetByPaymentIntentID(ctx, s.db, sessionOrIntentID)
		if err != nil {
			return err
		}
	}
	return s.repo.MarkPaymentFailed(s.db.WithContext(ctx), gc.ID)
}

// RefundPurchase voids a gift card whose PURCHASE was refunded through the
// payment provider, correlated by hosted-checkout session id or payment
// intent id (whichever the event carried).
//
// The card stops being spendable even when part of the balance has already
// been redeemed: the merchant chose to issue the refund, and leaving a
// funded card behind means paying for it twice. The value already spent is
// unrecoverable, so it is written to the ledger as a recorded merchant loss
// instead of vanishing.
//
// Idempotent: a redelivered webhook returns (card, false, nil) and writes
// nothing.
func (s *Service) RefundPurchase(ctx context.Context, sessionOrIntentID string) (*GiftCard, bool, error) {
	if sessionOrIntentID == "" {
		return nil, false, apperrors.ValidationFailed("reference", "checkout session or payment intent id required")
	}
	gc, err := s.repo.GetByCheckoutSessionID(ctx, s.db, sessionOrIntentID)
	if err != nil {
		gc, err = s.repo.GetByPaymentIntentID(ctx, s.db, sessionOrIntentID)
		if err != nil {
			return nil, false, err
		}
	}

	var res *VoidResult
	if err := s.Unit(ctx, func(tx *gorm.DB) error {
		var vErr error
		res, vErr = s.repo.VoidForPurchaseRefundInTx(tx, gc.ID)
		return vErr
	}); err != nil {
		return nil, false, fmt.Errorf("giftcard: void after purchase refund: %w", err)
	}

	if res.Voided && res.Redeemed.GreaterThan(decimal.Zero) && s.logger != nil {
		// The merchant will ask about this number. Make it greppable.
		s.logger.Warn("giftcard: purchase refunded on a partly-spent card — shortfall not recoverable",
			"card_id", gc.ID, "store_id", gc.StoreID,
			"redeemed_before_refund", res.Redeemed.StringFixed(2),
			"initial_balance", res.InitialBalance.StringFixed(2))
	}

	voided, err := s.repo.GetByID(ctx, s.db, gc.ID, gc.StoreID)
	if err != nil {
		return nil, res.Voided, fmt.Errorf("giftcard: reload after void: %w", err)
	}
	return voided, res.Voided, nil
}

func (s *Service) activatePending(ctx context.Context, gc *GiftCard, paymentIntentID string) (*GiftCard, bool, error) {
	if gc.Status != StatusPending {
		return gc, false, nil // already activated (or in a non-activatable state)
	}
	// One transaction: funding the balance, flipping the status and
	// writing the purchase ledger row either all land or none do.
	if err := s.Unit(ctx, func(tx *gorm.DB) error {
		return s.repo.ActivateAfterPayment(tx, gc.ID, paymentIntentID)
	}); err != nil {
		return nil, false, fmt.Errorf("giftcard: activate: %w", err)
	}
	// Reload so downstream sees the flipped status.
	activated, err := s.repo.GetByID(ctx, s.db, gc.ID, gc.StoreID)
	if err != nil {
		return nil, false, fmt.Errorf("giftcard: reload after activate: %w", err)
	}
	// Fire the delivery email now that the purchase has cleared.
	s.sendDeliveryIfPossible(ctx, activated)
	return activated, true, nil
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
		TenantID:      gc.TenantID.String(),
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
// Returns domain errors for not-found, expired, or non-redeemable cards.
//
// This is a truthfulness guard, not the security boundary — the boundary
// is the `status = 'active'` predicate inside DebitInTx. Its job is to
// stop the storefront showing a shopper a balance they cannot spend.
func (s *Service) CheckBalance(ctx context.Context, storeID uuid.UUID, code string) (*BalanceResult, error) {
	gc, err := s.repo.GetByCode(ctx, s.db, storeID, normalizeCode(code))
	if err != nil {
		return nil, err
	}

	// `pending` (payment not captured), `disabled` and `refunded` are all
	// non-redeemable. Report them identically to a miss so the endpoint
	// can't be used to probe which codes exist in which state.
	switch gc.Status {
	case StatusDisabled, StatusPending, StatusRefunded:
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

// ListByCustomerEmail returns up to 100 gift cards where the customer
// is purchaser OR recipient, for their My Account view.
func (s *Service) ListByCustomerEmail(ctx context.Context, storeID uuid.UUID, email string) ([]GiftCard, error) {
	if email == "" {
		return nil, nil
	}
	return s.repo.ListByCustomerEmail(ctx, s.db, storeID, strings.ToLower(strings.TrimSpace(email)))
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

// SetStatus flips a gift card between active and disabled — the only two
// legal admin-driven states. Wraps the atomic repository UPDATE in a
// transaction (matches every other mutating method in this file); the
// transition-legality rules themselves live in the repository's WHERE
// clause, not here (see SetStatus's doc comment in repository.go).
func (s *Service) SetStatus(ctx context.Context, storeID, tenantID, id uuid.UUID, to GiftCardStatus) (*GiftCard, error) {
	if to != StatusActive && to != StatusDisabled {
		return nil, apperrors.ValidationFailed("status", "must be \"active\" or \"disabled\"")
	}
	var gc *GiftCard
	err := s.Unit(ctx, func(tx *gorm.DB) error {
		var err error
		gc, err = s.repo.SetStatus(tx, id, tenantID, storeID, to)
		return err
	})
	if err != nil {
		return nil, err
	}
	return gc, nil
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
