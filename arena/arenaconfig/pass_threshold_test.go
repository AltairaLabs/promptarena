package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const passThresholdScenario = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: s
spec:
  id: s
  task_type: support
  description: d
  trials: 3
  turns:
    - role: user
      content: hi
      assertions:
        - type: content_includes
          params: {patterns: ["x"]}
          pass_threshold: `

// pass_threshold is a pass rate: a value outside 0..1 used to load and then
// make the assertion fail every trial set (above 1) or never fail (below 0).
func TestLoadScenario_RejectsPassThresholdOutOfRange(t *testing.T) {
	for _, v := range []string{"1.5", "-0.1"} {
		_, err := LoadScenario(writeLoaderSchemaFile(t, passThresholdScenario+v+"\n"))
		require.Error(t, err, "pass_threshold %s", v)
		assert.Contains(t, err.Error(), "pass_threshold")
	}
}

func TestLoadScenario_AcceptsPassThresholdInRange(t *testing.T) {
	for _, v := range []string{"0", "0.6667", "1"} {
		_, err := LoadScenario(writeLoaderSchemaFile(t, passThresholdScenario+v+"\n"))
		require.NoError(t, err, "pass_threshold %s", v)
	}
}
