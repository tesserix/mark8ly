package csvjob

import (
	"context"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
)

// GCSStore is the object-storage half of CSV import.
//
// The worker has always been able to read a CSV and write an error CSV —
// through the CSVReader / ErrorWriterFactory interfaces — but the only
// implementations lived in tests, so the package was green while the
// feature could not run (#897). Worse, the submit handler computed a
// gcs_path and discarded the uploaded bytes, so even a working reader
// would have found nothing at that path.
//
// This provides all three halves against one bucket: Upload for the
// handler, Open for the worker, and New for the error CSV.
type GCSStore struct {
	client *storage.Client
	bucket string
}

// NewGCSStore constructs a store over an existing storage client. The
// client is shared with the media uploader — CSV import has no reason to
// open a second connection.
func NewGCSStore(client *storage.Client, bucket string) *GCSStore {
	return &GCSStore{client: client, bucket: bucket}
}

// Upload writes the CSV body at path. Called by the submit handler before
// the job row is created: a queued job whose object does not exist is a
// job that can only fail, so the write has to happen first.
func (s *GCSStore) Upload(ctx context.Context, path string, r io.Reader) error {
	w := s.client.Bucket(s.bucket).Object(path).NewWriter(ctx)
	w.ContentType = "text/csv"
	if _, err := io.Copy(w, r); err != nil {
		// Close reports the commit error; abandon the object either way so a
		// half-written CSV is never left behind for the worker to parse.
		_ = w.Close()
		return fmt.Errorf("csvjob: upload %s: %w", path, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("csvjob: finalise %s: %w", path, err)
	}
	return nil
}

// Open implements CSVReader.
func (s *GCSStore) Open(ctx context.Context, gcsPath string) (io.ReadCloser, error) {
	rc, err := s.client.Bucket(s.bucket).Object(gcsPath).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("csvjob: open %s: %w", gcsPath, err)
	}
	return rc, nil
}

// New implements ErrorWriterFactory, wrapping a GCS object writer in the
// CSV encoder the worker already uses.
func (s *GCSStore) New(ctx context.Context, gcsPath string) (ErrorCSVWriter, error) {
	w := s.client.Bucket(s.bucket).Object(gcsPath).NewWriter(ctx)
	w.ContentType = "text/csv"
	return NewErrorCSVWriter(w), nil
}
