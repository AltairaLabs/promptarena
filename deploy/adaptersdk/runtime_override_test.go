package adaptersdk

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/deploy"
)

func TestValidateRuntimeOverride_NilIsValid(t *testing.T) {
	if errs := ValidateRuntimeOverride(nil, RuntimeOverrideSupport{}); len(errs) != 0 {
		t.Errorf("errs = %v, want none", errs)
	}
}

func TestValidateRuntimeOverride_FullySupported(t *testing.T) {
	o := &deploy.RuntimeOverride{
		Image:     "img",
		Command:   []string{"/run"},
		Args:      []string{"-x"},
		Env:       map[string]string{"A_1": "x", "_B": "y"},
		SecretEnv: map[string]deploy.SecretRef{"KEY": {Name: "s"}},
	}
	s := RuntimeOverrideSupport{Command: true, Args: true, SecretEnv: true, ReservedEnvPrefixes: []string{"PROMPTPACK_"}}
	if errs := ValidateRuntimeOverride(o, s); len(errs) != 0 {
		t.Errorf("errs = %v, want none", errs)
	}
}

// A field the platform cannot apply is refused, never dropped: deploying
// without it would run something other than what the config describes.
func TestValidateRuntimeOverride_RefusesUnsupportedFields(t *testing.T) {
	o := &deploy.RuntimeOverride{
		Command:   []string{"/run"},
		Args:      []string{"-x"},
		SecretEnv: map[string]deploy.SecretRef{"KEY": {Name: "s"}},
	}
	errs := ValidateRuntimeOverride(o, RuntimeOverrideSupport{})
	want := []string{"runtime_override.command", "runtime_override.args", "runtime_override.secret_env is not supported"}
	if len(errs) != len(want) {
		t.Fatalf("errs = %v, want %d", errs, len(want))
	}
	for i, w := range want {
		if !strings.Contains(errs[i], w) {
			t.Errorf("errs[%d] = %q, want it to contain %q", i, errs[i], w)
		}
	}
}

func TestValidateRuntimeOverride_EnvNames(t *testing.T) {
	o := &deploy.RuntimeOverride{
		Env: map[string]string{
			"1BAD":            "x",
			"HAS-DASH":        "x",
			"PROMPTPACK_PACK": "x",
			"PORT":            "x",
			"FINE":            "x",
		},
		SecretEnv: map[string]deploy.SecretRef{
			"FINE":           {Name: "s"},
			"PROMPTPACK_KEY": {Name: "s"},
			"NO_NAME":        {Name: "  "},
		},
	}
	s := RuntimeOverrideSupport{
		SecretEnv:           true,
		ReservedEnvPrefixes: []string{"PROMPTPACK_"},
		ReservedEnvNames:    []string{"PORT"},
	}
	got := ValidateRuntimeOverride(o, s)
	want := []string{
		`runtime_override.env "1BAD" is not a valid environment variable name`,
		`runtime_override.env "HAS-DASH" is not a valid environment variable name`,
		`runtime_override.env "PORT" is reserved: the adapter or platform sets it`,
		`runtime_override.env "PROMPTPACK_PACK" is reserved: PROMPTPACK_* names are set by the adapter`,
		`runtime_override.secret_env "FINE" is also set in runtime_override.env: set it in one place`,
		`runtime_override.secret_env "NO_NAME" needs a secret name`,
		`runtime_override.secret_env "PROMPTPACK_KEY" is reserved: PROMPTPACK_* names are set by the adapter`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("errs =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The schema must describe exactly the fields RuntimeOverride decodes, or an
// adapter that validates against it would reject a config the SDK accepts.
func TestRuntimeOverrideSchema_MatchesTheType(t *testing.T) {
	schema := RuntimeOverrideSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", schema["properties"])
	}
	var fields []string
	typ := reflect.TypeOf(deploy.RuntimeOverride{})
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		fields = append(fields, name)
		if _, ok := props[name]; !ok {
			t.Errorf("schema has no property for field %q", name)
		}
	}
	if len(props) != len(fields) {
		t.Errorf("schema has %d properties, type has %d fields", len(props), len(fields))
	}
	if _, err := json.Marshal(schema); err != nil {
		t.Errorf("schema does not marshal: %v", err)
	}
}

// The pack handoff names are reserved even when the adapter declares nothing,
// so no override can displace the pack the replacement runtime reads.
func TestValidateRuntimeOverride_HandoffNamesAlwaysReserved(t *testing.T) {
	o := &deploy.RuntimeOverride{Env: map[string]string{
		deploy.EnvPackJSON: "{}", deploy.EnvPackURI: "x", deploy.EnvAgent: "a",
	}}
	got := ValidateRuntimeOverride(o, RuntimeOverrideSupport{})
	if len(got) != 3 {
		t.Fatalf("errs = %v, want one per handoff name", got)
	}
	for _, e := range got {
		if !strings.Contains(e, "is reserved") {
			t.Errorf("err %q does not say the name is reserved", e)
		}
	}
}
