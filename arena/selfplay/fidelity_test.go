package selfplay

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

func TestSpecifics(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"My order is A-100, placed 28 October.", []string{"A-100"}},
		{"Email jo.smith@example.com or call 0207 946 0018.", []string{"jo.smith@example.com", "0207", "946", "0018"}},
		{"Ticket #4471, at 10:30, ref 2024-10-28.", []string{"4471", "10:30", "2024-10-28"}},
		{"I have 2 kids and waited 15 minutes.", nil},
		{"Repeat: A-100 and a-100.", []string{"A-100"}},
		{"No specifics here at all.", nil},
		{"Order #4471 on my 2nd try, the 28th, 100% sure.", []string{"4471"}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, Specifics(tt.text), tt.text)
	}
}

func TestInventedSpecifics(t *testing.T) {
	assert.Equal(t, []string{"b-7731"}, InventedSpecifics("Order A-100 or b-7731? T-555.", "a-100", "Ticket T-555"))
	assert.Empty(t, InventedSpecifics("No specifics.", ""))
}

func TestPersonaGivenText(t *testing.T) {
	given := PersonaGivenText(&arenaconfig.UserPersonaPack{
		Description: "D", SystemPrompt: "P", Goals: []string{"Goal G-1"}, Constraints: []string{"C"},
		Facts:        []arenaconfig.PersonaFact{{ID: "o", Value: "A-100"}},
		OptionalVars: map[string]string{"k": "V-9"},
		State:        map[string]arenaconfig.PersonaStateVar{"t": {Type: "string", Initial: "TK-1"}},
	})
	for _, want := range []string{"a-100", "goal g-1", "v-9", "tk-1", "p"} {
		assert.Contains(t, given, want)
	}
}
