package generators

import (
	"fmt"
	"os"
	"reflect"
	"strings"

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

// arenaModulePath is this module's import path. Types under it keep the plain
// $defs name when a name is contested; see qualifyingNamer.
const arenaModulePath = "github.com/AltairaLabs/promptarena"

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

// packagesByTypeName records, for every named type the reflector asks to name,
// which packages define that name. Two entries for one name is a collision.
func packagesByTypeName(target interface{}, fieldNameTag string) map[string]map[string]bool {
	seen := map[string]map[string]bool{}
	r := newReflector(fieldNameTag)
	r.Namer = func(t reflect.Type) string {
		if t.Name() != "" && t.PkgPath() != "" {
			if seen[t.Name()] == nil {
				seen[t.Name()] = map[string]bool{}
			}
			seen[t.Name()][t.PkgPath()] = true
		}
		return "" // fall through to the default name for this survey pass
	}
	r.Reflect(target)
	return seen
}

// qualifyingNamer names $defs entries by the Go type's base name, prefixing the
// package's last path element when more than one package defines that name.
//
// invopop keys $defs by reflect.Type.Name() alone. PromptPack 1.6 had promptkit
// generate its pack types from the schema, which gave packspec a type named
// Eval — the same base name arena's own replay-eval config type already used.
// Both claimed "#/$defs/Eval", the second reflected silently replaced the
// first, and `eval_specs` started validating against the pack eval definition,
// rejecting every real eval config. Nothing failed loudly; the schema was just
// wrong.
//
// Arena's own types keep the plain name so the common case stays readable, and
// an uncontested name is never rewritten — only the intruder is qualified.
func qualifyingNamer(collisions map[string]map[string]bool) func(reflect.Type) string {
	return func(t reflect.Type) string {
		name, pkg := t.Name(), t.PkgPath()
		if name == "" || pkg == "" {
			return ""
		}
		pkgs := collisions[name]
		if len(pkgs) < 2 || strings.HasPrefix(pkg, arenaModulePath+"/") {
			return ""
		}
		return packQualifier(pkg) + name
	}
}

// packQualifier turns an import path into the prefix qualifyingNamer puts in
// front of a contested type name: the last path element, title-cased.
func packQualifier(pkgPath string) string {
	last := pkgPath[strings.LastIndex(pkgPath, "/")+1:]
	if last == "" {
		return ""
	}
	return strings.ToUpper(last[:1]) + last[1:]
}

// Generate produces a JSON schema for the given SchemaConfig. It creates
// a reflector with standard settings, reflects the target type, sets the
// schema metadata (version, ID, title, description), adds the $schema
// property, and applies any custom modifications.
func Generate(cfg *SchemaConfig) (interface{}, error) {
	reflector := newReflector(cfg.FieldNameTag)

	// A survey pass first, so the real pass knows which base names two packages
	// both want and can qualify the non-arena one. Reflecting twice is cheap and
	// keeps the naming deterministic — deciding on the fly would hand the plain
	// name to whichever type the reflector happened to reach first.
	reflector.Namer = qualifyingNamer(packagesByTypeName(cfg.Target, cfg.FieldNameTag))

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
