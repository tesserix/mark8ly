// Command carrier-secrets-backfill migrates existing gsm:// (GCP Secret
// Manager) credential references to bao:// (OpenBao) references. This is
// phase 3 of the OpenBao migration: the admin engine's lazy rewrap
// (carriersecrets.ChainStore.MaybeRewrap) only fires on save, and is
// refused outright on the storefront engine by design (no OpenBao write
// grant there — least privilege). Rows that are never re-saved never
// migrate on their own; this job is what completes them.
//
// THESE ARE LIVE PAYMENT CREDENTIALS IN PRODUCTION. The safety property
// this job relies on is Backfiller.migrateRow's read-back verification: a
// row's DB reference is only ever updated after the NEW reference is
// proven, by an actual read, to resolve to the exact same plaintext the
// OLD reference resolved to. The old GCP SM secret is never deleted (see
// Backfiller.migrateRow's doc comment) — a bad migration is recoverable by
// hand precisely because that old copy is still there and the DB reference
// pointing at it is only ever swapped after the new one is proven equal.
package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
)

// Result summarizes one backfill run.
type Result struct {
	Examined int
	Skipped  int
	Migrated int
	Failed   int
}

// Backfiller drives the gsm:// -> bao:// migration over every row RowStore
// returns.
type Backfiller struct {
	Rows   RowStore
	Store  carriersecrets.Store
	DryRun bool
	Logger *slog.Logger
}

func (b *Backfiller) logger() *slog.Logger {
	if b.Logger != nil {
		return b.Logger
	}
	return slog.Default()
}

// Run walks every row RowStore.FetchAll returns, exactly once, in fetch
// order. Per row:
//
//  1. Skip unless IsGSMRef(row.Ref) — bao://, inline (noop:/aes:), and
//     empty values are left untouched. This is what makes the job
//     idempotent: a row already migrated (or never on GCP SM at all)
//     produces zero writes on a repeat run.
//  2. In dry-run mode, log what WOULD migrate and count it under
//     Migrated without touching the Store or the DB.
//  3. Otherwise call migrateRow, which performs the actual
//     get/put/verify/update sequence.
func (b *Backfiller) Run(ctx context.Context) (Result, error) {
	rows, err := b.Rows.FetchAll(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("carrier-secrets-backfill: fetch rows: %w", err)
	}

	var res Result
	log := b.logger()
	for _, row := range rows {
		res.Examined++

		if !carriersecrets.IsGSMRef(row.Ref) {
			res.Skipped++
			continue
		}

		if b.DryRun {
			newRef := carriersecrets.FormatBaoReference(row.Scope)
			log.Info("carrier-secrets-backfill: dry-run would migrate",
				"table", row.Table, "column", row.Column,
				"tenant_id", row.Scope.TenantID, "domain", row.Scope.Domain,
				"provider", row.Scope.Provider, "field", row.Scope.Field,
				"old_ref", row.Ref, "new_ref", newRef)
			res.Migrated++
			continue
		}

		if err := b.migrateRow(ctx, row); err != nil {
			res.Failed++
			log.Error("carrier-secrets-backfill: migration failed — DB reference left unchanged",
				"table", row.Table, "column", row.Column,
				"tenant_id", row.Scope.TenantID, "domain", row.Scope.Domain,
				"provider", row.Scope.Provider, "field", row.Scope.Field,
				"old_ref_len", len(row.Ref), "err", err)
			continue
		}
		res.Migrated++
	}
	return res, nil
}

// migrateRow performs the get -> put -> verify -> update sequence for one
// row, in that exact order. It updates the DB reference IF AND ONLY IF the
// read-back through the newly written reference reproduces, byte for byte,
// the plaintext read through the old one.
//
// This is the safety property that makes the whole operation
// reversible-by-construction: the old GCP SM secret is never deleted by
// this job (a later phase's decision, not this one's — see package doc),
// so as long as the DB reference isn't swapped until the new copy is
// PROVEN correct, a migration that fails at any step leaves the row
// exactly as readable as it was before this job ran. Never reorder or skip
// the verification read — that is what turns "we wrote something to
// OpenBao" into "we confirmed OpenBao actually has the right value" before
// the one thing that can't be undone cheaply (the DB write) happens.
//
// Never logs plaintext — only references, lengths, and outcomes.
func (b *Backfiller) migrateRow(ctx context.Context, row Row) error {
	plaintext, err := b.Store.Get(ctx, row.Ref)
	if err != nil {
		return fmt.Errorf("read old reference: %w", err)
	}

	newRef, err := b.Store.Put(ctx, row.Scope, plaintext)
	if err != nil {
		return fmt.Errorf("write new reference: %w", err)
	}

	verify, err := b.Store.Get(ctx, newRef)
	if err != nil {
		return fmt.Errorf("read back new reference for verification: %w", err)
	}
	if verify != plaintext {
		return fmt.Errorf("read-back verification failed: new reference %q does not reproduce the original value — DB reference left unchanged", newRef)
	}

	if err := b.Rows.UpdateReference(ctx, row, newRef); err != nil {
		return fmt.Errorf("update db reference: %w", err)
	}
	return nil
}
