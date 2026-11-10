package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/mcpsource"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/mcp"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// writeSkill creates <root>/<name>/SKILL.md and returns the skill directory.
func writeMountSkill(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o600))
	return dir
}

// A `path:` skill has no name; it used to be skipped, so the sandbox got no
// /skills mount at all.
func TestBuildSkillMounts_PathSkillUsesDirectoryName(t *testing.T) {
	dir := writeMountSkill(t, t.TempDir(), "codegen")
	mounts := buildSkillMounts([]prompt.SkillSourceConfig{{Path: dir}})
	require.Len(t, mounts, 1)
	assert.Equal(t, dir, mounts[0]["source"])
	assert.Equal(t, "/skills/codegen", mounts[0]["target"])
}

func TestBuildSkillMounts_SkillsDirectoryMountsEachSkillOnce(t *testing.T) {
	root := t.TempDir()
	codegen := writeMountSkill(t, root, "codegen")
	review := writeMountSkill(t, root, "review")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "not-a-skill"), 0o755))

	// The codegen-sandbox shape: the directory, and one of its skills again.
	mounts := buildSkillMounts([]prompt.SkillSourceConfig{{Path: root}, {Path: codegen}})
	require.Len(t, mounts, 2)
	assert.Equal(t, codegen, mounts[0]["source"])
	assert.Equal(t, "/skills/codegen", mounts[0]["target"])
	assert.Equal(t, review, mounts[1]["source"])
	assert.Equal(t, "/skills/review", mounts[1]["target"])
}

func TestBuildSkillMounts_SkipsInlineAndMissingSkills(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	assert.Nil(t, buildSkillMounts([]prompt.SkillSourceConfig{{Name: "inline"}, {Path: missing}}))
}

// User mounts used to be replaced wholesale by the skill mounts.
func TestInjectMountsIntoArgs_KeepsUserMountsFirst(t *testing.T) {
	user := map[string]any{"source": "/host/data", "target": "/data"}
	entry := &config.MCPServerConfig{SourceArgs: map[string]any{"mounts": []any{user}}}
	skill := map[string]any{"source": "/host/codegen", "target": "/skills/codegen", "readonly": true}

	require.NoError(t, injectMountsIntoArgs(entry, []map[string]any{skill}))

	assert.Equal(t, []any{user, skill}, entry.SourceArgs["mounts"])
}

// A user mount and a skill mount on one target used to resolve silently in
// the user's favour, dropping the skill from the sandbox.
func TestInjectMountsIntoArgs_TargetCollisionIsAnError(t *testing.T) {
	clash := map[string]any{"source": "/host/mine", "target": "/skills/codegen"}
	entry := &config.MCPServerConfig{Name: "sandbox", SourceArgs: map[string]any{"mounts": []any{clash}}}
	skill := map[string]any{"source": "/host/codegen", "target": "/skills/codegen"}

	err := injectMountsIntoArgs(entry, []map[string]any{skill})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `target "/skills/codegen" is also a skill mount`)
}

// mountKit is a config directory holding fixtures/ and eval/.
func mountKit(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for _, d := range []string{"fixtures/add", "eval"} {
		require.NoError(t, os.MkdirAll(filepath.Join(base, d), 0o755))
	}
	return base
}

// A relative mount reaches the source resolved against the config directory,
// with scenario variables expanded and defaults filled in.
func TestMCPSourceScope_ResolvesMountsAgainstConfigDir(t *testing.T) {
	src := &recordingSource{}
	mcpsource.RegisterMCPSource("rec-mounts-resolve", src)
	base := mountKit(t)
	mgr := newMCPSourceScope(mcp.NewRegistry())
	mgr.baseDir = base

	cfg := config.MCPServerConfig{
		Name: "sb", Source: "rec-mounts-resolve", Scope: "session",
		SourceArgs: map[string]any{"mounts": []any{
			map[string]any{"source": "fixtures/{{scenario.case}}", "target": "/workspace", "mode": "copy"},
		}},
	}
	require.NoError(t, mgr.OpenAll(context.Background(), mcpsource.ScopeSession, "s1",
		map[string]string{"case": "add"}, nil, []config.MCPServerConfig{cfg}))

	assert.Equal(t, []any{map[string]any{
		"source": filepath.Join(base, "fixtures", "add"), "target": "/workspace",
		"readonly": true, "mode": "copy", "when": "start",
	}}, src.lastArgs["mounts"])
}

func TestMCPSourceScope_BadMountFailsOpen(t *testing.T) {
	src := &recordingSource{}
	mcpsource.RegisterMCPSource("rec-mounts-bad", src)
	mgr := newMCPSourceScope(mcp.NewRegistry())
	mgr.baseDir = mountKit(t)

	cfg := config.MCPServerConfig{
		Name: "sb", Source: "rec-mounts-bad", Scope: "session",
		SourceArgs: map[string]any{"mounts": []any{map[string]any{"source": "../outside", "target": "/x"}}},
	}
	err := mgr.OpenAll(context.Background(), mcpsource.ScopeSession, "s1", nil, nil, []config.MCPServerConfig{cfg})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `mcp server "sb"`)
	assert.Contains(t, err.Error(), "outside the config directory")
	assert.Zero(t, src.opens.Load(), "the source is never opened with a bad mount")
}

// deliveringSource returns a closer that counts DeliverEval calls.
type deliveringSource struct{ delivered atomic.Int32 }

type deliveringCloser struct{ src *deliveringSource }

func (c deliveringCloser) Close() error { return nil }
func (c deliveringCloser) DeliverEval(context.Context) error {
	c.src.delivered.Add(1)
	return nil
}

func (s *deliveringSource) Open(context.Context, map[string]any) (mcpsource.MCPConn, io.Closer, error) {
	return mcpsource.MCPConn{URL: "http://fake"}, deliveringCloser{s}, nil
}

func TestMCPSourceScope_DeliverEvalFilesReachesEachDeliverer(t *testing.T) {
	src := &deliveringSource{}
	mcpsource.RegisterMCPSource("rec-mounts-deliver", src)
	plain := &recordingSource{}
	mcpsource.RegisterMCPSource("rec-mounts-plain", plain)
	mgr := newMCPSourceScope(mcp.NewRegistry())

	entries := []config.MCPServerConfig{
		{Name: "a", Source: "rec-mounts-deliver", Scope: "session"},
		{Name: "b", Source: "rec-mounts-plain", Scope: "session"},
	}
	require.NoError(t, mgr.OpenAll(context.Background(), mcpsource.ScopeSession, "s1", nil, nil, entries))

	require.NoError(t, mgr.deliverEvalFiles(context.Background()))
	assert.Equal(t, int32(1), src.delivered.Load())
}

func TestDeliverEvalFiles_ContextWithoutDeliveryIsNoOp(t *testing.T) {
	require.NoError(t, deliverEvalFiles(context.Background()))

	calls := 0
	ctx := withEvalDelivery(context.Background(), func(context.Context) error { calls++; return nil })
	require.NoError(t, deliverEvalFiles(ctx))
	assert.Equal(t, 1, calls)
}
