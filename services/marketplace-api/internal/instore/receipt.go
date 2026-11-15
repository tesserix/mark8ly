package instore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// A receipt link (#1106) shows payment and contact details only while fresh;
// items and totals stay visible for good.
const (
	receiptFreshFor   = 7 * 24 * time.Hour
	receiptResendWait = 10 * time.Minute
)

// ReceiptMailer emails a receipt-page link; orderdoc.Service implements it.
type ReceiptMailer interface {
	SendInStoreReceipt(ctx context.Context, orderID uuid.UUID, receiptURL string) error
}

// ReceiptService mints receipt links and reads receipts by token. Only the
// token's SHA-256 is stored, so the table can't be replayed into links.
type ReceiptService struct {
	db      *gorm.DB
	mailer  ReceiptMailer
	urlBase string // storefront URL template with {slug}
}

func NewReceiptService(db *gorm.DB, mailer ReceiptMailer, urlBase string) *ReceiptService {
	return &ReceiptService{db: db, mailer: mailer, urlBase: urlBase}
}

// ReceiptLine is one line as the customer sees it.
type ReceiptLine struct {
	Title         string          `json:"title"`
	OptionSummary *string         `json:"option_summary,omitempty"`
	ImageURL      *string         `json:"image_url,omitempty"`
	Quantity      int             `json:"quantity"`
	UnitPrice     decimal.Decimal `json:"unit_price"`
	LineTotal     decimal.Decimal `json:"line_total"`
}

