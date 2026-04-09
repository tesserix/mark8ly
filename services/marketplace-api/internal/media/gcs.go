// services/marketplace-api/internal/media/gcs.go
package media

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

// BuildStorageKey returns the canonical content-addressed object key for
// a tenant's product media upload. Per spec §14.9 the hash + filename
// shape is the stable contract; changing it requires a migration.
//
// The filename is sanitized to ASCII letters/digits/dot/dash/underscore.
// Any other character becomes a dash. Leading slashes are stripped.
func BuildStorageKey(tenantID, contentHash, filename string) string {
	filename = strings.TrimLeft(filename, "/")
	var b strings.Builder
	for _, r := range filename {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	safe := b.String()
	if safe == "" {
		safe = "file"
	}
	return fmt.Sprintf("tenants/%s/products/media/%s/%s", tenantID, contentHash, safe)
}

// GCSUploader is the real Uploader implementation backed by a GCS
// bucket. Construct via NewGCSUploader; wire in main.go when
// MARKETPLACE_GCS_BUCKET is set.
type GCSUploader struct {
	bucket     *storage.BucketHandle
	bucketName string
}

// NewGCSUploader constructs a GCSUploader. The caller is responsible
// for closing the underlying *storage.Client.
func NewGCSUploader(client *storage.Client, bucketName string) *GCSUploader {
	return &GCSUploader{
		bucket:     client.Bucket(bucketName),
		bucketName: bucketName,
	}
}

// Verify implements Uploader. Returns ErrNotFound when the object does
// not exist. Any other error is wrapped with context.
func (u *GCSUploader) Verify(ctx context.Context, key string) (*Attrs, error) {
	attrs, err := u.bucket.Object(key).Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("media: gcs attrs: %w", err)
	}
	return &Attrs{
		StorageKey:  key,
		Size:        attrs.Size,
		ContentType: attrs.ContentType,
	}, nil
}

// SignedURLGenerator is the interface the /media/upload-url handler
// type-asserts against. The real GCSUploader implements it; the
// FakeUploader does not (the endpoint returns 501 in dev).
type SignedURLGenerator interface {
	SignedUploadURL(ctx context.Context, key, contentType string, expires time.Duration) (string, time.Time, error)
}

// SignedUploadURL generates a V4 signed PUT URL the frontend uses to
// upload the object directly to GCS. Returns (url, expiresAt, error).
func (u *GCSUploader) SignedUploadURL(ctx context.Context, key, contentType string, expires time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().Add(expires)
	url, err := u.bucket.SignedURL(key, &storage.SignedURLOptions{
		Method:      "PUT",
		ContentType: contentType,
		Expires:     expiresAt,
		Scheme:      storage.SigningSchemeV4,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("media: signed url: %w", err)
	}
	return url, expiresAt, nil
}

// SignedReadURLGenerator is the interface the /media/:mediaId/recrop
// handler uses to hand the client a short-lived GET URL for the
// pristine original blob. Real GCSUploader implements it; the dev
// FakeUploader does not (recrop returns 501 in dev).
type SignedReadURLGenerator interface {
	SignedReadURL(ctx context.Context, key string, expires time.Duration) (string, time.Time, error)
}

// SignedReadURL generates a V4 signed GET URL for the given object key.
// Used by the recrop flow to let the client download the pristine
// original so it can re-crop against it in the browser.
func (u *GCSUploader) SignedReadURL(ctx context.Context, key string, expires time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().Add(expires)
	url, err := u.bucket.SignedURL(key, &storage.SignedURLOptions{
		Method:  "GET",
		Expires: expiresAt,
		Scheme:  storage.SigningSchemeV4,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("media: signed read url: %w", err)
	}
	return url, expiresAt, nil
}

// Compile-time checks.
var (
	_ Uploader               = (*GCSUploader)(nil)
	_ SignedURLGenerator     = (*GCSUploader)(nil)
	_ SignedReadURLGenerator = (*GCSUploader)(nil)
)
