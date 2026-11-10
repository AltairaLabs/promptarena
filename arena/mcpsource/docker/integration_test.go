//go:build integration_docker

package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/mcpsource"
)

// Run with:
//
//	docker pull ghcr.io/altairalabs/codegen-sandbox:latest
//	go -C tools/arena test ./mcpsource/docker/... -tags=integration_docker -v -count=1
func TestSource_AgainstRealDocker(t *testing.T) {
	image := os.Getenv("PROMPTKIT_SANDBOX_IMAGE")
	if image == "" {
		image = "ghcr.io/altairalabs/codegen-sandbox:latest"
	}

	s := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, closer, err := s.Open(ctx, map[string]any{
		"image": image,
		"env":   map[string]any{"DEV_MODE": "1"},
	})
	require.NoError(t, err)
	defer func() { _ = closer.Close() }()

	assert.Contains(t, conn.URL, "http://localhost:")
	assert.True(t, strings.HasSuffix(conn.URL, "/sse"), "URL should be the SSE endpoint, got %s", conn.URL)
}

// Each mount kind against the real sandbox image, which runs as an
// unprivileged user: a bind is read-only, a start copy is writable by that
// user without touching the host, and an eval copy appears only on delivery.
func TestSource_MountsAgainstRealDocker(t *testing.T) {
	image := os.Getenv("PROMPTKIT_SANDBOX_IMAGE")
	if image == "" {
		image = "ghcr.io/altairalabs/codegen-sandbox:latest"
	}
	kit, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for _, d := range []string{"ref", "seed", "eval"} {
		require.NoError(t, os.MkdirAll(filepath.Join(kit, d), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(kit, d, "f.txt"), []byte(d), 0o644))
	}

	s := New()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, closer, err := s.Open(ctx, map[string]any{
		"image": image,
		"env":   map[string]any{"DEV_MODE": "1"},
		"mounts": []any{
			map[string]any{"source": filepath.Join(kit, "ref"), "target": "/ref"},
			map[string]any{"source": filepath.Join(kit, "seed"), "target": "/workspace/seed", "mode": "copy"},
			map[string]any{"source": filepath.Join(kit, "eval"), "target": "/eval/scripts", "mode": "copy", "when": "eval"},
		},
	})
	require.NoError(t, err)
	defer func() { _ = closer.Close() }()

	cli := NewCLI()
	sh := func(cmd string) (string, error) {
		return cli.Exec(ctx, conn.ContainerID, []string{"sh", "-c", cmd})
	}

	got, err := sh("cat /ref/f.txt")
	require.NoError(t, err)
	assert.Equal(t, "ref", got)
	_, err = sh("echo x > /ref/f.txt")
	assert.Error(t, err, "a bind mount is read-only by default")

	_, err = sh("echo changed > /workspace/seed/f.txt")
	require.NoError(t, err, "a copy is writable by the container's user")
	host, err := os.ReadFile(filepath.Join(kit, "seed", "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "seed", string(host), "writes to a copy never reach the host")

	_, err = sh("test -e /eval/scripts/f.txt")
	assert.Error(t, err, "eval files are absent while the agent works")
	require.NoError(t, closer.(mcpsource.EvalDeliverer).DeliverEval(ctx))
	got, err = sh("cat /eval/scripts/f.txt")
	require.NoError(t, err)
	assert.Equal(t, "eval", got)
}
