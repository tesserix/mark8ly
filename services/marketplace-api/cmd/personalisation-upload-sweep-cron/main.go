// Command personalisation-upload-sweep-cron destroys buyer-supplied
// artwork that no order ever claimed (#963).
//
// # Unlike the stock-hold sweeper, this one IS load-bearing
//
// stock-hold-sweep-cron is housekeeping: availability is computed, so an
// expired hold already counts for nothing and a missed run only leaves
// dead rows. This is different. Every row here points at a photograph a
// shopper uploaded — often of a person — sitting in a bucket. If this job
// stops running, those accumulate indefinitely, and "we kept pictures of
// your children for a year because a CronJob was broken" is not a
// housekeeping failure.
//
// So a failure here exits non-zero and should page someone eventually,
// even though a single missed run is harmless.
//
// # Without a private bucket it does nothing, deliberately
//
// Uploads cannot exist without MARKETPLACE_PRIVATE_GCS_BUCKET, so neither
// can orphans. Running without one is a successful no-op rather than an
// error: it means the feature is off.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"cloud.google.com/go/storage"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/personalisationupload"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

const runTimeout = 10 * time.Minute

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("personalisation-upload-sweep-cron: DATABASE_URL not set")
		os.Exit(1)
	}
	bucket := os.Getenv("MARKETPLACE_PRIVATE_GCS_BUCKET")
	if bucket == "" {
		log.Info("personalisation-upload-sweep-cron: no private bucket; nothing can have been uploaded")
		return
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("personalisation-upload-sweep-cron: db open failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	gcsCtx, gcsCancel := context.WithTimeout(ctx, 5*time.Second)
	sc, err := storage.NewClient(gcsCtx)
	gcsCancel()
	if err != nil {
		log.Error("personalisation-upload-sweep-cron: gcs client", "err", err)
		os.Exit(1)
	}
	defer func() { _ = sc.Close() }()

	repo := personalisationupload.NewRepository(conn)
	svc := personalisationupload.NewService(personalisationupload.Config{
		DB:       conn,
		Repo:     repo,
		Uploader: media.NewGCSUploader(sc, bucket),
		Bucket:   bucket,
		Logger:   log,
	})

	res, err := personalisationupload.NewSweeper(repo, svc, log).Sweep(ctx)
	if err != nil {
		log.Error("personalisation-upload-sweep-cron: sweep failed", "err", err)
		os.Exit(1)
	}
	log.Info("personalisation-upload-sweep-cron: done",
		"rows", res.RowsDeleted, "objects", res.ObjectsDeleted)
}
