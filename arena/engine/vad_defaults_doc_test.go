package engine

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/agentkb"
)

// The VADConfig field comments become the scenario schema's descriptions,
// which editors show. They once gave 500 ms / 1000 ms / 60 s while the
// runtime used PromptKit's AudioTurnConfig defaults; pin them together.
func TestVADConfigSchemaDescriptionsStateRuntimeDefaults(t *testing.T) {
	raw, err := agentkb.Schema("scenario")
	require.NoError(t, err)
	var schema struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema))
	vad, ok := schema.Defs["VADConfig"]
	require.True(t, ok, "scenario schema has a VADConfig definition")

	d := stage.DefaultAudioTurnConfig()
	want := map[string]string{
		"silence_threshold_ms": fmt.Sprintf("(default: %d)", d.SilenceDuration.Milliseconds()),
		"min_speech_ms":        fmt.Sprintf("(default: %d)", d.MinSpeechDuration.Milliseconds()),
		"max_turn_duration_s":  fmt.Sprintf("(default: %d)", int(d.MaxTurnDuration.Seconds())),
	}
	for field, def := range want {
		assert.Contains(t, vad.Properties[field].Description, def, field)
	}
}
