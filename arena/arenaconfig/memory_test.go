package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigMemoryEnabled(t *testing.T) {
	tests := []struct {
		name   string
		memory interface{}
		want   bool
	}{
		{name: "no block", memory: nil, want: false},
		{name: "enabled true", memory: map[string]interface{}{"enabled": true}, want: true},
		{name: "enabled false", memory: map[string]interface{}{"enabled": false}, want: false},
		{name: "block without enabled stays on", memory: map[string]interface{}{}, want: true},
		{name: "yaml.v2 style map", memory: map[interface{}]interface{}{"enabled": false}, want: false},
		{name: "memory: false", memory: false, want: false},
		{name: "memory: true", memory: true, want: true},
		{name: "non-bool enabled stays on", memory: map[string]interface{}{"enabled": "no"}, want: true},
		{name: "other shape stays on", memory: "yes", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, (&Config{Memory: tt.memory}).MemoryEnabled())
		})
	}
}
