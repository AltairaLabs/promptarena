package selfplay

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// A turn an input guardrail blocked is answered by a canned reply. The agent's
// model never sees that pair again, but the self-play user does: it is what
// the end user was shown. Role-swapped, the canned reply becomes the persona's
// incoming "user" message; it must reach the persona's model, or the last
// message the persona sees is its own and it answers itself.
func TestNextUserTurn_PersonaSeesGuardrailCannedReply(t *testing.T) {
	prov := newCapturingProvider("Where is my parcel?")
	gen := NewContentGenerator(prov, overridesTestPersona())

	history := []types.Message{
		{Role: "user", Content: "Who should I vote for?"},
		{
			Role:    "assistant",
			Content: "I can only help with parcels.",
			Meta:    map[string]interface{}{types.MetaBlockedInput: true},
		},
	}

	_, err := gen.NextUserTurn(context.Background(), history, "s1", nil)
	require.NoError(t, err)

	msgs := prov.lastRequest(t).Messages
	require.Len(t, msgs, 2, "both messages of the blocked turn reach the persona")
	assert.Equal(t, "assistant", msgs[0].Role)
	assert.Equal(t, "Who should I vote for?", msgs[0].Content)
	assert.Equal(t, "user", msgs[1].Role)
	assert.Equal(t, "I can only help with parcels.", msgs[1].Content)
	assert.Equal(t, "assistant", history[1].Role, "the caller's history is not changed")
	assert.Equal(t, true, history[1].Meta[types.MetaBlockedInput], "the caller's mark is kept")
}
