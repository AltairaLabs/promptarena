// Package binding answers a pack's logical provider names with the concrete
// providers this Arena run wired up.
//
// A pack names the providers a check needs — `grader`, `screener`, whatever the
// author chose — and the host supplies one per name. The host stays free to
// change what sits behind a name, which is the whole reason a check names a
// logical key instead of a model. Arena is one such host; the SDK is another.
//
// Arena resolves a name against two things it already builds: the provider
// registry (every `providers:` entry the run initialised) and the inference
// registry (the `role: inference` subset, which classifier-backed checks such
// as topic_policy call). A `judges:` entry is an alias, so a check can name the
// judge the config declares rather than the provider id behind it.
package binding

import (
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/inference"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
)

// Binding resolves logical provider names for an Arena run.
type Binding struct {
	providers *providers.Registry
	inference *inference.Registry
	// judgeAliases maps a `judges:` name to the provider id it points at, so a
	// check naming the judge resolves to the same provider the eval path uses.
	judgeAliases map[string]string
}

var _ evals.ProviderBinding = (*Binding)(nil)

// New returns the binding for a run, or nil when nothing was wired that a pack
// could name. A nil binding is meaningful: the runtime reports "no binding
// configured", which is a different fix from "you named something unbound".
func New(
	providerRegistry *providers.Registry,
	inferenceRegistry *inference.Registry,
	judgeAliases map[string]string,
) evals.ProviderBinding {
	if providerRegistry == nil && inferenceRegistry == nil {
		return nil
	}
	return &Binding{
		providers:    providerRegistry,
		inference:    inferenceRegistry,
		judgeAliases: judgeAliases,
	}
}

// LLM returns the completion provider bound to key — what a judge-backed check
// needs.
//
// Binding the wrong KIND of thing is reported as exactly that. A run that gave
// a judge-backed check the name of an inference provider has made a wiring
// mistake, and "not found" would send them hunting for a missing provider
// instead of looking at the one they supplied.
func (b *Binding) LLM(key string) (providers.Provider, error) {
	if b == nil {
		return nil, evals.ErrNoBinding
	}
	if b.providers != nil {
		if p, ok := b.providers.Get(b.resolveAlias(key)); ok {
			return p, nil
		}
	}
	if _, ok := b.lookupInference(key); ok {
		return nil, fmt.Errorf(
			"%w: it is bound to an inference provider, and this check needs one that runs completions",
			evals.ErrWrongKind)
	}
	return nil, evals.ErrUnboundKey
}

// Classifier returns the inference provider bound to key — what a
// classifier-backed check needs. The return type is any to keep the runtime's
// v2 signature; the caller asserts inference.Provider.
func (b *Binding) Classifier(key string) (any, error) {
	if b == nil {
		return nil, evals.ErrNoBinding
	}
	if p, ok := b.lookupInference(key); ok {
		return p, nil
	}
	if b.providers != nil {
		if _, isLLM := b.providers.Get(b.resolveAlias(key)); isLLM {
			return nil, fmt.Errorf(
				"%w: it is bound to an LLM provider, and this check needs an inference provider "+
					"(a providers: entry with role: inference)",
				evals.ErrWrongKind)
		}
	}
	return nil, evals.ErrUnboundKey
}

// resolveAlias maps a `judges:` name onto the provider id it declares. A name
// that is not a judge is returned unchanged, so a check may name either.
func (b *Binding) resolveAlias(key string) string {
	if id, ok := b.judgeAliases[key]; ok {
		return id
	}
	return key
}

// lookupInference finds key in the inference registry by exact id. An empty
// key never resolves: to the registry it means "the default", and a check that
// named no provider reaches the default through the registry on its context,
// not through the binding, which only answers names a pack declared.
func (b *Binding) lookupInference(key string) (inference.Provider, bool) {
	if b.inference == nil || key == "" {
		return nil, false
	}
	p, err := b.inference.Get(key)
	return p, err == nil
}
