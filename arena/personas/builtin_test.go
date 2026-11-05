package personas

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"
)

func TestList_EveryBuiltinLoads(t *testing.T) {
	entries, err := List()
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
		assert.NotEmpty(t, e.Description, e.Name)
	}
	assert.Equal(t, []string{"adversarial-scaffold", "crescendo", "direct", "multi-technique", "refine"}, names)
}

// Built-ins are referenced by ID from scenario turns, so the ID must be the
// name the CLI lists them under.
func TestBuiltins_IDMatchesName(t *testing.T) {
	entries, err := List()
	require.NoError(t, err)
	for _, e := range entries {
		p, err := Load(e.Name)
		require.NoError(t, err)
		assert.Equal(t, e.Name, p.ID)
	}
}

func TestBuiltins_AdversarialUseFeedbackAndNotes(t *testing.T) {
	entries, err := List()
	require.NoError(t, err)
	for _, e := range entries {
		p, err := Load(e.Name)
		require.NoError(t, err)
		if !p.IsAdversarial() {
			continue
		}
		assert.True(t, p.PrivateNotes, e.Name)
		for _, v := range []string{
			selfplay.VarObjective, selfplay.VarObjectiveFeedback, selfplay.VarPersonaNotes,
			selfplay.VarRewoundMessage, selfplay.VarRewindsLeft,
		} {
			assert.Contains(t, p.SystemTemplate, "{{"+v+"}}", "%s should use {{%s}}", e.Name, v)
		}
	}
}

func TestRead_Unknown(t *testing.T) {
	_, err := Read("does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "personas list")
}

func TestWrite_CopiesAndLoadsFromDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "personas")
	path, err := Write(dir, "crescendo", false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "crescendo.persona.yaml"), path)

	want, err := Read("crescendo")
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	p, err := arenaconfig.LoadPersona(path)
	require.NoError(t, err)
	assert.Equal(t, "crescendo", p.ID)
}

func TestWrite_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "refine.persona.yaml")
	require.NoError(t, os.WriteFile(path, []byte("mine"), 0o600))

	_, err := Write(dir, "refine", false)
	require.Error(t, err)
	got, _ := os.ReadFile(path)
	assert.Equal(t, "mine", string(got))

	_, err = Write(dir, "refine", true)
	require.NoError(t, err)
	got, _ = os.ReadFile(path)
	assert.NotEqual(t, "mine", string(got))
}

func TestWrite_UnknownWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "personas")
	_, err := Write(dir, "nope", false)
	require.Error(t, err)
	_, statErr := os.Stat(dir)
	assert.True(t, os.IsNotExist(statErr))
}
