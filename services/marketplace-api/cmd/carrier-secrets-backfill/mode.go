package main

import "fmt"

// requireBaoPrimary refuses to run the backfill unless mode is exactly
// "bao". Running it under any other mode would not produce bao://
// references: under "gcpsm" every migrated row's Put writes back to GCP SM,
// "migrating" gsm:// to gsm:// — a pointless write against live payment
// credentials that still leaves the row unmigrated. Under "inline" there is
// no per-tenant reference to migrate at all. The only mode where this job's
// purpose (gsm:// -> bao://) is even meaningful is "bao".
func requireBaoPrimary(mode string) error {
	if mode != "bao" {
		return fmt.Errorf(
			"carrier-secrets-backfill: refusing to run — SHIPPING_SECRET_STORE=%q, must be %q (running under any other mode cannot produce bao:// references)",
			mode, "bao")
	}
	return nil
}
