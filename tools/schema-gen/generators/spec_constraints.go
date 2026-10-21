package generators

import (
	"fmt"

	"github.com/invopop/jsonschema"
)

// spec_patterns.go copies patterns from the spec; this file copies the other
// two kinds of constraint the reflector cannot see on packspec types: closed
// enums and property counts. As with patterns, the code decides which arena
// definition a spec rule applies to and the spec supplies the rule, so the two
// cannot drift. A pointer that stops resolving fails generation, so a spec
// rename surfaces on the dependency bump instead of silently dropping a check.

// specEnum names a property in a generated definition and the JSON Pointer of
// the spec enum it must carry. items marks an array property whose elements
// carry the enum.
type specEnum struct {
	def, property string
	items         bool
	pointer       string
}

// specEnums lists the closed enums of packspec types arena embeds as-is.
var specEnums = []specEnum{
	// RFC 0013.
	{def: "Governance", property: "autonomy_level",
		pointer: "/$defs/Governance/properties/autonomy_level/enum"},
	// RFC 0016.
	{def: "GovernanceIndependentOf", property: "axes", items: true,
		pointer: "/$defs/Governance/properties/independent_of/properties/axes/items/enum"},
	{def: "GovernanceIndependentOf", property: "enforcement",
		pointer: "/$defs/Governance/properties/independent_of/properties/enforcement/enum"},
}

// specPropertyCount names a generated definition and the JSON Pointer of the
// spec object whose minProperties/maxProperties it must carry.
type specPropertyCount struct {
	def, pointer string
}

// specPropertyCounts lists the objects the spec bounds by property count.
var specPropertyCounts = []specPropertyCount{
	// RFC 0016: a control carries exactly one of field, validator, eval and
	// external.
	{def: "ObligationControl", pointer: "/$defs/ObligationControl"},
}

// applySpecEnums closes the generated properties listed in specEnums to the
// spec's values. A definition or property absent from the schema is skipped.
func applySpecEnums(schema *jsonschema.Schema) error {
	if schema == nil || schema.Definitions == nil {
		return nil
	}
	spec, err := parseEmbeddedSpec()
	if err != nil {
		return err
	}
	for _, se := range specEnums {
		node, err := resolvePointer(spec, se.pointer)
		if err != nil {
			return fmt.Errorf("resolving %s for $defs/%s.%s: %w", se.pointer, se.def, se.property, err)
		}
		enum, ok := node.([]interface{})
		if !ok || len(enum) == 0 {
			return fmt.Errorf("%s for $defs/%s.%s is not a non-empty enum", se.pointer, se.def, se.property)
		}
		for _, def := range definitionsNamed(schema, se.def) {
			if err := setSpecEnum(def, se, enum); err != nil {
				return err
			}
		}
	}
	return nil
}

// setSpecEnum sets enum on def's property, or on its elements when se.items.
// The elements get a fresh schema: the reflector emits an untyped element as
// the shared jsonschema.TrueSchema, which must not be written through.
func setSpecEnum(def *jsonschema.Schema, se specEnum, enum []interface{}) error {
	if def.Properties == nil {
		return nil
	}
	prop, ok := def.Properties.Get(se.property)
	if !ok || prop == nil {
		return nil
	}
	if !se.items {
		prop.Enum = enum
		return nil
	}
	if prop.Type != "array" {
		return fmt.Errorf("$defs/%s.%s is not an array", se.def, se.property)
	}
	items := &jsonschema.Schema{}
	if prop.Items != nil && prop.Items != jsonschema.TrueSchema {
		*items = *prop.Items
	}
	items.Enum = enum
	prop.Items = items
	return nil
}

// applySpecPropertyCounts copies minProperties and maxProperties from the spec
// onto the definitions listed in specPropertyCounts.
func applySpecPropertyCounts(schema *jsonschema.Schema) error {
	if schema == nil || schema.Definitions == nil {
		return nil
	}
	spec, err := parseEmbeddedSpec()
	if err != nil {
		return err
	}
	for _, pc := range specPropertyCounts {
		minProps, maxProps, err := specPropertyBounds(spec, pc)
		if err != nil {
			return err
		}
		for _, def := range definitionsNamed(schema, pc.def) {
			def.MinProperties = minProps
			def.MaxProperties = maxProps
		}
	}
	return nil
}

// specPropertyBounds reads pc's minProperties and maxProperties from the spec;
// either may be nil, but not both.
func specPropertyBounds(spec map[string]interface{}, pc specPropertyCount) (minProps, maxProps *uint64, err error) {
	node, err := resolvePointer(spec, pc.pointer)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving %s for $defs/%s: %w", pc.pointer, pc.def, err)
	}
	obj, ok := node.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("%s for $defs/%s does not name an object", pc.pointer, pc.def)
	}
	if n, ok := specCount(obj, "minProperties"); ok {
		minProps = &n
	}
	if n, ok := specCount(obj, "maxProperties"); ok {
		maxProps = &n
	}
	if minProps == nil && maxProps == nil {
		return nil, nil, fmt.Errorf("%s for $defs/%s sets neither minProperties nor maxProperties", pc.pointer, pc.def)
	}
	return minProps, maxProps, nil
}

// specCount reads a non-negative integer keyword from a decoded spec object.
func specCount(obj map[string]interface{}, key string) (uint64, bool) {
	f, ok := obj[key].(float64)
	if !ok || f < 0 || f != float64(uint64(f)) {
		return 0, false
	}
	return uint64(f), true
}
