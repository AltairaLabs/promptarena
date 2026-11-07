package compiler

import (
	"fmt"
	"regexp"
)

// rePackVersion is the PromptPack schema's pattern for the pack version:
// Semantic Versioning 2.0.0 with an optional "v" prefix.
var rePackVersion = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// validatePackVersion rejects a version the PromptPack schema would reject, so a
// bad --pack-version fails before compiling rather than at schema validation.
func validatePackVersion(v string) error {
	if !rePackVersion.MatchString(v) {
		return fmt.Errorf("invalid pack version %q: must be a semantic version such as 1.2.0 or v1.2.0", v)
	}
	return nil
}
