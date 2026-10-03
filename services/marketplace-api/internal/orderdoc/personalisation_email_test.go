package orderdoc

import (
	"context"
	"strings"
	"testing"
)

// The buyer-facing half of #968.
//
// The confirmation email is the last moment a typo is cheap. Once the
// merchant has engraved "Ashar" the fix is a refund and a remake, and the
// buyer is the only person in the loop who can tell that it is wrong — so
// the email has to show them their own words back, and has to invite a
// correction rather than just reciting the order.
//
// The other half of this is a negative: no signed URL may appear. A link
// to a buyer's photograph in an inbox outlives its own expiry as a dead
// link and is not ours to forward.

func inputWithPersonalisation() DocumentInput {
	in := sampleInput("ORD-5150")
	in.Personalisation = []PersonalisationLine{
		{Item: "Custom figurine", Label: "Name to engrave", Value: "Asha"},
		{Item: "Custom figurine", Label: "Your photo", Value: "nana.jpg"},
	}
	return in
}

func TestRender_Invoice_EchoesPersonalisation(t *testing.T) {
	_, html, text, err := render(
		context.Background(), nil, KindInvoice, inputWithPersonalisation(), false)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for name, body := range map[string]string{"html": html, "text": text} {
		for _, want := range []string{
			"Name to engrave", "Asha", "Your photo", "nana.jpg",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s body missing %q — the buyer cannot check their own order", name, want)
			}
		}
		// The invitation to correct it is the point of the block.
		if !strings.Contains(strings.ToLower(body), "mistake") {
			t.Errorf("%s body does not invite a correction", name)
		}
	}
}

func TestRender_Invoice_SuppressesEmptyPersonalisation(t *testing.T) {
	// An ordinary order must not grow an empty "Your personalisation"
	// heading — it reads as a rendering bug to the person who receives it.
	in := sampleInput("ORD-5151")
	in.Personalisation = nil

	_, html, text, err := render(context.Background(), nil, KindInvoice, in, false)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for name, body := range map[string]string{"html": html, "text": text} {
		if strings.Contains(strings.ToLower(body), "personalisation") {
			t.Errorf("%s body rendered the personalisation block for an ordinary order", name)
		}
	}
}

func TestRender_Invoice_NeverEmbedsASignedURL(t *testing.T) {
	in := inputWithPersonalisation()
	// Whatever the Service hands over, the template must not turn it into
	// a link. This is the shape the loadPersonalisation contract promises
	// — a description, not a URL — and the assertion is here so a future
	// "helpful" change to either side trips a test.
	_, html, text, err := render(context.Background(), nil, KindInvoice, in, false)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for name, body := range map[string]string{"html": html, "text": text} {
		for _, leak := range []string{
			"storage.googleapis.com", "X-Goog-Signature", "buyer-uploads/",
		} {
			if strings.Contains(body, leak) {
				t.Errorf("%s body contains %q — a buyer's artwork URL in an inbox", name, leak)
			}
		}
	}
}
