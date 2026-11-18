package deploy

import (
	"strings"
	"testing"
)

func TestRuntimeOverrideFromConfig(t *testing.T) {
	cfg := `{
		"project": "p",
		"runtime_override": {
			"image": "registry.example/runtime:1",
			"command": ["/bin/run"],
			"args": ["-v"],
			"env": {"A": "1"},
			"secret_env": {"KEY": {"name": "my-secret", "version": "3"}}
		}
	}`
	got, err := RuntimeOverrideFromConfig(cfg)
	if err != nil {
		t.Fatalf("RuntimeOverrideFromConfig: %v", err)
	}
	if got.Image != "registry.example/runtime:1" {
		t.Errorf("Image = %q", got.Image)
	}
	if len(got.Command) != 1 || got.Command[0] != "/bin/run" {
		t.Errorf("Command = %v", got.Command)
	}
	if len(got.Args) != 1 || got.Args[0] != "-v" {
		t.Errorf("Args = %v", got.Args)
	}
	if got.Env["A"] != "1" {
		t.Errorf("Env = %v", got.Env)
	}
	if ref := got.SecretEnv["KEY"]; ref.Name != "my-secret" || ref.Version != "3" {
		t.Errorf("SecretEnv[KEY] = %+v", ref)
	}
}

// Absent, null and empty blocks all mean "deploy the adapter's runtime", so
// they must all come back nil — the CLI's capability check keys off nil.
func TestRuntimeOverrideFromConfig_AbsentIsNil(t *testing.T) {
	for name, cfg := range map[string]string{
		"empty config": "",
		"no block":     `{"project":"p"}`,
		"null block":   `{"runtime_override":null}`,
		"empty block":  `{"runtime_override":{}}`,
		"empty fields": `{"runtime_override":{"env":{},"command":[]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := RuntimeOverrideFromConfig(cfg)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != nil {
				t.Errorf("got %+v, want nil", got)
			}
		})
	}
}

func TestRuntimeOverrideFromConfig_Malformed(t *testing.T) {
	for name, tc := range map[string]struct{ cfg, want string }{
		"not JSON":      {`{`, "invalid config JSON"},
		"wrong type":    {`{"runtime_override":"image"}`, "runtime_override:"},
		"env not a map": {`{"runtime_override":{"env":["A"]}}`, "runtime_override:"},
		"unknown key":   {`{"runtime_override":{"imgae":"x"}}`, `unknown field "imgae"`},
		"numeric env":   {`{"runtime_override":{"env":{"PORT":8080}}}`, "quote numeric values"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := RuntimeOverrideFromConfig(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestRuntimeOverride_IsZero(t *testing.T) {
	var nilOverride *RuntimeOverride
	if !nilOverride.IsZero() {
		t.Error("nil override must be zero")
	}
	if !(&RuntimeOverride{}).IsZero() {
		t.Error("empty override must be zero")
	}
	if (&RuntimeOverride{Args: []string{"x"}}).IsZero() {
		t.Error("override with args must not be zero")
	}
	if (&RuntimeOverride{SecretEnv: map[string]SecretRef{"K": {Name: "s"}}}).IsZero() {
		t.Error("override with secret_env must not be zero")
	}
}
