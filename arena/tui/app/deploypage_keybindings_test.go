package app

import (
	"errors"
	"testing"

	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
	"github.com/AltairaLabs/promptarena/arena/tui/viewmodels"
	"github.com/AltairaLabs/promptarena/arena/tui/views"
)

// hasKey reports whether kb contains a binding whose Keys field is exactly
// key. Used throughout this file to assert a footer key hint is present (a
// working key SHOULD be shown) or absent (a key that does nothing MUST NOT be
// shown) for a given DeployPage state, mirroring the guards the corresponding
// handleXKey method actually applies.
func hasKey(kb []views.KeyBinding, key string) bool {
	for _, b := range kb {
		if b.Keys == key {
			return true
		}
	}
	return false
}

// TestKeyBindings_ChromeDelegatesToKeyBindings verifies p.chrome() (consumed
// by RenderWithChrome to draw the footer) always reflects p.keyBindings() —
// the single source of truth every other test in this file exercises
// directly.
func TestKeyBindings_ChromeDelegatesToKeyBindings(t *testing.T) {
	p := &DeployPage{state: deployStateLogin, pf: &flow.Preflight{Provider: "omnia"}}
	chrome := p.chrome()
	want := p.keyBindings()
	if len(chrome.KeyBindings) != len(want) {
		t.Fatalf("chrome().KeyBindings = %+v, want %+v", chrome.KeyBindings, want)
	}
	for i := range want {
		if chrome.KeyBindings[i] != want[i] {
			t.Fatalf("chrome().KeyBindings[%d] = %+v, want %+v", i, chrome.KeyBindings[i], want[i])
		}
	}
}

// TestKeyBindings_Preflight_StillProbing verifies that while the preflight
// probe hasn't completed yet (p.pf == nil), handlePreflightKey treats every
// key as a no-op, so the footer must show nothing but esc.
func TestKeyBindings_Preflight_StillProbing(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight}
	kb := p.keyBindings()
	for _, key := range []string{"l", "p", "r"} {
		if hasKey(kb, key) {
			t.Fatalf("expected no [%s] hint while preflight is still probing, got %+v", key, kb)
		}
	}
	if !hasKey(kb, "esc") {
		t.Fatalf("expected esc always present, got %+v", kb)
	}
}

// TestKeyBindings_Preflight_NeedsLogin verifies the footer offers [l] login
// (adapter supports login, not yet authenticated — so Ready() is false and
// [p] plan must not appear) and always offers [r] retry.
func TestKeyBindings_Preflight_NeedsLogin(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", AdapterFound: true, SupportsLogin: true, Authenticated: false,
	}}
	kb := p.keyBindings()
	if !hasKey(kb, "l") {
		t.Fatalf("expected [l] login hint, got %+v", kb)
	}
	if hasKey(kb, "p") {
		t.Fatalf("expected no [p] plan hint before Ready(), got %+v", kb)
	}
	if !hasKey(kb, "r") {
		t.Fatalf("expected [r] retry hint, got %+v", kb)
	}
}

// TestKeyBindings_Preflight_Ready verifies the footer offers [p] plan once
// pf.Ready() is true, and — since Ready() requires Authenticated — never
// offers [l] login at the same time (there is nothing left to log in for).
func TestKeyBindings_Preflight_Ready(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", AdapterFound: true, SupportsLogin: true, Authenticated: true,
	}}
	kb := p.keyBindings()
	if !hasKey(kb, "p") {
		t.Fatalf("expected [p] plan hint once Ready(), got %+v", kb)
	}
	if hasKey(kb, "l") {
		t.Fatalf("expected no [l] login hint once already authenticated, got %+v", kb)
	}
	if !hasKey(kb, "r") {
		t.Fatalf("expected [r] retry hint, got %+v", kb)
	}
}

// TestKeyBindings_Preflight_LoginUnsupported verifies the footer never offers
// [l] login when the adapter doesn't support login at all, regardless of
// auth state.
func TestKeyBindings_Preflight_LoginUnsupported(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", AdapterFound: true, SupportsLogin: false, Authenticated: false,
	}}
	kb := p.keyBindings()
	if hasKey(kb, "l") {
		t.Fatalf("expected no [l] login hint when adapter doesn't support login, got %+v", kb)
	}
}

// TestKeyBindings_Login verifies the login screen offers exactly [c] cancel
// and esc.
func TestKeyBindings_Login(t *testing.T) {
	p := &DeployPage{state: deployStateLogin, pf: &flow.Preflight{Provider: "omnia"}}
	kb := p.keyBindings()
	if !hasKey(kb, "c") {
		t.Fatalf("expected [c] cancel hint, got %+v", kb)
	}
	if !hasKey(kb, "esc") {
		t.Fatalf("expected esc hint, got %+v", kb)
	}
	if len(kb) != 2 {
		t.Fatalf("expected exactly 2 bindings on the login screen, got %+v", kb)
	}
}

