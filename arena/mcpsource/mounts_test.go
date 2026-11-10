package mcpsource

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kit returns a config directory holding fixtures/ (a directory) and
// data.json (a file), resolved through any symlinks in the temp path.
func kit(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(base, "fixtures"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "data.json"), []byte("{}"), 0o600))
	return base
}

func TestResolveMounts_ResolvesAndDefaults(t *testing.T) {
	base := kit(t)
	raw := []any{
		map[string]any{"source": "fixtures", "target": "/fixtures"},
		map[string]any{"source": "data.json", "target": "/data/data.json", "readonly": false},
		map[string]any{"source": "fixtures", "target": "/workspace", "mode": "copy"},
		map[string]any{"source": "fixtures", "target": "/eval", "mode": "copy", "when": "eval"},
	}

	got, err := ResolveMounts(raw, base, "session")
	require.NoError(t, err)

	assert.Equal(t, []Mount{
		{Source: filepath.Join(base, "fixtures"), Target: "/fixtures", ReadOnly: true, Mode: MountBind, When: MountAtStart},
		{Source: filepath.Join(base, "data.json"), Target: "/data/data.json", ReadOnly: false, Mode: MountBind, When: MountAtStart},
		{Source: filepath.Join(base, "fixtures"), Target: "/workspace", ReadOnly: true, Mode: MountCopy, When: MountAtStart},
		{Source: filepath.Join(base, "fixtures"), Target: "/eval", ReadOnly: true, Mode: MountCopy, When: MountAtEval},
	}, got)
}

// Programmatic callers build []map[string]any; YAML yields []any.
func TestResolveMounts_AcceptsTypedList(t *testing.T) {
	base := kit(t)
	got, err := ResolveMounts([]map[string]any{{"source": "fixtures", "target": "/f"}}, base, "session")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, filepath.Join(base, "fixtures"), got[0].Source)
}

func TestResolveMounts_Errors(t *testing.T) {
	base := kit(t)
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(base, "escape")))

	m := func(kv ...any) map[string]any {
		out := map[string]any{}
		for i := 0; i < len(kv); i += 2 {
			out[kv[i].(string)] = kv[i+1]
		}
		return out
	}
	tests := []struct {
		name  string
		raw   any
		scope string
		want  string
	}{
		{"not a list", "fixtures", "session", "must be a list"},
		{"entry not a map", []any{"fixtures"}, "session", "mounts[0]: must be a map"},
		{"dotdot escape", []any{m("source", "../x", "target", "/x")}, "session", "outside the config directory"},
		{"symlink escape", []any{m("source", "escape", "target", "/x")}, "session", "outside the config directory"},
		{"absolute outside", []any{m("source", outside, "target", "/x")}, "session", "outside the config directory"},
		{"missing source path", []any{m("source", "nope", "target", "/x")}, "session", "does not exist"},
		{"missing source key", []any{m("target", "/x")}, "session", "mounts[0] (/x): source is required"},
		{"missing target key", []any{m("source", "fixtures")}, "session", "mounts[0]: target is required"},
		{"relative target", []any{m("source", "fixtures", "target", "x")}, "session", "target must be an absolute path"},
		{"duplicate target", []any{m("source", "fixtures", "target", "/x"), m("source", "data.json", "target", "/x/")},
			"session", "mounts[1] (/x/): target already used by mounts[0]"},
		{"unknown key", []any{m("source", "fixtures", "target", "/x", "readOnly", true)}, "session", `unknown key "readOnly"`},
		{"readonly not bool", []any{m("source", "fixtures", "target", "/x", "readonly", "yes")}, "session", "readonly must be true or false"},
		{"bad mode", []any{m("source", "fixtures", "target", "/x", "mode", "link")}, "session", `mode must be "bind" or "copy"`},
		{"bad when", []any{m("source", "fixtures", "target", "/x", "when", "later")}, "session", `when must be "start" or "eval"`},
		{"eval bind", []any{m("source", "fixtures", "target", "/x", "when", "eval")}, "session", "when: eval needs mode: copy"},
		{"eval outside session", []any{m("source", "fixtures", "target", "/x", "mode", "copy", "when", "eval")},
			"scenario", "when: eval needs scope: session"},
	}
	for _, tt := range tests {
		got, err := ResolveMounts(tt.raw, base, tt.scope)
		if assert.Error(t, err, tt.name) {
			assert.Contains(t, err.Error(), tt.want, tt.name)
		}
		assert.Nil(t, got, "%s: a failed resolve returns no mounts", tt.name)
	}
}

func TestMountArgs_RoundTrips(t *testing.T) {
	base := kit(t)
	ms, err := ResolveMounts([]any{map[string]any{"source": "fixtures", "target": "/f", "mode": "copy"}}, base, "session")
	require.NoError(t, err)

	args := MountArgs(ms)
	assert.Equal(t, []any{map[string]any{
		"source": filepath.Join(base, "fixtures"), "target": "/f", "readonly": true, "mode": "copy", "when": "start",
	}}, args)

	// Resolving the normalised form again is stable: absolute paths inside the
	// kit pass, so the engine and the source can both run it.
	again, err := ResolveMounts(args, base, "session")
	require.NoError(t, err)
	assert.Equal(t, ms, again)
}

func TestExpandArgs_ExpandsScenarioVariablesRecursively(t *testing.T) {
	args := map[string]any{
		"image":  "img",
		"mounts": []any{map[string]any{"source": "fixtures/{{scenario.case}}", "target": "/w"}},
		"n":      3,
	}
	got := ExpandArgs(args, map[string]string{"case": "add"})
	assert.Equal(t, map[string]any{
		"image":  "img",
		"mounts": []any{map[string]any{"source": "fixtures/add", "target": "/w"}},
		"n":      3,
	}, got)
	assert.Nil(t, ExpandArgs(nil, map[string]string{"case": "add"}))
}
