package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/prompt"
)

func writeSkill(t *testing.T, dir, name, desc string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\nInstructions."
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

func writeSkillWithTools(t *testing.T, dir, name, desc string, tools []string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + name + "\ndescription: " + desc + "\nallowed-tools:\n"
	for _, tool := range tools {
		content += "  - " + tool + "\n"
	}
	content += "---\nInstructions."
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

func TestValidateSkillErrors_Directory(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		dir := t.TempDir()
		writeSkill(t, filepath.Join(dir, "skills", "my-skill"), "my-skill", "A skill")
		pack := &prompt.Pack{
			Skills:  []prompt.SkillSourceConfig{{Dir: "skills"}},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		assert.Empty(t, ValidateSkillErrors(pack, dir))
	})

	t.Run("missing directory errors", func(t *testing.T) {
		dir := t.TempDir()
		pack := &prompt.Pack{
			Skills:  []prompt.SkillSourceConfig{{Dir: "nope"}},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		errs := ValidateSkillErrors(pack, dir)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "does not exist")
	})

	t.Run("path traversal errors", func(t *testing.T) {
		dir := t.TempDir()
		pack := &prompt.Pack{
			Skills:  []prompt.SkillSourceConfig{{Dir: "../../etc"}},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		errs := ValidateSkillErrors(pack, dir)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "path traversal detected")
	})

	t.Run("duplicate directory names error", func(t *testing.T) {
		dir := t.TempDir()
		writeSkill(t, filepath.Join(dir, "a", "sk"), "dupe", "A")
		writeSkill(t, filepath.Join(dir, "b", "sk"), "dupe", "B")
		pack := &prompt.Pack{
			Skills:  []prompt.SkillSourceConfig{{Dir: "a"}, {Dir: "b"}},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		errs := ValidateSkillErrors(pack, dir)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "duplicate skill name")
	})

	t.Run("unparsable SKILL.md errors", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "skills", "bad")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		// Missing required frontmatter fields.
		require.NoError(t, os.WriteFile(
			filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: no name\n---\nbody"), 0o644))
		pack := &prompt.Pack{
			Skills:  []prompt.SkillSourceConfig{{Dir: "skills"}},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		errs := ValidateSkillErrors(pack, dir)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "skills[0]")
	})
}

func TestValidateSkillErrors_Inline(t *testing.T) {
	t.Run("complete inline skill", func(t *testing.T) {
		pack := &prompt.Pack{
			Skills: []prompt.SkillSourceConfig{
				{Name: "inline", Description: "desc", Instructions: "do it"},
			},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		assert.Empty(t, ValidateSkillErrors(pack, "/tmp"))
	})

	t.Run("incomplete inline skill", func(t *testing.T) {
		pack := &prompt.Pack{
			Skills:  []prompt.SkillSourceConfig{{Name: "incomplete"}},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		errs := ValidateSkillErrors(pack, "/tmp")
		require.Len(t, errs, 2)
		assert.Contains(t, errs[0], "missing required field: description")
		assert.Contains(t, errs[1], "missing required field: instructions")
	})

	t.Run("duplicate inline names error", func(t *testing.T) {
		pack := &prompt.Pack{
			Skills: []prompt.SkillSourceConfig{
				{Name: "shared", Description: "d1", Instructions: "i1"},
				{Name: "shared", Description: "d2", Instructions: "i2"},
			},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		errs := ValidateSkillErrors(pack, "/tmp")
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "duplicate skill name")
	})
}

func TestValidateSkillErrors_WorkflowState(t *testing.T) {
	t.Run("valid state skills dir", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "state-skills"), 0o755))
		pack := &prompt.Pack{
			Workflow: &prompt.WorkflowConfig{
				Version: 1, Entry: "start",
				States: map[string]*prompt.WorkflowState{
					"start": {PromptTask: "p", Skills: "state-skills"},
				},
			},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		assert.Empty(t, ValidateSkillErrors(pack, dir))
	})

	t.Run("skills none is skipped", func(t *testing.T) {
		pack := &prompt.Pack{
			Workflow: &prompt.WorkflowConfig{
				Version: 1, Entry: "start",
				States: map[string]*prompt.WorkflowState{
					"start": {PromptTask: "p", Skills: "none"},
				},
			},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		assert.Empty(t, ValidateSkillErrors(pack, "/tmp"))
	})

	t.Run("empty skills is skipped", func(t *testing.T) {
		pack := &prompt.Pack{
			Workflow: &prompt.WorkflowConfig{
				Version: 1, Entry: "start",
				States: map[string]*prompt.WorkflowState{
					"start": {PromptTask: "p", Skills: ""},
				},
			},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		assert.Empty(t, ValidateSkillErrors(pack, "/tmp"))
	})

	t.Run("glob pattern is skipped", func(t *testing.T) {
		pack := &prompt.Pack{
			Workflow: &prompt.WorkflowConfig{
				Version: 1, Entry: "start",
				States: map[string]*prompt.WorkflowState{
					"start": {PromptTask: "p", Skills: "skills/*"},
				},
			},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		assert.Empty(t, ValidateSkillErrors(pack, "/tmp"))
	})

	t.Run("missing state skills dir errors", func(t *testing.T) {
		dir := t.TempDir()
		pack := &prompt.Pack{
			Workflow: &prompt.WorkflowConfig{
				Version: 1, Entry: "start",
				States: map[string]*prompt.WorkflowState{
					"start": {PromptTask: "p", Skills: "gone"},
				},
			},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		errs := ValidateSkillErrors(pack, dir)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "does not exist")
	})

	t.Run("state skills traversal errors", func(t *testing.T) {
		dir := t.TempDir()
		pack := &prompt.Pack{
			Workflow: &prompt.WorkflowConfig{
				Version: 1, Entry: "start",
				States: map[string]*prompt.WorkflowState{
					"start": {PromptTask: "p", Skills: "../../etc"},
				},
			},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		errs := ValidateSkillErrors(pack, dir)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "path traversal detected")
	})
}

func TestValidateSkills_CrossRef(t *testing.T) {
	t.Run("missing tool warns", func(t *testing.T) {
		dir := t.TempDir()
		writeSkillWithTools(t, filepath.Join(dir, "skills", "sk"), "sk", "desc", []string{"known", "unknown"})
		pack := &prompt.Pack{
			Skills:  []prompt.SkillSourceConfig{{Dir: "skills"}},
			Tools:   map[string]*prompt.PackTool{"known": {Name: "known"}},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		warnings := ValidateSkills(pack, dir)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "unknown")
		assert.Contains(t, warnings[0], "not defined in pack tools")
	})

	t.Run("all tools known no warnings", func(t *testing.T) {
		dir := t.TempDir()
		writeSkillWithTools(t, filepath.Join(dir, "skills", "sk"), "sk", "desc", []string{"known"})
		pack := &prompt.Pack{
			Skills:  []prompt.SkillSourceConfig{{Dir: "skills"}},
			Tools:   map[string]*prompt.PackTool{"known": {Name: "known"}},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		assert.Empty(t, ValidateSkills(pack, dir))
	})

	t.Run("inline skills skipped by cross-ref", func(t *testing.T) {
		pack := &prompt.Pack{
			Skills: []prompt.SkillSourceConfig{
				{Name: "inline", Description: "d", Instructions: "i"},
			},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		assert.Empty(t, ValidateSkills(pack, "/tmp"))
	})

	t.Run("traversal dir skipped by cross-ref", func(t *testing.T) {
		dir := t.TempDir()
		pack := &prompt.Pack{
			Skills:  []prompt.SkillSourceConfig{{Dir: "../../etc"}},
			Prompts: map[string]*prompt.PackPrompt{"p": {ID: "p"}},
		}
		assert.Empty(t, ValidateSkills(pack, dir))
	})
}

func TestValidatePathContainment_Integration(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"relative ok", "skills", false},
		{"nested ok", "skills/sub", false},
		{"current dir ok", ".", false},
		{"escape errors", "../../etc", true},
		{"mid traversal errors", "skills/../../etc", true},
		{"absolute inside ok", filepath.Join(dir, "skills"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathContainment(tt.path, dir)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errPathTraversal)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
