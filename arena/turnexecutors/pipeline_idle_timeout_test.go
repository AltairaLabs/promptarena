package turnexecutors

import (
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
)

// idleTimeoutProvider is a provider whose stream_idle_timeout is set, as every
// provider built on providers.BaseProvider reports it.
type idleTimeoutProvider struct {
	providers.Provider
	idle time.Duration
}

func (p idleTimeoutProvider) StreamIdleTimeout() time.Duration { return p.idle }

// TestPipelineConfig_IdleTimeoutFollowsProviderStreamIdleTimeout pins the fix
// for reasoning models cancelled mid-thought. The pipeline's 30s idle default
// fired before a provider's longer stream_idle_timeout, so gemini-3.1-pro and
// gemini-4-argon failed capability-matrix scenarios with "pipeline idle
// timeout" although their providers allowed the silence.
func TestPipelineConfig_IdleTimeoutFollowsProviderStreamIdleTimeout(t *testing.T) {
	def := stage.DefaultPipelineConfig().IdleTimeout
	base := mock.NewProvider("p", "m", false)
	tests := []struct {
		name     string
		provider providers.Provider
		want     time.Duration
	}{
		{"longer provider timeout wins", idleTimeoutProvider{base, 3 * time.Minute}, 3 * time.Minute},
		{"shorter provider timeout keeps the default", idleTimeoutProvider{base, time.Second}, def},
		{"provider without one keeps the default", base, def},
		{"no provider keeps the default", nil, def},
	}
	e := &PipelineExecutor{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.pipelineConfig(tt.provider).IdleTimeout; got != tt.want {
				t.Errorf("IdleTimeout = %v, want %v", got, tt.want)
			}
		})
	}
}
