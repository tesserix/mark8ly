package pricing

import "sync/atomic"

// Source supplies prices from somewhere other than the compiled catalog —
// in practice the Tesserix console (#304, #392).
//
// It exists so the catalog's DATA can move to the console while its API and
// every call site stay exactly as they are. Deleting this package was
// explicitly not proposed: the lookup helpers have callers whose signatures
// should survive, and replacing the data behind them is a far smaller and
// safer change than removing them.
//
// # Returning false is a first-class answer, not an error
//
// A Source that cannot answer — cold cache, failed refresh, a currency it
// does not carry — returns false, and the helper falls through to the
// compiled catalog. That fallthrough IS the baked-snapshot cold start and
// the fail-open behaviour BACKLOG §P requires; there is no second mechanism
// for it, and none is wanted. A fresh pod during a console outage must not
// itself be the outage.
//
// The fallthrough is per lookup rather than wholesale, so a source carrying
// most currencies does not have to be abandoned for the one it lacks.
type Source interface {
	PPPOption(plan Plan, period Period, currency string) (Amount, bool)
	DevelopedOptions(plan Plan, period Period) (map[string]Amount, bool)
}

// active holds the installed Source, or nil for "use the compiled catalog".
//
// atomic because it is installed at startup and refreshed by a background
// ticker while requests read it concurrently. A plain field here would be a
// data race on the price of a subscription.
var active atomic.Pointer[Source]

// UseSource installs s as the price source, or restores the compiled catalog
// when s is nil.
//
// Nil is the default and the shipped state: the console cutover is enabled
// by configuration, so it can be reverted by removing configuration rather
// than by deploying.
func UseSource(s Source) {
	if s == nil {
		active.Store(nil)
		return
	}
	active.Store(&s)
}

// sourceOrNil returns the installed Source, if any.
func sourceOrNil() Source {
	if p := active.Load(); p != nil {
		return *p
	}
	return nil
}
