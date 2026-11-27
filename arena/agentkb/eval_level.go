package agentkb

import (
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
)

// Eval levels, as evalmeta.yaml records them.
const (
	EvalLevelTurn    = "turn"
	EvalLevelSession = "session"
)

var (
	evalLevelsOnce sync.Once
	evalLevels     map[string]string
)

// EvalLevel reports whether evalType judges a single turn (EvalLevelTurn) or
// the whole session (EvalLevelSession), as evalmeta.yaml records it. An alias
// reports its target's level. It returns "" for a type evalmeta.yaml does not
// describe, such as one a kit registers itself.
func EvalLevel(evalType string) string {
	evalLevelsOnce.Do(func() {
		evalLevels = map[string]string{}
		meta, err := loadEvalMeta()
		if err != nil {
			return
		}
		for name, h := range meta.Handlers {
			evalLevels[name] = h.Level
		}
		for _, pair := range evals.DefaultAliases() {
			if level, ok := evalLevels[pair[1]]; ok {
				evalLevels[pair[0]] = level
			}
		}
	})
	return evalLevels[evalType]
}
