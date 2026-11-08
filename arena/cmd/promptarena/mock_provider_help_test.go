package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The --mock-provider help on run, chat and serve must say what
// EnableMockProviderMode really does: LLM and self-play role providers are
// swapped, TTS and STT are not, and a keyless voice run needs a type: mock,
// role: tts provider (not the example id mock-tts).
func TestMockProviderHelp_SaysWhatIsReplaced(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage string
	}{
		{"run", runCmd.Flags().Lookup("mock-provider").Usage},
		{"chat", chatCmd.Flags().Lookup("mock-provider").Usage},
		{"serve", serveCmd.Flags().Lookup("mock-provider").Usage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotEmpty(t, tc.usage)
			assert.Contains(t, tc.usage, mockProviderFlagHelp)
			assert.Contains(t, tc.usage, "TTS and STT providers are not replaced")
			assert.Contains(t, tc.usage, "type: mock, role: tts")
			assert.NotContains(t, tc.usage, "mock-tts")
			assert.NotContains(t, tc.usage, "all providers")
		})
	}
}

func TestMockProviderModeLog_DoesNotClaimEveryProvider(t *testing.T) {
	assert.NotContains(t, mockProviderModeLog, "ALL")
	assert.Contains(t, mockProviderModeLog, "TTS and STT providers unchanged")
}
