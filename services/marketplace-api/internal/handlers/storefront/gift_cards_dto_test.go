package storefront

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
)

const testGiftCardCode = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func newTestCard(status giftcard.GiftCardStatus) giftcard.GiftCard {
	return giftcard.GiftCard{
		ID:             uuid.New(),
		Code:           testGiftCardCode,
		Status:         status,
		InitialBalance: decimal.RequireFromString("100.00"),
		CurrentBalance: decimal.RequireFromString("100.00"),
		CurrencyCode:   "AUD",
	}
}

// GET /account/gift-cards was the disclosure channel: anyone could put
// their own address in purchaser_email on the public purchase endpoint,
// abandon the payment, then read the full code of the unpaid card here.
func TestMyGiftCardDTO_MasksCodeOfNonActiveCards(t *testing.T) {
	for _, status := range []giftcard.GiftCardStatus{
		giftcard.StatusPending,
		giftcard.StatusDisabled,
		giftcard.StatusRefunded,
		giftcard.StatusDepleted,
	} {
		t.Run(string(status), func(t *testing.T) {
			dto := myGiftCardDTO(newTestCard(status))

			got, _ := dto["code_display"].(string)
			if got == giftcard.FormatCodeDisplay(testGiftCardCode) {
				t.Fatalf("code of a %s card was disclosed verbatim", status)
			}
			if got != maskedCodeDisplay {
				t.Fatalf("code_display = %q, want the mask %q", got, maskedCodeDisplay)
			}
			// The row itself must still be visible — the buyer needs to
			// see that the purchase exists and what state it is in.
			if dto["status"] != string(status) {
				t.Fatalf("status = %v, want %q", dto["status"], status)
			}
		})
	}
}

func TestMyGiftCardDTO_DisclosesCodeOfActiveCards(t *testing.T) {
	dto := myGiftCardDTO(newTestCard(giftcard.StatusActive))

	want := giftcard.FormatCodeDisplay(testGiftCardCode)
	if dto["code_display"] != want {
		t.Fatalf("code_display = %v, want %q — a paid, active card must still show its code",
			dto["code_display"], want)
	}
}
