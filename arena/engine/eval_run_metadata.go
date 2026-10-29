package engine

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
)

// evalMetadataKey is the eval-context metadata key carrying the run's
// identity and sandboxes. Exec evals receive it on stdin as
// context.metadata.arena.
const evalMetadataKey = "arena"

// sandboxPortsField is the per-sandbox key listing its published ports.
const sandboxPortsField = "ports"

// evalRunInfo is the run identity handed to evals. It lives on the run
// context so evals deep inside the conversation executor can read it.
type evalRunInfo struct {
	RunID        string
	ScenarioID   string
	ProviderID   string
	Region       string
	TrialIndex   int
	Labels       map[string]string
	ArtifactsDir string
}

type evalRunInfoKey struct{}

func withEvalRunInfo(ctx context.Context, info *evalRunInfo) context.Context {
	return context.WithValue(ctx, evalRunInfoKey{}, info)
}

func evalRunInfoFromContext(ctx context.Context) *evalRunInfo {
	info, _ := ctx.Value(evalRunInfoKey{}).(*evalRunInfo)
	return info
}

// arenaEvalMetadata builds the JSON-friendly document exec evals receive as
// context.metadata.arena: the run's identity plus, per sandbox, its container
// ID and the host side of each published port. Nil when ctx carries neither.
func arenaEvalMetadata(ctx context.Context) map[string]any {
	info := evalRunInfoFromContext(ctx)
	containers := sandboxContainerIDsFromContext(ctx)
	ports := sandboxPortsFromContext(ctx)
	if info == nil && len(containers) == 0 && len(ports) == 0 {
		return nil
	}

	meta := map[string]any{}
	if info != nil {
		meta["run_id"] = info.RunID
		meta["scenario_id"] = info.ScenarioID
		meta["provider_id"] = info.ProviderID
		meta["region"] = info.Region
		meta["trial_index"] = info.TrialIndex
		meta["labels"] = info.Labels
		meta["artifacts_dir"] = info.ArtifactsDir
	}

	sandboxes := map[string]any{}
	for server, cid := range containers {
		sandboxes[server] = map[string]any{"container_id": cid, sandboxPortsField: portView(ports[server])}
	}
	for server, p := range ports {
		if _, seen := sandboxes[server]; !seen {
			sandboxes[server] = map[string]any{sandboxPortsField: portView(p)}
		}
	}
	meta["sandboxes"] = sandboxes
	return meta
}

// portView maps each container port to its host URL and host port number.
func portView(urls map[string]string) map[string]any {
	out := make(map[string]any, len(urls))
	for ctrPort, raw := range urls {
		entry := map[string]any{"url": raw}
		if u, err := url.Parse(raw); err == nil {
			if hostPort, err := strconv.Atoi(u.Port()); err == nil {
				entry["host_port"] = hostPort
			}
		}
		out[ctrPort] = entry
	}
	return out
}

// withArenaEvalMetadata adds the arena document to evalCtx's metadata. The
// metadata map can be shared across runs, so it is copied, never mutated.
func withArenaEvalMetadata(ctx context.Context, evalCtx *evals.EvalContext) *evals.EvalContext {
	meta := arenaEvalMetadata(ctx)
	if meta == nil || evalCtx == nil {
		return evalCtx
	}
	merged := make(map[string]any, len(evalCtx.Metadata)+1)
	for k, v := range evalCtx.Metadata {
		merged[k] = v
	}
	merged[evalMetadataKey] = meta
	evalCtx.Metadata = merged
	return evalCtx
}

// scoringContext returns the context to score a finished run with. When the
// run timed out, ctx is already canceled and every eval would fail on
// start; the detached copy keeps ctx's values (run info, sandboxes) but not
// its deadline.
func scoringContext(ctx context.Context) context.Context {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return context.WithoutCancel(ctx)
	}
	return ctx
}