// Receipt is the receipt page's data. Payment and contact fields are set
// only while the link is fresh.
type Receipt struct {
	OrderNumber        string          `json:"order_number"`
	PlacedAt           time.Time       `json:"placed_at"`
	CurrencyCode       string          `json:"currency_code"`
	Lines              []ReceiptLine   `json:"lines"`
	Subtotal           decimal.Decimal `json:"subtotal"`
	DiscountTotal      decimal.Decimal `json:"discount_total"`
	TaxTotal           decimal.Decimal `json:"tax_total"`
	GrandTotal         decimal.Decimal `json:"grand_total"`
	TaxLabel           string          `json:"tax_label"`
	TimeZone           string          `json:"time_zone"` // the store's, for the date and time shown
	Fresh              bool            `json:"fresh"`
	PaymentMethod      *string         `json:"payment_method,omitempty"`
	PaymentMethodLabel *string         `json:"payment_method_label,omitempty"`
	ReceiptEmailMasked *string         `json:"receipt_email_masked,omitempty"`
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Issue mints a new link for an in-store order of this store.
func (s *ReceiptService) Issue(ctx context.Context, storeID, orderID uuid.UUID) (string, error) {
	var slug string
	err := s.db.WithContext(ctx).Raw(`
		SELECT st.slug FROM orders o JOIN stores st ON st.id = o.store_id
		 WHERE o.id = ? AND o.store_id = ? AND o.channel = 'in_store' AND o.deleted_at IS NULL`,
		orderID, storeID).Row().Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return "", apperrors.NotFound("order")
	}
	if err != nil {
		return "", fmt.Errorf("instore: receipt order: %w", err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("instore: receipt token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.db.WithContext(ctx).Exec(
		`INSERT INTO receipt_links (token_hash, order_id, store_id) VALUES (?, ?, ?)`,
		hashToken(token), orderID, storeID).Error; err != nil {
		return "", fmt.Errorf("instore: save receipt link: %w", err)
	}
	return strings.TrimRight(strings.ReplaceAll(s.urlBase, "{slug}", slug), "/") + "/r/" + token, nil
}

// SoldBy is who rang up an in-store order of this store; "" if no one is
// recorded. Not found when the order isn't an in-store order of this store.
func (s *ReceiptService) SoldBy(ctx context.Context, storeID, orderID uuid.UUID) (string, error) {
	var soldBy *string
	err := s.db.WithContext(ctx).Raw(
		`SELECT sold_by_user_id FROM orders WHERE id = ? AND store_id = ? AND channel = 'in_store' AND deleted_at IS NULL`,
		orderID, storeID).Row().Scan(&soldBy)
	if errors.Is(err, sql.ErrNoRows) {
		return "", apperrors.NotFound("order")
	}
	if err != nil {
		return "", fmt.Errorf("instore: receipt seller: %w", err)
	}
	if soldBy == nil {
		return "", nil
	}
	return *soldBy, nil
}

// Send saves the receipt address on the order (receipts only, never
// marketing), then mints and emails a link.
func (s *ReceiptService) Send(ctx context.Context, storeID, orderID uuid.UUID, email string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || addr.Name != "" {
		return "", apperrors.ValidationFailed("email", "enter a valid email address")
	}
	res := s.db.WithContext(ctx).Exec(
		`UPDATE orders SET customer_email = ?, updated_at = now()
		  WHERE id = ? AND store_id = ? AND channel = 'in_store' AND deleted_at IS NULL`,
		strings.ToLower(addr.Address), orderID, storeID)
	if res.Error != nil {
		return "", fmt.Errorf("instore: save receipt email: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return "", apperrors.NotFound("order")
	}
	url, err := s.Issue(ctx, storeID, orderID)
	if err != nil {
		return "", err
	}
	if err := s.mailer.SendInStoreReceipt(ctx, orderID, url); err != nil {
		return "", fmt.Errorf("instore: email receipt: %w", err)
	}
	return url, nil
}

type receiptOrder struct {
	ID                 uuid.UUID
	OrderNumber        string
	PlacedAt           time.Time
	CurrencyCode       string
	Subtotal           decimal.Decimal
	DiscountTotal      decimal.Decimal
	TaxTotal           decimal.Decimal
	GrandTotal         decimal.Decimal
	CustomerEmail      string
	PaymentMethod      *string
	PaymentMethodLabel *string
	CountryCode        string
	Timezone           string
	LinkCreatedAt      time.Time
}

func (s *ReceiptService) find(ctx context.Context, storeID uuid.UUID, token string) (*receiptOrder, error) {
	var o receiptOrder
	err := s.db.WithContext(ctx).Raw(`
		SELECT o.id, o.order_number, o.placed_at, o.currency_code, o.subtotal, o.discount_total,
		       o.tax_total, o.grand_total, o.customer_email, o.payment_method, o.payment_method_label,
		       st.country_code, st.timezone, rl.created_at AS link_created_at
		  FROM receipt_links rl
		  JOIN orders o ON o.id = rl.order_id AND o.deleted_at IS NULL
		  JOIN stores st ON st.id = o.store_id
		 WHERE rl.token_hash = ? AND rl.store_id = ?`, hashToken(token), storeID).Scan(&o).Error
	if err != nil {
		return nil, fmt.Errorf("instore: load receipt: %w", err)
	}
	if o.ID == uuid.Nil {
		return nil, apperrors.NotFound("receipt")
	}
	return &o, nil
}

// Load reads a receipt by token, scoped to the store whose address it was
// opened on.
func (s *ReceiptService) Load(ctx context.Context, storeID uuid.UUID, token string) (*Receipt, error) {
	o, err := s.find(ctx, storeID, token)
	if err != nil {
		return nil, err
	}
	var lines []ReceiptLine
	if err := s.db.WithContext(ctx).Raw(`
		SELECT title_snapshot AS title, option_summary, image_url, quantity, unit_price, line_total
		  FROM order_items WHERE order_id = ? ORDER BY created_at, id`, o.ID).Scan(&lines).Error; err != nil {
		return nil, fmt.Errorf("instore: receipt lines: %w", err)
	}
	r := &Receipt{
		OrderNumber: o.OrderNumber, PlacedAt: o.PlacedAt, CurrencyCode: o.CurrencyCode, Lines: lines,
		Subtotal: o.Subtotal, DiscountTotal: o.DiscountTotal, TaxTotal: o.TaxTotal, GrandTotal: o.GrandTotal,
		TaxLabel: "Tax", TimeZone: o.Timezone, Fresh: time.Since(o.LinkCreatedAt) < receiptFreshFor,
	}
	if o.CountryCode == "AU" {
		r.TaxLabel = "GST included"
	}
	if r.Fresh {
		r.PaymentMethod, r.PaymentMethodLabel = o.PaymentMethod, o.PaymentMethodLabel
		if m := maskEmail(o.CustomerEmail); m != "" {
			r.ReceiptEmailMasked = &m
		}
	}
	return r, nil
}

// Resend mails a new link to the address on the order, at most once per
// receiptResendWait. It never says whether there was an address, so a link
// holder learns nothing from it.
func (s *ReceiptService) Resend(ctx context.Context, storeID uuid.UUID, token string) error {
	o, err := s.find(ctx, storeID, token)
	if err != nil {
		return err
	}
	if o.CustomerEmail == "" {
		return nil
	}
	var recent int
	if err := s.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM receipt_links WHERE order_id = ? AND created_at > now() - make_interval(secs => ?)`,
		o.ID, receiptResendWait.Seconds()).Row().Scan(&recent); err != nil {
		return fmt.Errorf("instore: receipt resend check: %w", err)
	}
	if recent > 0 {
		return nil
	}
	url, err := s.Issue(ctx, storeID, o.ID)
	if err != nil {
		return err
	}
	return s.mailer.SendInStoreReceipt(ctx, o.ID, url)
}

// maskEmail shows enough to recognise the address: "j***@gmail.com".
func maskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at < 1 {
		return ""
	}
	return e[:1] + "***" + e[at:]
}
