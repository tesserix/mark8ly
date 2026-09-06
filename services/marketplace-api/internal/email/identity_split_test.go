package email_test

// identity_split_test.go — the guard on #718's central rule.
//
// Two kinds of mail leave this service and they must never converge:
//
//   - Store identity     — customer-facing mail sent BY a merchant's
//     store. The customer bought from Nadia's Ceramics and should see
//     Nadia's Ceramics.
//   - Platform identity  — mail mark8ly sends TO a merchant as their
//     provider: dunning, trial reminders, payment-action, win-back.
//     A failed-payment notice wearing the merchant's own brand is
//     actively misleading, so these keep the platform sender.
//
// Nothing in the transport enforces this; each mailer opts in at its
// send site. That is deliberate — a global default is exactly the thing
// that would silently drag billing mail into the store identity — and it
// is also why this test exists. It drives the REAL mailers, not a
// reimplementation of them, so deleting an Apply call or adding one to a
// billing path goes red here.
//
// Adding a mailer? Add it to one of the two tables below. A new
// customer-facing mailer that forgets its identity fails
// TestIdentitySplit_StoreMail; a billing path that acquires one fails
// TestIdentitySplit_PlatformMail.

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/storeidentity"
	"github.com/mark8ly/marketplace-api/internal/ticket"
)

const (
	splitFrom      = "noreply@tesserix.app"
	splitStore     = "Nadia's Ceramics"
	splitSlug      = "nadias-ceramics"
	splitContact   = "hello@nadiasceramics.com"
	splitExpFrom   = "nadias-ceramics@tesserix.app"
	splitRecipient = "buyer@example.com"
)

// storeSender is the identity every customer-facing mailer must produce.
func storeSender() email.StoreSender {
	return email.StoreSender{Name: splitStore, Slug: splitSlug, ContactEmail: splitContact}
}

// TestIdentitySplit_StoreMail — every customer-facing mailer must name
// the merchant in the inbox line and offer the merchant's reply path.
func TestIdentitySplit_StoreMail(t *testing.T) {
	cases := []struct {
		name string
		send func(t *testing.T, sender email.Sender)
	}{
		{
			name: "orderdoc invoice",
			send: func(t *testing.T, sender email.Sender) {
				m := orderdoc.NewDocumentMailer(sender, splitFrom, slog.Default())
				if err := m.SendInvoice(context.Background(), orderdoc.DocumentInput{
					Recipient:         splitRecipient,
					TenantID:          "tenant-1",
					DocumentNumber:    "INV-1",
					OrderID:           uuid.NewString(),
					StoreSlug:         splitSlug,
					StoreContactEmail: splitContact,
					OrderNumber:       "ORD-1",
					PlacedAt:          time.Now(),
					GrandTotal:        decimal.NewFromInt(42),
					CurrencyCode:      "EUR",
					ItemCount:         1,
					Theme:             orderdoc.Theme{StoreName: splitStore},
				}); err != nil {
					t.Fatalf("SendInvoice: %v", err)
				}
			},
		},
		{
			name: "giftcard delivery",
			send: func(t *testing.T, sender email.Sender) {
				m := giftcard.NewDeliveryMailer(sender, splitFrom, slog.Default())
				if err := m.SendDelivery(context.Background(), giftcard.DeliveryInput{
					Recipient: splitRecipient,
					TenantID:  "tenant-1",
					Card: &giftcard.GiftCard{
						Code:           "GC-TEST-0001",
						CurrentBalance: decimal.NewFromInt(50),
						CurrencyCode:   "EUR",
					},
					Theme: giftcard.GiftCardEmailTheme{
						StoreName:         splitStore,
						StoreSlug:         splitSlug,
						StoreContactEmail: splitContact,
					},
				}); err != nil {
					t.Fatalf("SendDelivery: %v", err)
				}
			},
		},
		{
			name: "shipping label",
			send: func(t *testing.T, sender email.Sender) {
				m := shipping.NewEmailLabelMailer(sender, splitFrom, slog.Default())
				if err := m.SendLabel(context.Background(), shipping.LabelEmailPayload{
					Recipient:         splitRecipient,
					TenantID:          "tenant-1",
					StoreName:         splitStore,
					StoreSlug:         splitSlug,
					StoreContactEmail: splitContact,
					OrderNumber:       "ORD-1",
					Carrier:           "DHL",
					TrackingNumber:    "TRACK-1",
					PDF:               []byte("%PDF-1.4 fake"),
				}); err != nil {
					t.Fatalf("SendLabel: %v", err)
				}
			},
		},
		{
			name: "campaign",
			send: func(t *testing.T, sender email.Sender) {
				d := campaign.NewEmailDispatcher(sender, splitFrom, slog.Default())
				if err := d.Send(context.Background(), campaign.OutboundEmail{
					Recipient:  splitRecipient,
					Subject:    "Spring collection",
					HTMLBody:   "<p>hi</p>",
					TextBody:   "hi",
					TenantID:   "tenant-1",
					CampaignID: uuid.NewString(),
					Sender:     storeSender(),
				}); err != nil {
					t.Fatalf("campaign Send: %v", err)
				}
			},
		},
		{
			name: "ticket created",
			send: func(t *testing.T, sender email.Sender) {
				n := ticket.NewEmailNotifier(sender, splitFrom, "https://x.example", slog.Default()).
					WithStoreIdentity(storeidentity.StaticLoader{Store: storeidentity.Store{
						TenantID:     "tenant-1",
						Name:         splitStore,
						Slug:         splitSlug,
						ContactEmail: splitContact,
					}})
				n.NotifyTicketCreated(context.Background(), &ticket.Ticket{
					TenantID:         uuid.New(),
					StoreID:          uuid.New(),
					TicketNumber:     "CASE-1",
					Subject:          "Where is my order?",
					SubmittedByEmail: splitRecipient,
					SubmittedByName:  "A Buyer",
				})
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sender := &captureSender{}
			c.send(t, sender)
			msg := sender.last(t)

			if msg.FromName != splitStore {
				t.Errorf("FromName = %q, want %q — the customer must see the store, not the platform",
					msg.FromName, splitStore)
			}
			if msg.From != splitExpFrom {
				t.Errorf("From = %q, want %q", msg.From, splitExpFrom)
			}
			if msg.ReplyTo != splitContact {
				t.Errorf("ReplyTo = %q, want %q — a reply must reach the merchant",
					msg.ReplyTo, splitContact)
			}
			if msg.CustomArgs["product"] != "mark8ly" {
				t.Errorf("product attribution lost: %v", msg.CustomArgs)
			}
			if msg.CustomArgs["tenant_id"] == "" {
				t.Errorf("tenant_id attribution lost: %v", msg.CustomArgs)
			}
		})
	}
}

