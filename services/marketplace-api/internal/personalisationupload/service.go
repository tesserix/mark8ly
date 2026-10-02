package personalisationupload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ErrUploadsDisabled is returned by every entry point when no private
// bucket is configured. Handlers translate it to 501.
//
// A sentinel rather than an apperrors code because this service has no
// NotImplemented code and the existing 501s (admin media, branding) are
// written by the handler too — adding a code would change the shared
// error mapping for one caller's benefit.
var ErrUploadsDisabled = errors.New("personalisationupload: no private bucket configured")

// MaxUploadsPerCart bounds how many objects one cart token can create.
//
// The signed PUT cannot cap the object's SIZE (see Confirm), so this caps
// the COUNT instead. Without it, a signed URL endpoint reachable by any
// shopper is an open invitation to fill a bucket. 60 is far above what a
// real cart needs — ten personalised lines with a six-image field — and
// far below what makes a dent.
const MaxUploadsPerCart = 60

// SignedURLTTL is how long a buyer has to complete one PUT.
const SignedURLTTL = 15 * time.Minute

// Service owns the buyer upload lifecycle.
type Service struct {
	db       *gorm.DB
	repo     Repository
	products product.Repository
	// uploader writes to the PRIVATE bucket. Nil when none is configured,
	// which disables buyer uploads entirely rather than falling back to
	// the public product-media bucket.
	uploader media.Uploader
	bucket   string
	maxBytes int64
	logger   *slog.Logger
}

// Config bundles Service dependencies.
type Config struct {
	DB       *gorm.DB
	Repo     Repository
	Products product.Repository
	// Uploader and Bucket MUST refer to the private bucket. Passing the
	// public product-media bucket here would publish buyer artwork.
	Uploader media.Uploader
	Bucket   string
	// MaxBytes caps one object, enforced at Confirm. Zero means 15 MiB.
	MaxBytes int64
	Logger   *slog.Logger
}

// NewService constructs a Service.
func NewService(cfg Config) *Service {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 15 << 20
	}
	return &Service{
		db: cfg.DB, repo: cfg.Repo, products: cfg.Products,
		uploader: cfg.Uploader, bucket: cfg.Bucket,
		maxBytes: cfg.MaxBytes, logger: cfg.Logger,
	}
}

// Enabled reports whether buyer uploads are configured.
//
// False means no private bucket, and every entry point returns
// NotImplemented rather than writing to the public bucket. Failing closed
// is the point: a fallback would put a photograph of someone's child
// somewhere anyone holding the URL could read.
func (s *Service) Enabled() bool {
	return s != nil && s.uploader != nil && s.bucket != ""
}

// BuildKey returns the object key for a buyer upload.
//
// Under `buyer-uploads/` rather than the `tenants/` prefix product media
// uses, so the two can never be confused by a reaper, a lifecycle rule or
// a human reading a bucket listing. The cart token is in the path so an
// operator can see whose work an orphan belonged to without a database.
func BuildKey(tenantID, cartToken, uploadID, filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	safe := make([]byte, 0, len(ext))
	for _, r := range ext {
		if r == '.' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			safe = append(safe, byte(r))
		}
	}
	if len(safe) == 0 || safe[0] != '.' {
		safe = append([]byte("."), safe...)
	}
	return fmt.Sprintf("buyer-uploads/%s/%s/%s%s", tenantID, cartToken, uploadID, string(safe))
}

// CreateRequest asks for a signed PUT.
type CreateRequest struct {
	StoreID     string
	TenantID    string
	ProductID   string
	FieldID     string
	CartToken   string
	Filename    string
	ContentType string
	ContentHash string
}

