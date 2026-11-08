package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AltairaLabs/PromptKit/runtime/v2/hooks"
	"github.com/AltairaLabs/PromptKit/runtime/v2/hooks/execconfig"
)

// applyRuntimeHooks wires the pass-through runtime config (config.Runtime) into
// the engine. Arena wraps the runtime, so hooks declared under `runtime:` are
// turned into runtime hooks with the SAME builder the SDK uses
// (hooks.BuildExecHooks) — the two never drift.
//
// Currently only session-type hooks are threaded into the engine (they fire the
// SessionHook lifecycle around each run). Provider, tool and eval hooks declared
// in config are rejected with a clear error, because Arena does not consume
// them — failing loudly beats silently dropping them.
func (e *Engine) applyRuntimeHooks() error {
	if e.config == nil || e.config.Runtime == nil || len(e.config.Runtime.Hooks) == 0 {
		return nil
	}
	rc := e.config.Runtime
	if err := rejectEvalHooks(rc.Hooks); err != nil {
		return err
	}

	sandboxes, err := hooks.ResolveSandboxes(rc.Sandboxes)
	if err != nil {
		return fmt.Errorf("resolving sandboxes: %w", err)
	}
	provider, tool, session, err := hooks.BuildExecHooks(rc.Hooks, sandboxes)
	if err != nil {
		return err
	}
	if len(provider) > 0 || len(tool) > 0 {
		return fmt.Errorf("arena config currently supports only session-type runtime hooks; " +
			"provider/tool hooks are not yet wired into the per-turn pipeline")
	}
	if len(session) == 0 {
		return nil
	}

	opts := make([]hooks.Option, 0, len(session))
	for _, h := range session {
		opts = append(opts, hooks.WithSessionHook(h))
	}
	e.WithSessionHooks(hooks.NewRegistry(opts...))
	return nil
}

// rejectEvalHooks fails on any `hook: eval` binding. hooks.BuildExecHooks
// leaves eval hooks to the caller, and Arena does not wire them: its eval
// runner also runs every assertion and self-play objective check, so an eval
// hook would fire for far more than pack evals. Rather than drop the binding
// silently, say so. Names are sorted so the error is stable.
func rejectEvalHooks(bindings map[string]*execconfig.ExecHook) error {
	var names []string
	for name, b := range bindings {
		if b != nil && b.Hook == hooks.HookTypeEval {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return fmt.Errorf("arena config does not support eval-type runtime hooks (%s); "+
		"use runtime.evals to bind an eval type to a program instead", strings.Join(names, ", "))
}
