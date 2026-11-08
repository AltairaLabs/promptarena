package engine

import (
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

func sessionHookConfig() *arenaconfig.Config {
	return &arenaconfig.Config{Runtime: &config.RuntimeConfigSpec{
		Hooks: map[string]*config.ExecHook{
			"capture": {Hook: "session", ExecBinding: config.ExecBinding{Command: "echo"}},
		},
	}}
}

func TestApplyRuntimeHooks_NilRuntimeIsNoop(t *testing.T) {
	e := &Engine{config: &arenaconfig.Config{}}
	if err := e.applyRuntimeHooks(); err != nil {
		t.Fatalf("nil runtime should be a no-op, got %v", err)
	}
	if e.sessionHooks != nil {
		t.Fatal("no hooks should be registered for an empty runtime")
	}
}

func TestApplyRuntimeHooks_RegistersSessionHook(t *testing.T) {
	e := &Engine{config: sessionHookConfig()}
	if err := e.applyRuntimeHooks(); err != nil {
		t.Fatalf("applyRuntimeHooks: %v", err)
	}
	if e.sessionHooks == nil {
		t.Fatal("expected a session hook registry to be wired")
	}
}

func TestApplyRuntimeHooks_RejectsProviderAndToolHooks(t *testing.T) {
	for _, hookType := range []string{"provider", "tool"} {
		e := &Engine{config: &arenaconfig.Config{Runtime: &config.RuntimeConfigSpec{
			Hooks: map[string]*config.ExecHook{
				"x": {Hook: hookType, ExecBinding: config.ExecBinding{Command: "echo"}},
			},
		}}}
		if err := e.applyRuntimeHooks(); err == nil {
			t.Fatalf("%s hook should be rejected (not yet wired into the pipeline)", hookType)
		}
	}
}

func TestApplyRuntimeHooks_RejectsEvalHooks(t *testing.T) {
	e := &Engine{config: &arenaconfig.Config{Runtime: &config.RuntimeConfigSpec{
		Hooks: map[string]*config.ExecHook{
			"score-b":  {Hook: "eval", ExecBinding: config.ExecBinding{Command: "echo"}},
			"score-a":  {Hook: "eval", ExecBinding: config.ExecBinding{Command: "echo"}},
			"lifetime": {Hook: "session", ExecBinding: config.ExecBinding{Command: "echo"}},
		},
	}}}
	err := e.applyRuntimeHooks()
	if err == nil {
		t.Fatal("eval hook should be rejected, not silently dropped")
	}
	for _, want := range []string{"eval-type runtime hooks (score-a, score-b)", "runtime.evals"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should contain %q", err, want)
		}
	}
	if e.sessionHooks != nil {
		t.Fatal("no hooks should be wired when the config is rejected")
	}
}

func TestApplyRuntimeHooks_UndeclaredSandboxErrors(t *testing.T) {
	e := &Engine{config: &arenaconfig.Config{Runtime: &config.RuntimeConfigSpec{
		Hooks: map[string]*config.ExecHook{
			"x": {Hook: "session", ExecBinding: config.ExecBinding{Command: "echo", Sandbox: "ghost"}},
		},
	}}}
	if err := e.applyRuntimeHooks(); err == nil {
		t.Fatal("expected error for hook referencing an undeclared sandbox")
	}
}
