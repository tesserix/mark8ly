// Package orderdoc — storefront_pdf_fetcher.go
//
// Concrete StorefrontPDFFetcher that calls the storefront's
// /api/internal/orders/:id/{invoice,receipt} routes over HTTP. The
// route is gated by the same MARKETPLACE_INTERNAL_AUTH_SECRET that
// admin → marketplace-api uses, so we get service-to-service auth for
// free. PDF rendering stays in TS (single source of truth — see the
// mailer package comment) and we just stream the bytes back here so
// SendGrid can attach them.

package orderdoc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPStorefrontPDFFetcher implements StorefrontPDFFetcher by calling
// the storefront's internal PDF render routes.
type HTTPStorefrontPDFFetcher struct {
	// BaseURLTemplate is the storefront URL with a {slug} placeholder.
	// Same value as the storefrontURLBase used to build OrderURL.
	BaseURLTemplate string
	// SharedSecret is forwarded as X-Internal-Auth on every request
	// and must equal the storefront's MARKETPLACE_INTERNAL_AUTH_SECRET.
	SharedSecret string
	// Client overrides the default http.Client (mainly for tests).
	Client *http.Client
}

// NewHTTPStorefrontPDFFetcher constructs a fetcher with sensible
// defaults. Returns nil if either input is empty — the caller treats
// nil as "PDF attachments disabled".
func NewHTTPStorefrontPDFFetcher(baseURLTemplate, sharedSecret string) *HTTPStorefrontPDFFetcher {
	if baseURLTemplate == "" || sharedSecret == "" {
		return nil
	}
	return &HTTPStorefrontPDFFetcher{
		BaseURLTemplate: baseURLTemplate,
		SharedSecret:    sharedSecret,
		Client:          &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchInvoicePDF GETs /api/internal/orders/:id/invoice on the
// storefront and returns the rendered bytes.
func (f *HTTPStorefrontPDFFetcher) FetchInvoicePDF(ctx context.Context, in DocumentInput) ([]byte, error) {
	return f.fetch(ctx, "invoice", in)
}

// FetchReceiptPDF mirrors FetchInvoicePDF for the post-delivery
// receipt PDF.
func (f *HTTPStorefrontPDFFetcher) FetchReceiptPDF(ctx context.Context, in DocumentInput) ([]byte, error) {
	return f.fetch(ctx, "receipt", in)
}

func (f *HTTPStorefrontPDFFetcher) fetch(ctx context.Context, kind string, in DocumentInput) ([]byte, error) {
	if in.OrderID == "" {
		return nil, fmt.Errorf("orderdoc: missing order_id on document input")
	}
	if in.StoreSlug == "" {
		return nil, fmt.Errorf("orderdoc: missing store_slug on document input")
	}
	base := strings.TrimRight(strings.ReplaceAll(f.BaseURLTemplate, "{slug}", in.StoreSlug), "/")
	q := url.Values{}
	q.Set("slug", in.StoreSlug)
	if in.Recipient != "" {
		q.Set("customer_email", in.Recipient)
	}
	target := fmt.Sprintf("%s/api/internal/orders/%s/%s?%s", base, in.OrderID, kind, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("orderdoc: build pdf request: %w", err)
	}
	req.Header.Set("X-Internal-Auth", f.SharedSecret)
	req.Header.Set("Accept", "application/pdf")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("orderdoc: pdf GET: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("orderdoc: storefront pdf returned %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}
