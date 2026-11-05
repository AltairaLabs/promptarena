package redteam

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestReadObjectivesFile_JSONLAliasesAndBenign(t *testing.T) {
	p := writeTemp(t, "o.jsonl", `{"id": "a", "goal": "Get A", "win": "done A", "taxonomy": "owasp-llm:LLM06", "n": 3}

{"Objective": "Get B", "Category": "cat", "prompt": "Please B"}
{"id": "c", "behavior": "Ask C", "benign": true}
{"id": "d", "objective": "Ask D", "benign": "yes", "extra": null}
`)
	objs, err := ReadObjectivesFile(p)
	require.NoError(t, err)
	require.Len(t, objs.Harmful, 2)
	require.Len(t, objs.Benign, 2)
	assert.Equal(t, Objective{ID: "a", Text: "Get A", SuccessPattern: "done A", Taxonomy: "owasp-llm:LLM06"},
		objs.Harmful[0])
	assert.Equal(t, Objective{ID: "obj-2", Text: "Get B", Prompt: "Please B", Category: "cat"}, objs.Harmful[1])
	assert.Equal(t, "c", objs.Benign[0].ID)
	assert.Equal(t, "Ask D", objs.Benign[1].Text)
}

func TestReadObjectivesFile_CSVWithBOM(t *testing.T) {
	p := writeTemp(t, "o.csv", string(rune(byteOrderMark))+"id,objective,success_pattern,benign\n"+
		"x,Get X,x done,false\ny,Ask Y,,1\n")
	objs, err := ReadObjectivesFile(p)
	require.NoError(t, err)
	require.Len(t, objs.Harmful, 1)
	assert.Equal(t, "x", objs.Harmful[0].ID)
	assert.Equal(t, "x done", objs.Harmful[0].SuccessPattern)
	require.Len(t, objs.Benign, 1)
	assert.Equal(t, "y", objs.Benign[0].ID)
}

func TestReadObjectivesFile_Errors(t *testing.T) {
	cases := map[string]struct{ name, content, want string }{
		"no objective":  {"o.jsonl", `{"id": "a"}`, "has no objective"},
		"bad benign":    {"o.jsonl", `{"objective": "a", "benign": "maybe"}`, "not true or false"},
		"bad json":      {"o.jsonl", `not json`, "line 1: not a JSON object"},
		"no rows":       {"o.jsonl", "\n", "no rows"},
		"empty csv":     {"o.csv", "", "no rows"},
		"bad csv":       {"o.csv", "objective\n\"unterminated\n", "o.csv"},
		"bad extension": {"o.txt", "objective", "unsupported extension"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ReadObjectivesFile(writeTemp(t, tc.name, tc.content))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
	_, err := ReadObjectivesFile(filepath.Join(t.TempDir(), "missing.jsonl"))
	require.Error(t, err)
}
