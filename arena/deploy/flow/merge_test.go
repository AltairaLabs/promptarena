package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// arenaManifest wraps a spec body in the k8s-style arena manifest envelope that
// LoadConfig expects (apiVersion/kind/metadata/spec).
func arenaManifest(specBody string) []byte {
	var b strings.Builder
	b.WriteString("apiVersion: promptkit.altairalabs.ai/v1alpha1\n")
	b.WriteString("kind: Arena\n")
	b.WriteString("metadata:\n  name: test\n")
	b.WriteString("spec:\n")
	b.WriteString(specBody)
	return []byte(b.String())
}

// parseDeployConfig is a test helper that extracts spec.deploy.config from a
// merged arena manifest.
func parseDeployConfig(t *testing.T, doc []byte) map[string]interface{} {
	t.Helper()
	var parsed struct {
		Spec struct {
			Deploy struct {
				Provider string                 `yaml:"provider"`
				Config   map[string]interface{} `yaml:"config"`
			} `yaml:"deploy"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(doc, &parsed); err != nil {
		t.Fatalf("re-parse merged doc: %v\n%s", err, doc)
	}
	return parsed.Spec.Deploy.Config
}

func TestMergeProfileIntoExistingConfig(t *testing.T) {
	doc := arenaManifest(`  deploy:
    provider: omnia
    config:
      region: us-east-1
`)
	profile := map[string]interface{}{
		"endpoint": "https://omnia.example.com",
		"token":    "secret-123",
	}

	merged, err := mergeProfileIntoConfigDoc(doc, profile, "omnia")
	if err != nil {
		t.Fatalf("mergeProfileIntoConfigDoc() error: %v", err)
	}

	cfg := parseDeployConfig(t, merged)
	if cfg["region"] != "us-east-1" {
		t.Errorf("existing region lost: got %v", cfg["region"])
	}
	if cfg["endpoint"] != "https://omnia.example.com" {
		t.Errorf("endpoint = %v, want merged in", cfg["endpoint"])
	}
	if cfg["token"] != "secret-123" {
		t.Errorf("token = %v, want merged in", cfg["token"])
	}
}

func TestMergeProfileOverridesExistingKey(t *testing.T) {
	doc := arenaManifest(`  deploy:
    provider: omnia
    config:
      token: old-token
`)
	profile := map[string]interface{}{"token": "new-token"}

	merged, err := mergeProfileIntoConfigDoc(doc, profile, "omnia")
	if err != nil {
		t.Fatalf("mergeProfileIntoConfigDoc() error: %v", err)
	}

	cfg := parseDeployConfig(t, merged)
	if cfg["token"] != "new-token" {
		t.Errorf("token = %v, want new-token (override)", cfg["token"])
	}
}

func TestMergeProfileDeepMergesNestedMaps(t *testing.T) {
	doc := arenaManifest(`  deploy:
    provider: omnia
    config:
      auth:
        mode: token
        scope: read
`)
	profile := map[string]interface{}{
		"auth": map[string]interface{}{
			"scope": "write",
			"realm": "prod",
		},
	}

	merged, err := mergeProfileIntoConfigDoc(doc, profile, "omnia")
	if err != nil {
		t.Fatalf("mergeProfileIntoConfigDoc() error: %v", err)
	}

	cfg := parseDeployConfig(t, merged)
	auth, ok := cfg["auth"].(map[string]interface{})
	if !ok {
		t.Fatalf("auth not a map: %v", cfg["auth"])
	}
	if auth["mode"] != "token" {
		t.Errorf("auth.mode = %v, want token (preserved)", auth["mode"])
	}
	if auth["scope"] != "write" {
		t.Errorf("auth.scope = %v, want write (overridden)", auth["scope"])
	}
	if auth["realm"] != "prod" {
		t.Errorf("auth.realm = %v, want prod (added)", auth["realm"])
	}
}

func TestMergeProfileCreatesDeploySection(t *testing.T) {
	doc := arenaManifest(`  providers:
    - file: providers/openai.yaml
`)
	profile := map[string]interface{}{"endpoint": "https://omnia.example.com"}

	merged, err := mergeProfileIntoConfigDoc(doc, profile, "omnia")
	if err != nil {
		t.Fatalf("mergeProfileIntoConfigDoc() error: %v", err)
	}

	var parsed struct {
		Spec struct {
			Deploy struct {
				Provider string                 `yaml:"provider"`
				Config   map[string]interface{} `yaml:"config"`
			} `yaml:"deploy"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(merged, &parsed); err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if parsed.Spec.Deploy.Provider != "omnia" {
		t.Errorf("deploy.provider = %q, want omnia", parsed.Spec.Deploy.Provider)
	}
	if parsed.Spec.Deploy.Config["endpoint"] != "https://omnia.example.com" {
		t.Errorf("deploy.config.endpoint = %v, want merged in", parsed.Spec.Deploy.Config["endpoint"])
	}
	// The existing spec content must survive.
	if !strings.Contains(string(merged), "providers/openai.yaml") {
		t.Errorf("existing spec content lost:\n%s", merged)
	}
}

func TestMergeProfileCreatesConfigUnderExistingDeploy(t *testing.T) {
	doc := arenaManifest(`  deploy:
    provider: omnia
`)
	profile := map[string]interface{}{"endpoint": "https://omnia.example.com"}

	merged, err := mergeProfileIntoConfigDoc(doc, profile, "omnia")
	if err != nil {
		t.Fatalf("mergeProfileIntoConfigDoc() error: %v", err)
	}

	cfg := parseDeployConfig(t, merged)
	if cfg["endpoint"] != "https://omnia.example.com" {
		t.Errorf("deploy.config.endpoint = %v, want merged in", cfg["endpoint"])
	}
}

func TestMergeProfileDoesNotOverrideExistingProvider(t *testing.T) {
	doc := arenaManifest(`  deploy:
    provider: agentcore
    config:
      region: us-east-1
`)
	profile := map[string]interface{}{"endpoint": "https://omnia.example.com"}

	merged, err := mergeProfileIntoConfigDoc(doc, profile, "omnia")
	if err != nil {
		t.Fatalf("mergeProfileIntoConfigDoc() error: %v", err)
	}

	var parsed struct {
		Spec struct {
			Deploy struct {
				Provider string `yaml:"provider"`
			} `yaml:"deploy"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(merged, &parsed); err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if parsed.Spec.Deploy.Provider != "agentcore" {
		t.Errorf("deploy.provider = %q, want agentcore (preserved)", parsed.Spec.Deploy.Provider)
	}
}

func TestMergeProfilePreservesComments(t *testing.T) {
	doc := []byte(`# Arena config for my project
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  deploy:
    provider: omnia
    config:
      # region is required
      region: us-east-1
`)
	profile := map[string]interface{}{"token": "secret-123"}

	merged, err := mergeProfileIntoConfigDoc(doc, profile, "omnia")
	if err != nil {
		t.Fatalf("mergeProfileIntoConfigDoc() error: %v", err)
	}

	out := string(merged)
	if !strings.Contains(out, "# Arena config for my project") {
		t.Errorf("top-level comment lost:\n%s", out)
	}
	if !strings.Contains(out, "# region is required") {
		t.Errorf("inline comment lost:\n%s", out)
	}
}

func TestMergeProfilePreservesTwoSpaceIndent(t *testing.T) {
	doc := arenaManifest(`  deploy:
    provider: omnia
    config:
      workspace: existing-ws
`)
	profile := map[string]interface{}{"token": "abc"}

	merged, err := mergeProfileIntoConfigDoc(doc, profile, "omnia")
	if err != nil {
		t.Fatalf("mergeProfileIntoConfigDoc() error: %v", err)
	}

	out := string(merged)
	// The whole document must keep the repo's 2-space indentation; a 4-space
	// reindent (yaml.Marshal's default) would rewrite every line.
	if !strings.Contains(out, "\n  name: test\n") {
		t.Errorf("metadata.name should stay at 2-space indent:\n%s", out)
	}
	if strings.Contains(out, "\n    name: test\n") {
		t.Errorf("document was reindented to 4 spaces:\n%s", out)
	}
}

func TestMergeProfileMissingSpecErrors(t *testing.T) {
	doc := []byte("apiVersion: promptkit.altairalabs.ai/v1alpha1\nkind: Arena\n")
	profile := map[string]interface{}{"endpoint": "https://omnia.example.com"}

	_, err := mergeProfileIntoConfigDoc(doc, profile, "omnia")
	if err == nil {
		t.Fatal("expected error when manifest has no spec: section")
	}
}

func TestMergeProfileIntoConfigFile_MissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")
	err := MergeProfileIntoConfigFile(path, map[string]interface{}{"workspace": "demo"}, "omnia")
	if err == nil || !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("expected config-not-found error, got %v", err)
	}
}

func TestMergeProfileIntoConfigFile_PreservesPerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "arena.yaml")
	manifest := arenaManifest(`  deploy:
    provider: omnia
    config:
      region: us-east-1
`)
	if err := os.WriteFile(path, manifest, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if err := MergeProfileIntoConfigFile(path, map[string]interface{}{"token": "abc"}, "omnia"); err != nil {
		t.Fatalf("MergeProfileIntoConfigFile: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read merged config: %v", err)
	}
	cfg := parseDeployConfig(t, data)
	if cfg["token"] != "abc" {
		t.Errorf("token = %v, want abc (merged from profile)", cfg["token"])
	}
	if cfg["region"] != "us-east-1" {
		t.Errorf("region = %v, want us-east-1 (preserved)", cfg["region"])
	}
}
