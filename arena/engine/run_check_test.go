package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"

	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
)

func TestRunCheck(t *testing.T) {
	e := &Engine{evalOrchestrator: NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")}
	msgs := []types.Message{{Role: "user", Content: "refund?"}, {Role: "assistant", Content: "Your refund is issued."}}
	check := asrt.AssertionConfig{Type: "content_includes", Params: map[string]interface{}{
		"patterns": []interface{}{"refund"},
	}}

	assert.True(t, e.RunCheck(context.Background(), check, msgs, "c").Passed)
	check.Params["patterns"] = []interface{}{"store credit"}
	assert.False(t, e.RunCheck(context.Background(), check, msgs, "c").Passed)

	none := (&Engine{}).RunCheck(context.Background(), check, msgs, "c")
	assert.Equal(t, "no eval runner configured", none.Details["error"])
}
