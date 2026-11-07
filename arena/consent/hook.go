// Package consent provides consent simulation for PromptArena scenarios.
// It implements a hooks.ToolHook that intercepts client-side tool execution
// and simulates consent grant, denial, or timeout based on scenario overrides.
package consent

import (
	"context"
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/v2/hooks"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

type contextKey int

// Consent override actions a scenario turn may name in consent_overrides.
const (
	ActionGrant   = "grant"
	ActionDeny    = "deny"
	ActionTimeout = "timeout"
)

// Actions lists every valid consent_overrides value.
var Actions = []string{ActionGrant, ActionDeny, ActionTimeout}

// consentStatusKey is the decision metadata key naming the simulated outcome.
const consentStatusKey = "consent_status"

// IsValidAction reports whether action is a consent_overrides value the hook
// understands.
func IsValidAction(action string) bool {
	return action == ActionGrant || action == ActionDeny || action == ActionTimeout
}

const (
	consentOverridesKey contextKey = iota
	toolRegistryKey
)

// SimulationHook implements hooks.ToolHook to simulate consent decisions
// for client-side tools during PromptArena scenario execution.
type SimulationHook struct{}

// NewSimulationHook creates a new consent simulation hook.
func NewSimulationHook() *SimulationHook {
	return &SimulationHook{}
}

// Name returns the hook name.
func (h *SimulationHook) Name() string {
	return "consent-simulation"
}

// BeforeExecution checks consent overrides and blocks denied/timed-out tools.
func (h *SimulationHook) BeforeExecution(ctx context.Context, req hooks.ToolRequest) hooks.Decision {
	overrides := OverridesFromContext(ctx)
	if overrides == nil {
		return hooks.Allow
	}

	action, ok := overrides[req.Name]
	if !ok {
		return hooks.Allow
	}

	switch action {
	case ActionGrant:
		return hooks.Decision{Allow: true, Metadata: map[string]any{
			consentStatusKey: "granted",
		}}

	case ActionDeny:
		// The SDK's RejectClientTool uses the same message, so a simulated
		// denial reads to the model exactly like a real one.
		reason := tools.ConsentDeclinedMessage(req.Name, h.lookupDeclineStrategy(ctx, req.Name))
		return hooks.DenyWithMetadata(reason, map[string]any{
			consentStatusKey: "denied",
		})

	case ActionTimeout:
		return hooks.DenyWithMetadata(
			fmt.Sprintf("Tool %s timed out waiting for user consent", req.Name),
			map[string]any{consentStatusKey: "timeout"},
		)

	default:
		// The loader rejects unknown values, so this is only reachable from a
		// caller that built the overrides itself. Fail closed: granting on a
		// typo such as "denied" let a tool run that the scenario meant to block.
		return hooks.DenyWithMetadata(
			fmt.Sprintf("Tool %s: unknown consent override %q (want %s, %s or %s)",
				req.Name, action, ActionGrant, ActionDeny, ActionTimeout),
			map[string]any{consentStatusKey: "invalid"},
		)
	}
}

// AfterExecution is a no-op — always allows.
func (h *SimulationHook) AfterExecution(_ context.Context, _ hooks.ToolRequest, _ hooks.ToolResponse) hooks.Decision {
	return hooks.Allow
}

// lookupDeclineStrategy retrieves the decline strategy from the tool descriptor.
func (h *SimulationHook) lookupDeclineStrategy(ctx context.Context, toolName string) string {
	registry := ToolRegistryFromContext(ctx)
	if registry == nil {
		return ""
	}

	descriptor := registry.Get(toolName)
	if descriptor == nil || descriptor.ClientConfig == nil || descriptor.ClientConfig.Consent == nil {
		return ""
	}

	return descriptor.ClientConfig.Consent.DeclineStrategy
}

// WithConsentOverrides stores consent overrides in the context.
func WithConsentOverrides(ctx context.Context, overrides map[string]string) context.Context {
	return context.WithValue(ctx, consentOverridesKey, overrides)
}

// OverridesFromContext retrieves consent overrides from the context.
func OverridesFromContext(ctx context.Context) map[string]string {
	v, _ := ctx.Value(consentOverridesKey).(map[string]string)
	return v
}

// WithToolRegistry stores a tool registry in the context.
func WithToolRegistry(ctx context.Context, registry *tools.Registry) context.Context {
	return context.WithValue(ctx, toolRegistryKey, registry)
}

// ToolRegistryFromContext retrieves a tool registry from the context.
func ToolRegistryFromContext(ctx context.Context) *tools.Registry {
	v, _ := ctx.Value(toolRegistryKey).(*tools.Registry)
	return v
}
