package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePersonasList_Table(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writePersonasList(&buf, false))
	out := buf.String()
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "crescendo")
	assert.Contains(t, out, "adversarial")
}

func TestWritePersonasList_JSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writePersonasList(&buf, true))
	var entries []map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entries))
	require.NotEmpty(t, entries)
	assert.Equal(t, "adversarial-scaffold", entries[0]["name"])
}

func TestPersonasShow(t *testing.T) {
	var buf bytes.Buffer
	personasShowCmd.SetOut(&buf)
	t.Cleanup(func() { personasShowCmd.SetOut(nil) })

	require.NoError(t, personasShowCmd.RunE(personasShowCmd, []string{"crescendo"}))
	assert.Contains(t, buf.String(), "kind: Persona")
	require.Error(t, personasShowCmd.RunE(personasShowCmd, []string{"nope"}))
}

func TestPersonasAddCmd_UsesDirFlag(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p")
	personasDirFlag = dir
	t.Cleanup(func() { personasDirFlag = "personas"; personasAddCmd.SetOut(nil) })
	var buf bytes.Buffer
	personasAddCmd.SetOut(&buf)

	require.NoError(t, personasAddCmd.RunE(personasAddCmd, []string{"adversarial-scaffold"}))
	assert.FileExists(t, filepath.Join(dir, "adversarial-scaffold.persona.yaml"))
}

func TestAddPersona_PrintsConfigEntry(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "personas")
	var buf bytes.Buffer
	require.NoError(t, addPersona(&buf, "refine", dir, false))
	out := buf.String()
	assert.Contains(t, out, "- file: "+filepath.ToSlash(filepath.Join(dir, "refine.persona.yaml")))
	assert.Contains(t, out, "persona: refine")

	require.Error(t, addPersona(&buf, "refine", dir, false))
}
