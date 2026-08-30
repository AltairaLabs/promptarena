package engine

import (
	"testing"
)

// TestInteractiveSession_SystemPrompt_NilSafe covers the paths where no prompt
// can be resolved. The console calls this when a session opens, before any turn
// has run, so it must degrade to "" rather than panicking on a half-built
// engine — an empty string simply means no system turn is shown early, and the
// real one still arrives with the first completed turn.
func TestInteractiveSession_SystemPrompt_NilSafe(t *testing.T) {
	var nilSession *InteractiveSession
	if got := nilSession.SystemPrompt(); got != "" {
		t.Errorf("nil session = %q, want empty", got)
	}

	if got := (&InteractiveSession{}).SystemPrompt(); got != "" {
		t.Errorf("session with no engine = %q, want empty", got)
	}

	if got := (&InteractiveSession{engine: &Engine{}}).SystemPrompt(); got != "" {
		t.Errorf("engine with no prompt registry = %q, want empty", got)
	}
}
