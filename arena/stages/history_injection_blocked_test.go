package stages

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestSwapMessageRole_DropsBlockedInputMarks(t *testing.T) {
	tests := []struct {
		name     string
		msg      types.Message
		wantRole string
		wantMeta map[string]interface{}
	}{
		{
			name: "canned reply loses its mark",
			msg: types.Message{Role: "assistant", Meta: map[string]interface{}{
				types.MetaBlockedInput: true, "validator_type": "topic_policy",
			}},
			wantRole: "user",
			wantMeta: map[string]interface{}{"validator_type": "topic_policy"},
		},
		{
			name: "late-blocked spoken request loses both marks",
			msg: types.Message{Role: "user", Meta: map[string]interface{}{
				types.MetaBlockedInput: true, blockedReplyBeforeMetaKey: true,
			}},
			wantRole: "assistant",
			wantMeta: map[string]interface{}{},
		},
		{
			name:     "unmarked message keeps its meta",
			msg:      types.Message{Role: "user", Meta: map[string]interface{}{"k": "v"}},
			wantRole: "assistant",
			wantMeta: map[string]interface{}{"k": "v"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(tt.msg.Meta)
			got := swapMessageRole(&tt.msg)
			assert.Equal(t, tt.wantRole, got.Role)
			assert.Equal(t, tt.wantMeta, got.Meta)
			assert.Len(t, tt.msg.Meta, before, "the original message's meta is not written")
		})
	}
}
