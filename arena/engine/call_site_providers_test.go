package engine

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
)

// callSiteBinding binds keys to providers; wrong names a key bound to an
// inference provider.
type callSiteBinding struct {
	llm   map[string]providers.Provider
	wrong string
}

func (b callSiteBinding) LLM(key string) (providers.Provider, error) {
	if key == b.wrong {
		return nil, evals.ErrWrongKind
	}
	if p, ok := b.llm[key]; ok {
		return p, nil
	}
	return nil, evals.ErrUnboundKey
}

func (b callSiteBinding) Classifier(string) (any, error) { return nil, evals.ErrUnboundKey }

func callSiteEngine(t *testing.T, packJSON string, binding evals.ProviderBinding) *Engine {
	t.Helper()
	var pack prompt.Pack
	require.NoError(t, json.Unmarshal([]byte(packJSON), &pack))
	return &Engine{
		config:           &arenaconfig.Config{LoadedPack: &pack},
		evalOrchestrator: &EvalOrchestrator{providerBinding: binding},
	}
}

// A composition step whose key the pack does not declare, this config binds
// nothing to, or binds to the wrong kind fails the run before it starts, as
// sdk.Open does (promptarena#298).
func TestCheckCallSiteProviders(t *testing.T) {
	const pack = `{"id":"p","name":"p","version":"1",
		"requires":{"providers":["judge","unbound","wrongkind",{"key":"ranker","role":"inference"}]},
		"prompts":{"q":{"id":"q","name":"q","version":"1","system_template":"q"}},
		"compositions":{"c":{"version":1,"steps":[
			{"id":"%s","kind":"%s","prompt_task":"q","provider":"%s"}]}}}`
	textOnly := mock.NewProvider("judge-model", "m", false)
	tools := mock.NewToolProvider("tool-model", "m", false, nil)
	binding := callSiteBinding{llm: map[string]providers.Provider{"judge": textOnly, "agentic": tools}, wrong: "wrongkind"}

	cases := []struct {
		kind, key string
		want      string // "" passes
	}{
		{"prompt", "judge", ""},
		{"prompt", "undeclared", "does not declare"},
		{"prompt", "unbound", "binds nothing to"},
		{"prompt", "wrongkind", "inference provider"},
		{"prompt", "ranker", `role "inference"`},
		{"agent", "judge", "without tool support"},
	}
	for _, tc := range cases {
		t.Run(tc.kind+"/"+tc.key, func(t *testing.T) {
			e := callSiteEngine(t, sprintfPack(pack, tc.kind, tc.key), binding)
			err := e.checkCallSiteProviders()
			if tc.want == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Contains(t, err.Error(), `composition "c" step "s"`)
		})
	}

	assert.ErrorContains(t, callSiteEngine(t, sprintfPack(pack, "prompt", "judge"), nil).checkCallSiteProviders(),
		"wires no providers")
	assert.NoError(t, callSiteEngine(t, `{"id":"p","name":"p","version":"1","prompts":{}}`, nil).checkCallSiteProviders(),
		"a pack naming no key needs no binding")
}

func sprintfPack(format, kind, key string) string {
	return fmt.Sprintf(format, "s", kind, key)
}