// TestKeyBindings_Planning verifies the planning (spinner) screen has no
// handler of its own — handleKey's switch falls to the default no-op case for
// deployStatePlanning — so the footer must show only esc.
func TestKeyBindings_Planning(t *testing.T) {
	p := &DeployPage{state: deployStatePlanning, pf: &flow.Preflight{Provider: "omnia"}}
	kb := p.keyBindings()
	if len(kb) != 1 || kb[0].Keys != "esc" {
		t.Fatalf("expected only esc on the planning screen, got %+v", kb)
	}
}

// TestKeyBindings_Plan_WithChanges verifies the plan screen offers [a] apply
// once the plan has real changes, plus [space] toggle and esc.
func TestKeyBindings_Plan_WithChanges(t *testing.T) {
	p := &DeployPage{state: deployStatePlan, planDiff: viewmodels.BuildPlanDiff(planWithChanges())}
	kb := p.keyBindings()
	if !hasKey(kb, "a") {
		t.Fatalf("expected [a] apply hint when the plan has changes, got %+v", kb)
	}
	if !hasKey(kb, "space") {
		t.Fatalf("expected [space] toggle hint, got %+v", kb)
	}
	if !hasKey(kb, "esc") {
		t.Fatalf("expected esc hint, got %+v", kb)
	}
}

// TestKeyBindings_Plan_NoChanges verifies the plan screen never offers [a]
// apply for an all-no-change plan (handlePlanKey's 'a' is gated on
// planHasChanges, and there is nothing for the operator to apply).
func TestKeyBindings_Plan_NoChanges(t *testing.T) {
	p := &DeployPage{state: deployStatePlan, planDiff: viewmodels.BuildPlanDiff(planAllNoChange())}
	kb := p.keyBindings()
	if hasKey(kb, "a") {
		t.Fatalf("expected no [a] apply hint for an all-no-change plan, got %+v", kb)
	}
	if !hasKey(kb, "space") {
		t.Fatalf("expected [space] toggle hint regardless of changes, got %+v", kb)
	}
}

// TestKeyBindings_Confirm_DefaultEnv verifies the quiet [y/N] confirm screen
// offers [y] apply and [n] cancel — handleConfirmKey advances to applying
// only on 'y' and backs out to Plan on every other key (including 'n'), so
// advertising 'n' as the canonical negative response is accurate and the
// working key must be shown.
func TestKeyBindings_Confirm_DefaultEnv(t *testing.T) {
	p := &DeployPage{state: deployStateConfirm, pf: &flow.Preflight{Env: flow.DefaultEnv}}
	kb := p.keyBindings()
	if !hasKey(kb, "y") {
		t.Fatalf("expected [y] hint on the default-env confirm screen, got %+v", kb)
	}
	if !hasKey(kb, "n") {
		t.Fatalf("expected [n] cancel hint on the default-env confirm screen, got %+v", kb)
	}
	if !hasKey(kb, "esc") {
		t.Fatalf("expected esc hint, got %+v", kb)
	}
}

// TestKeyBindings_Confirm_TypedEnv verifies the type-to-confirm screen (a
// non-default env) offers [enter] confirm and esc, and does not carry the
// default-env's [y]/[n] hints (which don't apply to this mode).
func TestKeyBindings_Confirm_TypedEnv(t *testing.T) {
	p := &DeployPage{state: deployStateConfirm, pf: &flow.Preflight{Env: "production"}}
	kb := p.keyBindings()
	if !hasKey(kb, keyEnter) {
		t.Fatalf("expected [enter] hint on the typed-env confirm screen, got %+v", kb)
	}
	if hasKey(kb, "y") || hasKey(kb, "n") {
		t.Fatalf("expected no [y]/[n] hints on the typed-env confirm screen, got %+v", kb)
	}
}

// TestKeyBindings_Applying verifies the applying (spinner) screen has no
// handler of its own, mirroring TestKeyBindings_Planning: only esc.
func TestKeyBindings_Applying(t *testing.T) {
	p := &DeployPage{state: deployStateApplying}
	kb := p.keyBindings()
	if len(kb) != 1 || kb[0].Keys != "esc" {
		t.Fatalf("expected only esc on the applying screen, got %+v", kb)
	}
}

// TestKeyBindings_ApplyResult verifies the apply-result screen offers [s]
// status and esc.
func TestKeyBindings_ApplyResult(t *testing.T) {
	p := &DeployPage{state: deployStateApplyResult}
	kb := p.keyBindings()
	if !hasKey(kb, "s") {
		t.Fatalf("expected [s] status hint, got %+v", kb)
	}
	if !hasKey(kb, "esc") {
		t.Fatalf("expected esc hint, got %+v", kb)
	}
}

