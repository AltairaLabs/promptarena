package agentkb

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

// The embedded schemas are a generated mirror of schemas/v1alpha1 (the source of
// truth produced by schema-gen). Regenerate with `go generate ./arena/agentkb/...`
// or `make schemas`; TestSchemas_ByteMatchGeneratedSource guards against drift.
//
//go:generate sh -c "cp ../../schemas/v1alpha1/*.json schemas/ && cp ../../schemas/v1alpha1/common/*.json schemas/common/"

//go:embed schemas/*.json schemas/common/*.json
var schemasFS embed.FS

// SchemaNames lists the top-level config schema type names (sorted), excluding the
// common/ $ref directory.
func SchemaNames() ([]string, error) {
	entries, err := schemasFS.ReadDir("schemas")
	if err != nil {
		return nil, fmt.Errorf("read schemas dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names, nil
}

// SchemaFS returns the embedded schemas as a filesystem with <type>.json at its
// root, the layout config.UseSchemaFS expects.
func SchemaFS() fs.FS {
	sub, err := fs.Sub(schemasFS, "schemas")
	if err != nil {
		// fs.Sub fails only for an invalid path, and "schemas" is a constant.
		panic(fmt.Sprintf("agentkb: embedded schemas: %v", err))
	}
	return sub
}

// UseEmbeddedSchemas makes every config validation in this process use the
// schemas embedded in the binary — the ones `promptarena schema` prints —
// instead of fetching the hosted copy or finding a schemas/ directory relative
// to the working directory. Without it the same binary could accept a config
// in one directory and reject it in another, and validate against a newer
// schema than it was built with. Both CLIs call it first thing in main.
func UseEmbeddedSchemas() {
	config.UseSchemaFS(SchemaFS())
}

// Schema returns the raw JSON schema bytes for a config type (e.g. "scenario").
func Schema(name string) ([]byte, error) {
	b, err := schemasFS.ReadFile("schemas/" + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("unknown schema %q: %w", name, err)
	}
	return b, nil
}
