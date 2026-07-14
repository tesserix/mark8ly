// Package product — service_media_recrop.go: RecropMedia loads an
// existing product_media row, generates a fresh content-addressed
// storage key for the cropped result, and returns signed URLs so the
// frontend can download the pristine original, apply the crop in a
// canvas, and upload the cropped blob directly to GCS.
//
// The row is NOT mutated here. The client commits by calling the
// existing PATCH /media/:mediaId endpoint with the new storage_key.
// gcs_path_original is never touched so future recrops still read the
// pristine source.
package product

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// RecropMediaRequest is the service-level DTO for POST
// /products/:id/media/:mediaId/recrop.
type RecropMediaRequest struct {
	ProductID   string
	MediaID     string
	StoreID     string
	TenantID    string
	Filename    string
	ContentType string
}

// RecropMediaResponse carries the signed URLs the handler returns to
// the client along with the freshly allocated storage key.
type RecropMediaResponse struct {
	SourceOriginalURL string
	UploadURL         string
	NewStorageKey     string
	ExpiresAt         time.Time
}

// RecropMedia verifies the target media row exists under the given
// tenant/store, then allocates a new content-addressed storage key and
// signs a PUT URL for it plus a GET URL for the pristine original blob.
//
// Returns apperrors.NotFound when the media row cannot be resolved to
// the caller's tenant/store. Returns apperrors.NotImplemented when the
// wired uploader does not implement the signing interfaces (dev fake).
func (s *Service) RecropMedia(
	ctx context.Context,
	req RecropMediaRequest,
	putSigner media.SignedURLGenerator,
	getSigner media.SignedReadURLGenerator,
	ttl time.Duration,
) (*RecropMediaResponse, error) {
	// Load the full aggregate so we can find the media row and verify
	// tenant/store scope in a single query. Cross-tenant lookups return
	// NotFound from the repo.
	agg, err := s.repo.GetByIDForStore(ctx, req.ProductID, req.StoreID, req.TenantID)
	if err != nil {
		return nil, err
	}

	var row *Media
	for i := range agg.Media {
		if agg.Media[i].ID == req.MediaID {
			row = &agg.Media[i]
			break
		}
	}
	if row == nil {
		return nil, apperrors.NotFound("media")
	}
	if row.GcsPathOriginal == "" {
		// Should never happen post-migration 000005, but guard anyway.
		return nil, apperrors.NotFound("media")
	}

	// Allocate a new content-addressed storage key. We don't have a
	// client-supplied content hash yet (the blob doesn't exist), so
	// derive a random 32-hex nonce. This still round-trips through
	// media.BuildStorageKey so the key shape stays identical to fresh
	// uploads.
	nonce, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("product: recrop: nonce: %w", err)
	}
	filename := req.Filename
	if filename == "" {
		filename = "recrop.jpg"
	}
	newKey := media.BuildStorageKey(req.TenantID, nonce, filename)

	contentType := req.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	// Sign a GET URL against the pristine original and a PUT URL
	// against the new key.
	readURL, _, err := getSigner.SignedReadURL(ctx, row.GcsPathOriginal, ttl)
	if err != nil {
		return nil, fmt.Errorf("product: recrop: sign read: %w", err)
	}
	putURL, expiresAt, err := putSigner.SignedUploadURL(ctx, newKey, contentType, ttl)
	if err != nil {
		return nil, fmt.Errorf("product: recrop: sign put: %w", err)
	}

	return &RecropMediaResponse{
		SourceOriginalURL: readURL,
		UploadURL:         putURL,
		NewStorageKey:     newKey,
		ExpiresAt:         expiresAt,
	}, nil
}

// randomHex returns 2n hex characters of cryptographic randomness.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