// TestKeyBindings_Status verifies the status screen offers both [q] and esc
// to back out (handleStatusKey handles 'q' directly since, unlike esc, it has
// no global handling once the wizard is a non-root page).
func TestKeyBindings_Status(t *testing.T) {
	p := &DeployPage{state: deployStateStatus}
	kb := p.keyBindings()
	if !hasKey(kb, "q") {
		t.Fatalf("expected [q] hint, got %+v", kb)
	}
	if !hasKey(kb, "esc") {
		t.Fatalf("expected esc hint, got %+v", kb)
	}
}

// TestKeyBindings_Error_AuthFailure_LoginSupported verifies the error screen
// offers [l] log in for an auth-failure error when the adapter supports
// login, matching handleErrorKey's guard exactly.
func TestKeyBindings_Error_AuthFailure_LoginSupported(t *testing.T) {
	err := errors.New("authentication failed: invalid credentials")
	p := &DeployPage{state: deployStateError, err: err, pf: &flow.Preflight{Provider: "omnia", SupportsLogin: true}}
	kb := p.keyBindings()
	if !hasKey(kb, "l") {
		t.Fatalf("expected [l] log in hint, got %+v", kb)
	}
}

// TestKeyBindings_Error_AuthFailure_LoginUnsupported verifies the CRITICAL
// invariant this task exists to enforce: handleErrorKey's 'l' case only fires
// when classifyDeployErr is errorKindAuth AND p.pf != nil && p.pf.SupportsLogin
// — so the footer must not advertise [l] when the adapter doesn't support
// login, even though the error itself classifies as an auth failure. Before
// this task's fix, keyBindings() checked only classifyDeployErr, not
// SupportsLogin, so it advertised a dead [l] key here.
func TestKeyBindings_Error_AuthFailure_LoginUnsupported(t *testing.T) {
	err := errors.New("authentication failed: invalid credentials")
	p := &DeployPage{state: deployStateError, err: err, pf: &flow.Preflight{Provider: "omnia", SupportsLogin: false}}
	kb := p.keyBindings()
	if hasKey(kb, "l") {
		t.Fatalf("expected no [l] log in hint when the adapter doesn't support login, got %+v", kb)
	}
	// Cross-check against the handler: pressing 'l' really must be a no-op.
	np, cmd := p.handleErrorKey(keyRunes("l"))
	if cmd != nil || np.(*DeployPage).state != deployStateError {
		t.Fatal("'l' must be a no-op when the adapter doesn't support login")
	}
}

// TestKeyBindings_Error_AuthFailure_NilPreflight verifies the same invariant
// as TestKeyBindings_Error_AuthFailure_LoginUnsupported for the case where
// p.pf is nil outright (e.g. an auth failure surfacing before any preflight
// probe ever completed) — handleErrorKey's guard requires p.pf != nil, so the
// footer must not advertise [l] here either.
func TestKeyBindings_Error_AuthFailure_NilPreflight(t *testing.T) {
	err := errors.New("authentication failed: invalid credentials")
	p := &DeployPage{state: deployStateError, err: err}
	kb := p.keyBindings()
	if hasKey(kb, "l") {
		t.Fatalf("expected no [l] log in hint when p.pf is nil, got %+v", kb)
	}
	np, cmd := p.handleErrorKey(keyRunes("l"))
	if cmd != nil || np.(*DeployPage).state != deployStateError {
		t.Fatal("'l' must be a no-op when p.pf is nil")
	}
}

// TestKeyBindings_Error_LockContention verifies the error screen offers [r]
// retry for a lock-contention error.
func TestKeyBindings_Error_LockContention(t *testing.T) {
	err := errors.New("deploy lock is held by another process; wait for the other deploy to finish or remove the lock file")
	p := &DeployPage{state: deployStateError, err: err}
	kb := p.keyBindings()
	if !hasKey(kb, "r") {
		t.Fatalf("expected [r] retry hint, got %+v", kb)
	}
}

// TestKeyBindings_Error_AdapterMissing verifies the error screen offers no
// recovery-key hints for an adapter-missing error (the install command is
// shown in the body, not the footer, and there is no key that does anything
// beyond the always-present esc).
func TestKeyBindings_Error_AdapterMissing(t *testing.T) {
	err := errors.New("adapter not found for provider \"omnia\": adapter binary not found\nInstall it with: promptarena deploy adapter install omnia")
	p := &DeployPage{state: deployStateError, err: err, pf: &flow.Preflight{Provider: "omnia"}}
	kb := p.keyBindings()
	if len(kb) != 1 || kb[0].Keys != "esc" {
		t.Fatalf("expected only esc for an adapter-missing error, got %+v", kb)
	}
}

// TestKeyBindings_Error_Generic verifies the error screen offers no
// recovery-key hints for a generic (unclassified) error.
func TestKeyBindings_Error_Generic(t *testing.T) {
	err := errors.New("plan failed: adapter exited unexpectedly")
	p := &DeployPage{state: deployStateError, err: err}
	kb := p.keyBindings()
	if len(kb) != 1 || kb[0].Keys != "esc" {
		t.Fatalf("expected only esc for a generic error, got %+v", kb)
	}
}
