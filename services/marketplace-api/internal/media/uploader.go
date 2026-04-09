// Package media owns the interface through which marketplace-api's service
// layer verifies that uploaded media objects exist in GCS. The real GCS-
// backed implementation lands in M5; M3 ships the interface and a fake
// implementation for tests.
package media

import (
	"context"
	"errors"
)

// Attrs is the subset of GCS object attributes the service layer cares
// about pre-transaction. See spec §14.9.
type Attrs struct {
	StorageKey  string
	Size        int64
	ContentType string
}

// ErrNotFound signals the object does not exist at the given storage_key.
var ErrNotFound = errors.New("media: upload not found")

// Uploader verifies that an uploaded object exists and returns its metadata.
// The real implementation calls GCS HEAD; M3's fake holds an in-memory map.
type Uploader interface {
	Verify(ctx context.Context, storageKey string) (*Attrs, error)
}

// FakeUploader is an in-memory Uploader used by tests. Callers register
// expected storage keys via Register() before invoking service code.
type FakeUploader struct {
	attrs map[string]*Attrs
}

// NewFakeUploader returns a FakeUploader with an empty registry.
func NewFakeUploader() *FakeUploader { return &FakeUploader{attrs: map[string]*Attrs{}} }

// Register seeds the fake with an attrs record.
func (f *FakeUploader) Register(a Attrs) {
	copy := a
	f.attrs[a.StorageKey] = &copy
}

// Verify implements Uploader.
func (f *FakeUploader) Verify(_ context.Context, key string) (*Attrs, error) {
	a, ok := f.attrs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return a, nil
}
