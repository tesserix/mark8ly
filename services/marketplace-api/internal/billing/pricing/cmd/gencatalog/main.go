// Command gencatalog writes internal/billing/pricing/catalog_data.go, the
// generated half of the price catalog: the developedAmounts and pppAmounts
// tables catalog.go builds its PriceDescriptors from. Run it with -o pointing
// at the committed file:
//
//	cd services/marketplace-api && go run ./internal/billing/pricing/cmd/gencatalog \
//	  -o internal/billing/pricing/catalog_data.go
//
// -o exists rather than a stdout redirect because the destination is a file in
// the package being compiled: `> catalog_data.go` truncates it before `go run`
// builds, and the build then fails on the file it was about to replace.
//
// See pricing.RegenerateCatalogCommand for the single source of that command
// string (also quoted in the generated file's header and in
// gencatalog_test.go's failure message).
//
// # Sources
//
// -source=literal (the default) renders the file from the tables already
// compiled into the pricing package. It touches no network, so CI and an
// offline build behave exactly as they did before -source existed.
//
// -source=console renders it from the platform console's plan catalog, which
// owns these amounts, reading it through internal/billing/consolecatalog —
// the same client and the same CONSOLE_CATALOG_* configuration the service's
// parity monitor runs on. It needs credentials, so it is a deliberate manual
// run, never part of a build:
//
//	CONSOLE_CATALOG_URL=… CONSOLE_CATALOG_TOKEN_URL=… CONSOLE_CATALOG_SCOPE=… //	CONSOLE_CATALOG_CLIENT_ID=… CONSOLE_CATALOG_CLIENT_SECRET=… //	go run ./internal/billing/pricing/cmd/gencatalog -source=console | diff - internal/billing/pricing/catalog_data.go
//
// An empty diff there is evidence in its own right: the console and the
// fail-open fallback agree, established without the parity monitor.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// Sources this command can generate from. Literal is the default so nothing
// that runs without credentials — CI, an offline build, a regeneration after
// a rendering change — acquires a network dependency.
const (
	sourceLiteral = "literal"
	sourceConsole = "console"
)

func main() {
	out := flag.String("o", "", "write the generated file here; empty writes to stdout")
	source := flag.String("source", sourceLiteral,
		"where the amounts come from: \"literal\" (the pricing package's own tables) or \"console\" (the platform console, via CONSOLE_CATALOG_*)")
	flag.Parse()

	data, err := generate(*source)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gencatalog:", err) //nolint:logging-smoke // dev-only CLI: human-readable error to stderr before exit
		os.Exit(1)
	}

	if *out == "" {
		os.Stdout.WriteString(data)
		return
	}
	if err := os.WriteFile(*out, []byte(data), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gencatalog:", err) //nolint:logging-smoke // dev-only CLI: human-readable error to stderr before exit
		os.Exit(1)
	}
}

// generate renders the file from the named source. It returns an error rather
// than writing a partial file on any failure, so a console that is
// unreachable or answering short leaves the committed fallback untouched.
func generate(source string) (string, error) {
	switch source {
	case sourceLiteral:
		return pricing.GenerateCatalogData()
	case sourceConsole:
		cfg := consoleConfigFromEnv()
		if !cfg.Configured() {
			return "", fmt.Errorf("-source=console needs CONSOLE_CATALOG_URL, CONSOLE_CATALOG_TOKEN_URL, " +
				"CONSOLE_CATALOG_CLIENT_ID and CONSOLE_CATALOG_CLIENT_SECRET in the environment")
		}
		ctx, cancel := context.WithTimeout(context.Background(), consoleFetchTimeout)
		defer cancel()
		return generateFromConsole(ctx, newConsoleClient(cfg))
	default:
		return "", fmt.Errorf("unknown -source %q: want %q or %q", source, sourceLiteral, sourceConsole)
	}
}