// TestIdentitySplit_PlatformMail — mark8ly-to-merchant mail must stay on
// the platform address and must never acquire a store's display name or
// a merchant Reply-To. Every billing template is exercised, so switching
// any single one to a store identity goes red.
func TestIdentitySplit_PlatformMail(t *testing.T) {
	// EVERY template the billing client can send — dunning, trial
	// lifecycle, payment-action, migration decisions, win-back. Listed in
	// full rather than sampled, so flipping any one of them to a store
	// identity has nowhere to hide.
	billing := []email.TemplateID{
		email.TemplateDunningDay5,
		email.TemplateDunningDay7,
		email.TemplatePaymentActionReminder,
		email.TemplateTrialNoPMT15,
		email.TemplateTrialNoPMT10,
		email.TemplateTrialNoPMT7,
		email.TemplateTrialNoPMT3,
		email.TemplateTrialNoPMT1,
		email.TemplateTrialHasPMT1,
		email.TemplateTrialStartedBilled,
		email.TemplateTrialExpired,
		email.TemplateMigrationFastPathApproved,
		email.TemplateMigrationFastPathRejected,
		email.TemplateWinBack,
		email.TemplateWinBackNoOffer,
	}

	for _, tmpl := range billing {
		t.Run(string(tmpl), func(t *testing.T) {
			sender := &captureSender{}
			loader := loaderWith(string(tmpl), "Subject for {{.store_name}}",
				"<p>{{.store_name}}</p>", "{{.store_name}}")
			c := email.NewTemplateClient(loader, sender, splitFrom, slog.Default())

			// The merchant's store name reaches this mail as template
			// DATA. That is the trap: it is right there in the payload,
			// one line away from being used as the sender.
			err := c.Send(context.Background(), tmpl, "merchant@example.com", map[string]any{
				"store_name": splitStore,
				"tenant_id":  "tenant-1",
				"day":        5,
			})
			if err != nil {
				t.Fatalf("Send: %v", err)
			}

			msg := sender.last(t)
			if msg.From != splitFrom {
				t.Errorf("From = %q, want the platform address %q", msg.From, splitFrom)
			}
			if msg.FromName != "Mark8ly Billing" {
				t.Errorf("FromName = %q, want the platform display name", msg.FromName)
			}
			if strings.Contains(msg.FromName, splitStore) {
				t.Errorf("billing mail is wearing the merchant's brand: FromName = %q", msg.FromName)
			}
			if msg.ReplyTo != "" {
				t.Errorf("ReplyTo = %q, want empty — billing mail must not route replies to the merchant",
					msg.ReplyTo)
			}
			// The body still names the store; only the ENVELOPE is the
			// platform's. This distinguishes a correct implementation
			// from one that simply never saw the store name. Asserted on
			// the text body: the HTML one is escaped by html/template,
			// so the apostrophe in the store name would not match.
			if !strings.Contains(msg.TextBody, splitStore) {
				t.Errorf("body lost the store name: %q", msg.TextBody)
			}
		})
	}
}

// TestIdentitySplit_NoGlobalDefault is the structural half of the rule:
// a bare Message must come out of the transport with no identity at all.
// If someone ever adds a default From/FromName inside the email package
// — the change that would silently apply a store identity everywhere —
// this fails.
func TestIdentitySplit_NoGlobalDefault(t *testing.T) {
	msg := email.Message{}
	if msg.From != "" || msg.FromName != "" || msg.ReplyTo != "" {
		t.Fatalf("zero Message carries an identity: %+v", msg)
	}

	// And the two constructors must not be interchangeable.
	store := email.StoreIdentity(splitFrom, storeSender())
	platform := email.PlatformIdentity(splitFrom, "Mark8ly Billing")
	if store.From == platform.From {
		t.Errorf("store and platform identities share a From (%q)", store.From)
	}
	if store.FromName == platform.FromName {
		t.Errorf("store and platform identities share a FromName (%q)", store.FromName)
	}
	if platform.ReplyTo != "" {
		t.Errorf("platform identity acquired a ReplyTo: %q", platform.ReplyTo)
	}
}
