package generators

import (
	"fmt"
	"os"

	"github.com/invopop/jsonschema"
)

// goCommentDirs maps a Go module base import path to a repo-relative source
// directory whose doc comments should populate schema field descriptions. The
// comment-map key invopop builds is gopath.Join(base, dir(file)), which must
// equal the reflected type's PkgPath — so base+dir together must reproduce the
// full import path, and the generator must run from the repo root for the
// relative dir to resolve.
var goCommentDirs = []struct{ base, dir string }{
	{"github.com/AltairaLabs/promptarena", "arena/arenaconfig"},
}

// draftSchemaVersion is the JSON Schema draft all generated schemas declare.
const draftSchemaVersion = "https://json-schema.org/draft-07/schema"

// jsonTypeString is the JSON Schema primitive type name for strings.
const jsonTypeString = "string"

// SchemaConfig holds the configuration for generating a JSON schema from a Go type.
type SchemaConfig struct {
	// Target is the Go struct instance to reflect (e.g., &arenaconfig.ArenaConfig{}).
	Target interface{}
	// Filename is the schema output filename (e.g., "arena.json").
	Filename string
	// Title is the human-readable schema title.
	Title string
	// Description is the schema description.
	Description string
	// FieldNameTag overrides the struct tag used for field names.
	// Defaults to "yaml" if empty.
	FieldNameTag string
	// Customize is an optional callback to apply additional modifications
	// to the generated schema (e.g., adding examples or oneOf constraints).
	Customize func(*jsonschema.Schema)
}

// newReflector creates a jsonschema.Reflector with the standard configuration
// used across all schema generators.
func newReflector(fieldNameTag string) jsonschema.Reflector {
	if fieldNameTag == "" {
		fieldNameTag = "yaml"
	}
	return jsonschema.Reflector{
		AllowAdditionalProperties:  false,
		ExpandedStruct:             true,
		FieldNameTag:               fieldNameTag,
		RequiredFromJSONSchemaTags: false,
	}
}

// Generate produces a JSON schema for the given SchemaConfig. It creates
// a reflector with standard settings, reflects the target type, sets the
// schema metadata (version, ID, title, description), adds the $schema
// property, and applies any custom modifications.
func Generate(cfg *SchemaConfig) (interface{}, error) {
	reflector := newReflector(cfg.FieldNameTag)

	// Populate field descriptions from Go doc comments on arena-local config
	// types. Harmless for schemas whose target lives elsewhere (no keys match).
	// Best-effort: when the source tree isn't reachable from the cwd (e.g. unit
	// tests running from the package dir), skip rather than fail — the CLI
	// chdirs to the repo root so comments are extracted for real regeneration.
	for _, c := range goCommentDirs {
		if _, statErr := os.Stat(c.dir); statErr != nil {
			continue
		}
		if err := reflector.AddGoComments(c.base, c.dir); err != nil {
			return nil, fmt.Errorf("extract go comments from %s: %w", c.dir, err)
		}
	}

	schema := reflector.Reflect(cfg.Target)

	schema.Version = draftSchemaVersion
	schema.ID = jsonschema.ID(schemaBaseURL + "/" + cfg.Filename)
	schema.Title = cfg.Title
	schema.Description = cfg.Description

	allowSchemaField(schema)

	if cfg.Customize != nil {
		cfg.Customize(schema)
	}

	return schema, nil
}
