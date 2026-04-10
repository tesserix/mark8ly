package admin

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
)

// IssueGiftCardRequest is the wire DTO for POST /gift-cards.
type IssueGiftCardRequest struct {
	InitialBalance decimal.Decimal `json:"initial_balance" binding:"required"`
	CurrencyCode   string          `json:"currency_code"   binding:"required,len=3"`
	SenderName     *string         `json:"sender_name"`
	SenderEmail    *string         `json:"sender_email"`
	RecipientName  *string         `json:"recipient_name"`
	RecipientEmail *string         `json:"recipient_email"`
	Message        *string         `json:"message"`
	ExpiresAt      *time.Time      `json:"expires_at"`
}

// AdminGiftCardResponse is the wire DTO for gift card list/detail.
type AdminGiftCardResponse struct {
	ID             string          `json:"id"`
	Code           string          `json:"code"`
	CodeDisplay    string          `json:"code_display"`
	InitialBalance decimal.Decimal `json:"initial_balance"`
	CurrentBalance decimal.Decimal `json:"current_balance"`
	CurrencyCode   string          `json:"currency_code"`
	Status         string          `json:"status"`
	SenderName     *string         `json:"sender_name,omitempty"`
	SenderEmail    *string         `json:"sender_email,omitempty"`
	RecipientName  *string         `json:"recipient_name,omitempty"`
	RecipientEmail *string         `json:"recipient_email,omitempty"`
	Message        *string         `json:"message,omitempty"`
	PurchasedAt    *time.Time      `json:"purchased_at,omitempty"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// AdminGiftCardTxnResponse is the wire DTO for a transaction ledger row.
type AdminGiftCardTxnResponse struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Amount       decimal.Decimal `json:"amount"`
	BalanceAfter decimal.Decimal `json:"balance_after"`
	OrderID      *string         `json:"order_id,omitempty"`
	Note         *string         `json:"note,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// AdminGiftCardDetailResponse includes the card and its transaction ledger.
type AdminGiftCardDetailResponse struct {
	AdminGiftCardResponse
	Transactions []AdminGiftCardTxnResponse `json:"transactions"`
}

func toAdminGiftCardResponse(gc *giftcard.GiftCard) AdminGiftCardResponse {
	return AdminGiftCardResponse{
		ID:             gc.ID.String(),
		Code:           gc.Code,
		CodeDisplay:    giftcard.FormatCodeDisplay(gc.Code),
		InitialBalance: gc.InitialBalance,
		CurrentBalance: gc.CurrentBalance,
		CurrencyCode:   gc.CurrencyCode,
		Status:         string(gc.Status),
		SenderName:     gc.SenderName,
		SenderEmail:    gc.SenderEmail,
		RecipientName:  gc.RecipientName,
		RecipientEmail: gc.RecipientEmail,
		Message:        gc.Message,
		PurchasedAt:    gc.PurchasedAt,
		ExpiresAt:      gc.ExpiresAt,
		CreatedAt:      gc.CreatedAt,
	}
}

func toAdminGiftCardTxnResponse(txn *giftcard.Transaction) AdminGiftCardTxnResponse {
	resp := AdminGiftCardTxnResponse{
		ID:           txn.ID.String(),
		Type:         string(txn.Type),
		Amount:       txn.Amount,
		BalanceAfter: txn.BalanceAfter,
		Note:         txn.Note,
		CreatedAt:    txn.CreatedAt,
	}
	if txn.OrderID != nil {
		s := txn.OrderID.String()
		resp.OrderID = &s
	}
	return resp
}
