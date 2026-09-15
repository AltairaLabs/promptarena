package engine

import "context"

// runIDKey is the context key carrying the identity of the run currently
// executing. Arena shares one tool registry across every concurrent run, so
// executors registered in it are shared objects: they must resolve per-run
// state at execute time rather than capturing it at registration. This key is
// how they find out which run is asking.
//
// A distinct unexported type keeps the key from colliding with any other
// package's context values.
type runIDKey struct{}

// withRunID returns a child context carrying runID. Stamped once per run, at
// the point the run context is assembled, so every turn, pipeline stage and
// tool call beneath it inherits the identity.
func withRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey{}, runID)
}

// runIDFromContext returns the run identity stamped by withRunID, or "" when
// the context did not come from a run (unit tests, interactive sessions).
// Callers that require isolation must treat "" as an error rather than as a
// shared default — an unscoped fallback is the bug this key exists to prevent.
func runIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(runIDKey{}).(string); ok {
		return v
	}
	return ""
}
