package assertions

// Arena's own eval types register themselves with PromptKit's default eval
// registry, so every package that uses assertions can run them.
import (
	_ "github.com/AltairaLabs/promptarena/v2/arena/assertions/canary"  // the canary eval
	_ "github.com/AltairaLabs/promptarena/v2/arena/assertions/refusal" // the refusal eval
)
