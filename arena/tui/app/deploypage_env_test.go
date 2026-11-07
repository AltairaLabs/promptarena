package app

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/deploy/flow"
	"github.com/AltairaLabs/promptarena/v2/arena/tui/views"
	"github.com/AltairaLabs/promptarena/v2/deploy"
)

// TestPreflight_EnvKeyCyclesTargetEnvironment is the regression for #410:
// the hub's Deploy page always targeted "default", so a non-default
// environment (and its type-the-name confirmation) was unreachable.
func TestPreflight_EnvKeyCyclesTargetEnvironment(t *testing.T) {
	p := &DeployPage{
		state: deployStatePreflight,
		pf: &flow.Preflight{
			Provider:     "omnia",
			Env:          flow.DefaultEnv,
			Environments: []string{flow.DefaultEnv, "production", "staging"},
		},
	}
	np, cmd := p.handlePreflightKey(keyRunes("e"))
	dp := np.(*DeployPage)
	if dp.opts.Env != "production" {
		t.Fatalf("opts.Env = %q, want production", dp.opts.Env)
	}
	if cmd == nil {
		t.Fatal("switching environment must re-run preflight")
	}
	if dp.pf != nil {
		t.Error("the old environment's preflight snapshot must be dropped")
	}
}

func TestPreflight_EnvKeyIgnoredWithOneEnvironment(t *testing.T) {
	p := &DeployPage{
		state: deployStatePreflight,
		pf:    &flow.Preflight{Env: flow.DefaultEnv, Environments: []string{flow.DefaultEnv}},
	}
	_, cmd := p.handlePreflightKey(keyRunes("e"))
	if cmd != nil || p.opts.Env != "" {
		t.Fatalf("with one environment 'e' must do nothing (env=%q)", p.opts.Env)
	}
}

func TestPreflightKeyBindings_ShowsEnvKeyOnlyWithChoices(t *testing.T) {
	has := func(kb []views.KeyBinding) bool {
		for _, b := range kb {
			if b.Keys == "e" {
				return true
			}
		}
		return false
	}
	if has(preflightKeyBindings(&flow.Preflight{Environments: []string{flow.DefaultEnv}})) {
		t.Error("no 'e' hint with a single environment")
	}
	if !has(preflightKeyBindings(&flow.Preflight{Environments: []string{flow.DefaultEnv, "prod"}})) {
		t.Error("'e' hint expected with several environments")
	}
}

func TestNextEnv(t *testing.T) {
	envs := []string{"default", "production", "staging"}
	cases := map[string]string{"default": "production", "staging": "default", "unlisted": "default"}
	for cur, want := range cases {
		if got := nextEnv(envs, cur); got != want {
			t.Errorf("nextEnv(%q) = %q, want %q", cur, got, want)
		}
	}
}

// TestRunPlan_HoldsTheEnvironmentLock is #407 item 6 on the TUI side:
// planning rewrites state, so it must not run while another command holds
// the environment's lock.
func TestRunPlan_HoldsTheEnvironmentLock(t *testing.T) {
	dir := t.TempDir()
	release, err := flow.LockEnv(dir, "staging")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	fp := &fakeDeployProvider{plan: planWithChanges()}
	dc := &arenaconfig.DeployConfig{Provider: "fake"}
	opts := flow.Options{ProjectDir: dir, Env: "staging"}
	sess := flow.NewSession(opts, &arenaconfig.Config{Deploy: dc}, dc, fp,
		deploy.NewEnvStateStore(dir, "staging"), []byte(`{}`), `{}`, func() error { return nil })

	sink := &loginMsgSink{}
	runPlan(context.Background(), opts, sess, sink.send)

	msgs := sink.snapshot()
	if len(msgs) != 1 {
		t.Fatalf("msgs = %v, want one deployErrMsg", msgs)
	}
	em, ok := msgs[0].(deployErrMsg)
	if !ok || !strings.Contains(em.err.Error(), "held by another process") {
		t.Fatalf("msg = %#v, want lock contention", msgs[0])
	}
}

func TestAdapterLogWriter_LogsCompleteLines(t *testing.T) {
	var got []string
	w := &adapterLogWriter{log: func(l string) { got = append(got, l) }}
	_, _ = w.Write([]byte("first\r\nsec"))
	_, _ = w.Write([]byte("ond\n\n   \nthird"))
	if want := []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("logged %v, want %v (partial line held back, blanks dropped)", got, want)
	}
	// The default writer logs through the runtime logger; it must not panic.
	if _, err := newAdapterLogWriter().Write([]byte("adapter line\n")); err != nil {
		t.Fatal(err)
	}
}
