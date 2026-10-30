package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	pkgconfig "github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func sandboxRunContext() context.Context {
	ctx := withEvalRunInfo(context.Background(), &evalRunInfo{
		RunID: "run-1", ScenarioID: "build-service", ProviderID: "claude", Region: "default",
		TrialIndex: 2, Labels: map[string]string{"arm": "spec"}, ArtifactsDir: "/out/artifacts/run-1",
	})
	ctx = withSandboxContainerIDs(ctx, map[string]string{"sandbox": "cid-123"})
	return withSandboxPorts(ctx, map[string]map[string]string{
		"sandbox": {"5000": "http://localhost:54321"},
	})
}

func TestArenaEvalMetadata_NilWithoutRunOrSandbox(t *testing.T) {
	assert.Nil(t, arenaEvalMetadata(context.Background()))
}

func TestArenaEvalMetadata_RunAndSandboxes(t *testing.T) {
	meta := arenaEvalMetadata(sandboxRunContext())
	require.NotNil(t, meta)

	// The document reaches eval programs as JSON, so check it through JSON.
	raw, err := json.Marshal(meta)
	require.NoError(t, err)
	var doc struct {
		RunID        string            `json:"run_id"`
		ScenarioID   string            `json:"scenario_id"`
		TrialIndex   int               `json:"trial_index"`
		Labels       map[string]string `json:"labels"`
		ArtifactsDir string            `json:"artifacts_dir"`
		Sandboxes    map[string]struct {
			ContainerID string `json:"container_id"`
			Ports       map[string]struct {
				HostPort int    `json:"host_port"`
				URL      string `json:"url"`
			} `json:"ports"`
		} `json:"sandboxes"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	assert.Equal(t, "run-1", doc.RunID)
	assert.Equal(t, "build-service", doc.ScenarioID)
	assert.Equal(t, 2, doc.TrialIndex)
	assert.Equal(t, "spec", doc.Labels["arm"])
	assert.Equal(t, "/out/artifacts/run-1", doc.ArtifactsDir)
	sb := doc.Sandboxes["sandbox"]
	assert.Equal(t, "cid-123", sb.ContainerID)
	assert.Equal(t, 54321, sb.Ports["5000"].HostPort)
	assert.Equal(t, "http://localhost:54321", sb.Ports["5000"].URL)
}

func TestArenaEvalMetadata_PortsWithoutContainer(t *testing.T) {
	ctx := withSandboxPorts(context.Background(), map[string]map[string]string{
		"remote": {"8000": "not a url\x7f"},
	})
	meta := arenaEvalMetadata(ctx)
	sandboxes := meta["sandboxes"].(map[string]any)
	ports := sandboxes["remote"].(map[string]any)["ports"].(map[string]any)
	entry := ports["8000"].(map[string]any)
	assert.NotContains(t, entry, "host_port", "an unparseable URL has no host port")
	assert.NotContains(t, sandboxes["remote"], "container_id")
}

func TestWithArenaEvalMetadata_CopiesSharedMetadata(t *testing.T) {
	shared := map[string]any{"judge_targets": "x"}
	evalCtx := &evals.EvalContext{Metadata: shared}

	got := withArenaEvalMetadata(sandboxRunContext(), evalCtx)

	assert.Contains(t, got.Metadata, evalMetadataKey)
	assert.Equal(t, "x", got.Metadata["judge_targets"])
	assert.NotContains(t, shared, evalMetadataKey, "the shared map must not be mutated")

	plain := &evals.EvalContext{Metadata: shared}
	assert.Same(t, plain, withArenaEvalMetadata(context.Background(), plain))
	assert.Nil(t, withArenaEvalMetadata(sandboxRunContext(), nil))
}

func TestScoringContext(t *testing.T) {
	live := context.Background()
	assert.Equal(t, live, scoringContext(live))

	timedOut, cancel := context.WithDeadline(sandboxRunContext(), time.Now().Add(-time.Second))
	defer cancel()
	scoring := scoringContext(timedOut)
	assert.NoError(t, scoring.Err(), "a timed-out run is scored on a live context")
	assert.Equal(t, "run-1", evalRunInfoFromContext(scoring).RunID, "values survive the detach")

	cancelled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	assert.Error(t, scoringContext(cancelled).Err(), "a cancelled (not timed-out) run stays cancelled")
}

// captureHandler records the metadata a session eval was invoked with.
type captureHandler struct{ metadata map[string]any }

func (h *captureHandler) Type() string { return "capture" }

func (h *captureHandler) Eval(_ context.Context, evalCtx *evals.EvalContext, _ map[string]any) (*evals.EvalResult, error) {
	h.metadata = evalCtx.Metadata
	score := 1.0
	return &evals.EvalResult{Type: h.Type(), Score: &score}, nil
}

func TestRunSessionEvals_ReceivesArenaMetadata(t *testing.T) {
	handler := &captureHandler{}
	registry := evals.NewEvalTypeRegistry()
	registry.Register(handler)
	orch := NewEvalOrchestrator(registry, []evals.EvalDef{
		{ID: "probe", Type: "capture", Trigger: evals.TriggerOnSessionComplete},
	}, false, nil, "chat")

	orch.RunSessionEvals(sandboxRunContext(), []types.Message{types.NewAssistantMessage("done")}, "run-1")

	require.NotNil(t, handler.metadata)
	arena := handler.metadata[evalMetadataKey].(map[string]any)
	assert.Equal(t, "run-1", arena["run_id"])
	assert.Contains(t, arena["sandboxes"], "sandbox")
}

func TestRegisterExecEvals_RunsHostProgramWithArenaMetadata(t *testing.T) {
	dir := t.TempDir()
	stdinCopy := filepath.Join(dir, "stdin.json")
	script := filepath.Join(dir, "evals", "probe.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(script), 0o750))
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ncat > "+stdinCopy+"\n"+
		`echo '{"score": 0.5, "detail": "half", "data": {"functional_score": 0.5, "security_score": 1}}'`+"\n"), 0o700)) //nolint:gosec // test script must be executable

	cfg := &arenaconfig.Config{ConfigDir: dir, Runtime: &pkgconfig.RuntimeConfigSpec{
		Evals: map[string]*pkgconfig.ExecBinding{"host_probe": {Command: "evals/probe.sh", TimeoutMs: 10000}, "nil": nil},
	}}
	registry := evals.NewEvalTypeRegistry()
	registerExecEvals(registry, cfg)

	orch := NewEvalOrchestrator(registry, []evals.EvalDef{
		{ID: "probe", Type: "host_probe", Trigger: evals.TriggerOnSessionComplete},
	}, false, nil, "chat")
	results := orch.RunSessionEvals(sandboxRunContext(), []types.Message{types.NewAssistantMessage("done")}, "run-1")

	require.Len(t, results, 1)
	require.NotNil(t, results[0].Score)
	assert.InDelta(t, 0.5, *results[0].Score, 1e-9)
	assert.Equal(t, 1.0, results[0].Details["security_score"])

	raw, err := os.ReadFile(stdinCopy) //nolint:gosec // path is under t.TempDir
	require.NoError(t, err)
	var req struct {
		Context struct {
			Metadata struct {
				Arena struct {
					Sandboxes map[string]struct {
						ContainerID string `json:"container_id"`
					} `json:"sandboxes"`
				} `json:"arena"`
			} `json:"metadata"`
		} `json:"context"`
	}
	require.NoError(t, json.Unmarshal(raw, &req))
	assert.Equal(t, "cid-123", req.Context.Metadata.Arena.Sandboxes["sandbox"].ContainerID)
}

func TestRegisterExecEvals_NoRuntimeIsNoop(t *testing.T) {
	registry := evals.NewEvalTypeRegistry()
	registerExecEvals(registry, nil)
	registerExecEvals(registry, &arenaconfig.Config{})
	_, err := registry.Get("host_probe")
	assert.Error(t, err)
}

func TestResolveEvalCommand(t *testing.T) {
	assert.Equal(t, filepath.Join("/kit", "evals/score.py"), resolveEvalCommand("/kit", "evals/score.py"))
	assert.Equal(t, "python3", resolveEvalCommand("/kit", "python3"), "a bare name is left for PATH lookup")
	assert.Equal(t, "/abs/score.py", resolveEvalCommand("/kit", "/abs/score.py"))
	assert.Equal(t, "evals/score.py", resolveEvalCommand("", "evals/score.py"))
}
