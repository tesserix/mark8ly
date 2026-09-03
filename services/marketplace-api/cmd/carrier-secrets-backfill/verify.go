package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
)

// VerifyResult is the census Verify produces.
type VerifyResult struct {
	Examined int
	Resolved int
	Failed   int
	// ByScheme counts references by reference prefix: "bao", "gsm",
	// "inline", "empty", or "unknown". This is the decision input for
	// mark8ly#621 — a non-zero "gsm" count means GCP Secret Manager is
	// still serving at least one live credential and must not be deleted.
	ByScheme map[string]int
}

// referenceScheme classifies a stored reference by prefix. It never returns
// any part of the reference itself: an unrecognised value can be a raw
// pre-encryption credential, so it must not reach logs.
func referenceScheme(ref string) string {
	switch {
	case ref == "":
		return "empty"
	case carriersecrets.IsBaoRef(ref):
		return "bao"
	case carriersecrets.IsGSMRef(ref):
		return "gsm"
	case carriersecrets.IsInlineRef(ref):
		return "inline"
	default:
		return "unknown"
	}
}

// Verify reads every stored carrier-credential reference through the real
// Store and reports whether each one still resolves, plus a census by
// reference scheme.
//
// It exists for mark8ly#621. Retiring GCP Secret Manager rests on the claim
// that nothing reads gsm:// any more, and the carriersecrets_events_total
// fallback counter cannot establish that on its own: with every reference
// already bao://, the counter reads zero whether the credential paths were
// exercised or never reached at all. Verify supplies the missing half — it
// drives a real read through each of the credentials in the database, so a
// zero counter afterwards means "exercised and clean" rather than "not
// measured".
//
// Verify NEVER writes. It calls only Store.Get, never Put or Destroy, and
// never touches the DB beyond the initial FetchAll. It also never logs a
// secret value: failures are reported by table/column/id and scheme only.
func (b *Backfiller) Verify(ctx context.Context) (VerifyResult, error) {
	rows, err := b.Rows.FetchAll(ctx)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("carrier-secrets-backfill: fetch rows: %w", err)
	}

	res := VerifyResult{ByScheme: make(map[string]int)}
	log := b.logger()

	for _, row := range rows {
		res.Examined++
		scheme := referenceScheme(row.Ref)
		res.ByScheme[scheme]++

		// An empty column is not a configured credential; dialling a
		// backend for it would invent a read that production never does.
		if scheme == "empty" {
			continue
		}

		if _, getErr := b.Store.Get(ctx, row.Ref); getErr != nil {
			res.Failed++
			log.Error("carrier-secrets-backfill: reference did not resolve",
				slog.String("table", row.Table),
				slog.String("column", row.Column),
				slog.String("id", row.ID),
				slog.String("scheme", scheme),
				slog.String("err", getErr.Error()))
			continue
		}
		res.Resolved++
	}

	return res, nil
}
