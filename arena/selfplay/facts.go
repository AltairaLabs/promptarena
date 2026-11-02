package selfplay

import (
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// factsRule tells a persona with declared facts to state no other specifics.
const factsRule = `

The specifics you know (numbers, dates, names, codes, addresses) are only
the facts listed for you. Never make up any other specific. If you are asked
for something that is not in your facts, say you don't have it.`

// FactsInstruction returns the text appended to the persona's system prompt
// when it declares facts: the rule, and the facts themselves unless its
// system_template already places them with {{facts}}. It is empty for a
// persona without facts.
func FactsInstruction(persona *arenaconfig.UserPersonaPack) string {
	if persona == nil || len(persona.Facts) == 0 {
		return ""
	}
	if strings.Contains(persona.SystemTemplate, "{{"+arenaconfig.PersonaVarFacts+"}}") {
		return factsRule
	}
	return factsRule + "\nYour facts:\n" + persona.FactsText()
}
