package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `skill remove` takes the project-level copy first. With a user-level copy
// too, it used to print "Removed" and leave the skill resolvable without a word.
func TestRunSkillRemove_ReportsRemainingCopy(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", userHome)
	project := t.TempDir()
	t.Chdir(project)

	projectCopy := filepath.Join(project, ".promptkit", "skills", "org", "skill")
	userCopy := filepath.Join(userHome, "promptkit", "skills", "org", "skill")
	require.NoError(t, os.MkdirAll(projectCopy, 0o750))
	require.NoError(t, os.MkdirAll(userCopy, 0o750))

	var err error
	out := captureStdout(t, func() { err = runSkillRemove(nil, []string{"@org/skill"}) })
	require.NoError(t, err)
	assert.NoDirExists(t, projectCopy)
	assert.DirExists(t, userCopy)
	assert.Contains(t, out, "still installed at "+userCopy)

	out = captureStdout(t, func() { err = runSkillRemove(nil, []string{"@org/skill"}) })
	require.NoError(t, err)
	assert.NoDirExists(t, userCopy)
	assert.NotContains(t, out, "still installed")
}