// CreateResult is what the buyer's browser needs to perform the upload.
type CreateResult struct {
	UploadID  string    `json:"upload_id"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CreateUploadURL validates the request and issues a signed PUT.
//
// The field must belong to a live product IN THIS STORE and be an image
// field. Without that check an upload id could be created against another
// merchant's product, and the row would later be attached to a line that
// had nothing to do with it.
func (s *Service) CreateUploadURL(ctx context.Context, req CreateRequest) (*CreateResult, error) {
	if !s.Enabled() {
		return nil, ErrUploadsDisabled
	}
	if !IsAllowedContentType(req.ContentType) {
		return nil, apperrors.ValidationFailed("content_type",
			"images must be JPEG, PNG or WebP. HEIC photos need converting first.")
	}
	if strings.TrimSpace(req.Filename) == "" {
		return nil, apperrors.ValidationFailed("filename", "filename is required")
	}
	if _, err := uuid.Parse(req.CartToken); err != nil {
		return nil, apperrors.ValidationFailed("cart_token", "cart_token must be a uuid")
	}

	field, err := s.products.GetPersonalisationField(ctx, req.FieldID, req.StoreID, req.TenantID)
	if err != nil {
		return nil, err
	}
	if field.Kind != product.PersonalisationKindImage {
		return nil, apperrors.ValidationFailed("field_id", "this field does not take an image")
	}
	if field.ProductID != req.ProductID {
		return nil, apperrors.ValidationFailed("field_id", "this field does not belong to that product")
	}

	n, err := s.repo.CountForCart(ctx, req.CartToken)
	if err != nil {
		return nil, err
	}
	if n >= MaxUploadsPerCart {
		return nil, apperrors.ValidationFailed("cart",
			"this cart has too many uploads; remove some before adding more")
	}

	signer, ok := s.uploader.(media.SignedURLGenerator)
	if !ok {
		return nil, ErrUploadsDisabled
	}

	id := uuid.NewString()
	key := BuildKey(req.TenantID, req.CartToken, id, req.Filename)
	url, expiresAt, err := signer.SignedUploadURL(ctx, key, req.ContentType, SignedURLTTL)
	if err != nil {
		return nil, err
	}

	row := &Upload{
		ID: id, TenantID: req.TenantID, StoreID: req.StoreID,
		ProductID: req.ProductID, FieldID: req.FieldID, CartToken: req.CartToken,
		StorageKeyOriginal: key,
		ContentHash:        req.ContentHash,
		ContentType:        req.ContentType,
		OriginalFilename:   req.Filename,
		State:              StatePending,
		ExpiresAt:          time.Now().Add(TTL),
	}
	if err := s.repo.Insert(ctx, row); err != nil {
		return nil, err
	}
	return &CreateResult{UploadID: id, URL: url, ExpiresAt: expiresAt}, nil
}

// Confirm checks that the object really arrived and is what it claimed.
//
// THIS IS WHERE SIZE IS ENFORCED, and it has to be, because a V4 signed
// PUT cannot cap length: the signature covers the method, key, content
// type and expiry, and nothing else. So the object is accepted, measured,
// and destroyed if it is over the cap.
//
// The residual risk is real and bounded rather than removed: inside the
// URL's 15-minute life a hostile client can write up to the cap
// repeatedly. MaxUploadsPerCart and the bucket lifecycle rule are what
// bound it.
func (s *Service) Confirm(ctx context.Context, id, cartToken string, w, h *int) (*Upload, error) {
	if !s.Enabled() {
		return nil, ErrUploadsDisabled
	}
	up, err := s.repo.GetForCart(ctx, id, cartToken)
	if err != nil {
		return nil, err
	}

	attrs, err := s.uploader.Verify(ctx, up.StorageKeyOriginal)
	if err != nil {
		return nil, apperrors.UploadNotFound(up.StorageKeyOriginal)
	}

	if attrs.Size > s.maxBytes {
		// Destroy it rather than leave an over-cap object paid for by us
		// and reachable by its signed URL.
		s.destroy(ctx, up.StorageKeyOriginal)
		if _, delErr := s.repo.DeleteForCart(ctx, id, cartToken); delErr != nil {
			s.logger.Error("personalisationupload: could not drop over-cap row",
				"upload_id", id, "err", delErr)
		}
		return nil, apperrors.ValidationFailed("file",
			fmt.Sprintf("that image is too large. The limit is %d MB.", s.maxBytes>>20))
	}
	if !IsAllowedContentType(attrs.ContentType) {
		s.destroy(ctx, up.StorageKeyOriginal)
		if _, delErr := s.repo.DeleteForCart(ctx, id, cartToken); delErr != nil {
			s.logger.Error("personalisationupload: could not drop wrong-type row",
				"upload_id", id, "err", delErr)
		}
		return nil, apperrors.ValidationFailed("content_type",
			"images must be JPEG, PNG or WebP")
	}

	if err := s.repo.MarkVerified(ctx, id, cartToken, attrs.Size, attrs.ContentType, w, h); err != nil {
		return nil, err
	}
	return s.repo.GetForCart(ctx, id, cartToken)
}

// CropResult carries the signed URLs a browser needs to re-crop.
type CropResult struct {
	// SourceURL is a short-lived GET for the PRISTINE original, so the
	// crop is always re-derived from full quality rather than compounding
	// loss across repeated crops.
	SourceURL string    `json:"source_url"`
	UploadURL string    `json:"upload_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PrepareCrop issues the pair of URLs the browser needs, and records the
// rectangle. The original is never rewritten.
func (s *Service) PrepareCrop(ctx context.Context, id, cartToken string, rect CropRect) (*CropResult, error) {
	if !s.Enabled() {
		return nil, ErrUploadsDisabled
	}
	if rect.W <= 0 || rect.H <= 0 || rect.X < 0 || rect.Y < 0 {
		return nil, apperrors.ValidationFailed("crop", "crop must be a positive rectangle")
	}
	up, err := s.repo.GetForCart(ctx, id, cartToken)
	if err != nil {
		return nil, err
	}

	putSigner, ok := s.uploader.(media.SignedURLGenerator)
	if !ok {
		return nil, ErrUploadsDisabled
	}
	getSigner, ok := s.uploader.(media.SignedReadURLGenerator)
	if !ok {
		return nil, ErrUploadsDisabled
	}

	previewKey := up.StorageKeyOriginal + ".preview-" + uuid.NewString()[:8] + ".jpg"
	putURL, expiresAt, err := putSigner.SignedUploadURL(ctx, previewKey, "image/jpeg", SignedURLTTL)
	if err != nil {
		return nil, err
	}
	getURL, _, err := getSigner.SignedReadURL(ctx, up.StorageKeyOriginal, SignedURLTTL)
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(rect)
	if err != nil {
		return nil, apperrors.ValidationFailed("crop", "crop could not be encoded")
	}
	if err := s.repo.SetCrop(ctx, id, cartToken, previewKey, encoded); err != nil {
		return nil, err
	}
	return &CropResult{SourceURL: getURL, UploadURL: putURL, ExpiresAt: expiresAt}, nil
}

// PreviewURL hands the buyer a short-lived GET for their own image.
//
// Short-lived and signed because the bucket is private: this is the only
// way the shopper sees what they uploaded, and the URL must not be
// something that keeps working if it leaks.
func (s *Service) PreviewURL(ctx context.Context, id, cartToken string) (string, time.Time, error) {
	if !s.Enabled() {
		return "", time.Time{}, ErrUploadsDisabled
	}
	up, err := s.repo.GetForCart(ctx, id, cartToken)
	if err != nil {
		return "", time.Time{}, err
	}
	signer, ok := s.uploader.(media.SignedReadURLGenerator)
	if !ok {
		return "", time.Time{}, ErrUploadsDisabled
	}
	// Prefer the preview; fall back to the original when no crop happened.
	key := up.StorageKeyOriginal
	if up.StorageKey != nil && *up.StorageKey != "" {
		key = *up.StorageKey
	}
	return signerRead(ctx, signer, key)
}

func signerRead(ctx context.Context, s media.SignedReadURLGenerator, key string) (string, time.Time, error) {
	url, exp, err := s.SignedReadURL(ctx, key, SignedURLTTL)
	if err != nil {
		return "", time.Time{}, err
	}
	return url, exp, nil
}

// Delete removes an upload the buyer changed their mind about, and its
// objects. A claimed upload is refused: an order depends on it.
func (s *Service) Delete(ctx context.Context, id, cartToken string) error {
	if !s.Enabled() {
		return ErrUploadsDisabled
	}
	up, err := s.repo.DeleteForCart(ctx, id, cartToken)
	if err != nil {
		return err
	}
	s.destroyBoth(ctx, up)
	return nil
}

// destroyBoth removes an upload's original and preview objects.
func (s *Service) destroyBoth(ctx context.Context, up *Upload) {
	s.destroy(ctx, up.StorageKeyOriginal)
	if up.StorageKey != nil && *up.StorageKey != "" {
		s.destroy(ctx, *up.StorageKey)
	}
}

// destroy deletes one object, best-effort. A failure is logged and the
// row still goes: a stranded object costs storage, a stranded row costs
// the buyer their ability to re-upload.
func (s *Service) destroy(ctx context.Context, key string) {
	d, ok := s.uploader.(media.Deleter)
	if !ok || key == "" {
		return
	}
	if err := d.Delete(ctx, key); err != nil {
		s.logger.Error("personalisationupload: object outlived its row",
			"storage_key", key, "err", err)
	}
}
