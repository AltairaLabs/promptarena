package docker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/mcpsource"
)

type fakeCLI struct {
	runCalls  []RunSpec
	runID     string
	runErr    error
	stopCalls []string
	stopErr   error
	execCalls [][]string
	execOut   string
	execErr   error
	// calls records Exec, ExecRoot and Copy in order, e.g. "root:mkdir -p /x",
	// "cp:/host/x/.->/x", "exec:id -u", so a test can assert the sequence.
	calls   []string
	copyErr error
}

func (f *fakeCLI) Run(_ context.Context, spec RunSpec) (string, error) {
	f.runCalls = append(f.runCalls, spec)
	return f.runID, f.runErr
}
func (f *fakeCLI) Stop(_ context.Context, id string) error {
	f.stopCalls = append(f.stopCalls, id)
	return f.stopErr
}
func (f *fakeCLI) Exec(_ context.Context, _ string, argv []string) (string, error) {
	f.execCalls = append(f.execCalls, argv)
	f.calls = append(f.calls, "exec:"+strings.Join(argv, " "))
	switch strings.Join(argv, " ") {
	case "id -u":
		return "100", nil
	case "id -g":
		return "101", nil
	}
	return f.execOut, f.execErr
}
func (f *fakeCLI) ExecRoot(_ context.Context, _ string, argv []string) (string, error) {
	f.calls = append(f.calls, "root:"+strings.Join(argv, " "))
	return "", nil
}
func (f *fakeCLI) Copy(_ context.Context, _ string, hostPath, ctrPath string) error {
	f.calls = append(f.calls, "cp:"+hostPath+"->"+ctrPath)
	return f.copyErr
}

func TestSource_Open_RequiresImage(t *testing.T) {
	s := &Source{cli: &fakeCLI{}}
	_, _, err := s.Open(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image")
}

func TestSource_Open_StartsContainerAndReturnsURL(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer health.Close()

	cli := &fakeCLI{runID: "cid1"}
	s := &Source{cli: cli, urlForContainer: func(_, _ string) string { return health.URL }}

	conn, closer, err := s.Open(context.Background(), map[string]any{"image": "x"})
	require.NoError(t, err)
	// The URL is the SSE endpoint itself, so the MCP client connects to it as
	// written instead of retrying base+/sse with a warning.
	assert.Equal(t, health.URL+"/sse", conn.URL)
	require.NotNil(t, closer)
	require.Len(t, cli.runCalls, 1)
	assert.Equal(t, "x", cli.runCalls[0].Image)

	require.NoError(t, closer.Close())
	assert.Equal(t, []string{"cid1"}, cli.stopCalls)
}

func TestSource_Open_ProbesSSEPath(t *testing.T) {
	var probed []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probed = append(probed, r.URL.Path)
		if r.URL.Path == "/sse" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cli := &fakeCLI{runID: "cid-sse"}
	s := &Source{cli: cli, urlForContainer: func(_, _ string) string { return srv.URL }}

	_, closer, err := s.Open(context.Background(), map[string]any{"image": "x"})
	require.NoError(t, err)
	defer func() { _ = closer.Close() }()

	require.NotEmpty(t, probed, "expected at least one HTTP probe")
	assert.Contains(t, probed, "/sse", "source must probe /sse for SSE-MCP readiness")
}

func TestSource_Open_HealthTimeoutClosesContainer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cli := &fakeCLI{runID: "cid-timeout"}
	s := &Source{
		cli:                   cli,
		urlForContainer:       func(_, _ string) string { return srv.URL },
		healthTimeoutOverride: 50 * time.Millisecond,
	}

	_, _, err := s.Open(context.Background(), map[string]any{"image": "x"})
	require.Error(t, err)
	assert.Equal(t, []string{"cid-timeout"}, cli.stopCalls, "failed Open must stop the container")
}

func TestSource_Open_ClonesRepoWhenSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli := &fakeCLI{runID: "cid2"}
	s := &Source{cli: cli, urlForContainer: func(_, _ string) string { return srv.URL }}

	_, closer, err := s.Open(context.Background(), map[string]any{
		"image":  "x",
		"repo":   "https://github.com/foo/bar",
		"branch": "main",
	})
	require.NoError(t, err)
	defer func() { _ = closer.Close() }()

	var cloneCall []string
	for _, c := range cli.execCalls {
		for _, arg := range c {
			if arg == "clone" {
				cloneCall = c
			}
		}
	}
	require.NotEmpty(t, cloneCall, "expected a git clone exec")
	assert.Contains(t, cloneCall, "https://github.com/foo/bar")
	assert.Contains(t, cloneCall, "main")
	assert.Contains(t, cloneCall, "/workspace")
}

func TestSource_Open_ContainerStartFailureSurfaces(t *testing.T) {
	cli := &fakeCLI{runErr: errors.New("boom")}
	s := &Source{cli: cli, urlForContainer: func(_, _ string) string { return "http://x" }}

	_, _, err := s.Open(context.Background(), map[string]any{"image": "x"})
	require.Error(t, err)
	assert.Empty(t, cli.stopCalls, "no container to stop when Run failed")
}

func TestSource_Open_PropagatesMountsAndEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli := &fakeCLI{runID: "cid3"}
	s := &Source{cli: cli, urlForContainer: func(_, _ string) string { return srv.URL }}
	skills := realTempDir(t)

	_, closer, err := s.Open(context.Background(), map[string]any{
		"image": "x",
		"env":   map[string]any{"FOO": "bar"},
		"mounts": []any{
			map[string]any{"source": skills, "target": "/skills/codegen", "readonly": true},
		},
	})
	require.NoError(t, err)
	defer func() { _ = closer.Close() }()

	require.Len(t, cli.runCalls, 1)
	spec := cli.runCalls[0]
	assert.Equal(t, "bar", spec.Env["FOO"])
	require.Len(t, spec.Mounts, 1)
	assert.Equal(t, skills, spec.Mounts[0].Source)
	assert.Equal(t, "/skills/codegen", spec.Mounts[0].Target)
	assert.True(t, spec.Mounts[0].ReadOnly)
}

// YAML turns `DEV_MODE: 1` into an int; it used to be dropped silently.
func TestEnvFromArgs_FormatsScalars(t *testing.T) {
	env, err := envFromArgs(map[string]any{
		"S": "x", "I": 1, "F": 1.5, "B": true, "N": nil,
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"S": "x", "I": "1", "F": "1.5", "B": "true", "N": ""}, env)
}

func TestEnvFromArgs_RejectsNonScalars(t *testing.T) {
	_, err := envFromArgs(map[string]any{"L": []any{"a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "args.env.L")

	_, err = envFromArgs([]any{"A=1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "args.env must be a map")

	env, err := envFromArgs(nil)
	require.NoError(t, err)
	assert.Nil(t, env)
}

func TestSource_Open_BadEnvFailsBeforeStartingContainer(t *testing.T) {
	cli := &fakeCLI{runID: "cid-env"}
	s := &Source{cli: cli, urlForContainer: func(_, _ string) string { return "http://x" }}
	_, _, err := s.Open(context.Background(), map[string]any{"image": "x", "env": map[string]any{"M": map[string]any{}}})
	require.Error(t, err)
	assert.Empty(t, cli.runCalls)
}

// realTempDir is t.TempDir with symlinks resolved (macOS /var is a link),
// matching the path the mount resolver reports.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func TestMountsFromArgs_SplitsByModeAndWhen(t *testing.T) {
	dir := realTempDir(t)
	mounts, err := mountsFromArgs([]map[string]any{
		{"source": dir, "target": "/ro"},
		{"source": dir, "target": "/rw", "readonly": false},
		{"source": dir, "target": "/w", "mode": "copy"},
		{"source": dir, "target": "/eval", "mode": "copy", "when": "eval"},
	})
	require.NoError(t, err)
	assert.Equal(t, []Mount{{Source: dir, Target: "/ro", ReadOnly: true}, {Source: dir, Target: "/rw"}}, mounts.binds)
	require.Len(t, mounts.atStart, 1)
	assert.Equal(t, "/w", mounts.atStart[0].Target)
	require.Len(t, mounts.atEval, 1)
	assert.Equal(t, "/eval", mounts.atEval[0].Target)
}

// A relative source used to reach `docker run -v` as written, which Docker
// reads as the name of a new, empty volume.
func TestMountsFromArgs_RejectsRelativeSource(t *testing.T) {
	_, err := mountsFromArgs([]any{map[string]any{"source": "fixtures", "target": "/f"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an absolute path")

	none, err := mountsFromArgs(nil)
	require.NoError(t, err)
	assert.Equal(t, sandboxMounts{}, none)
}

func TestSource_Open_CopiesStartMountsBeforeHealthAndHoldsEvalMounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := realTempDir(t)
	file := filepath.Join(dir, "data.json")
	require.NoError(t, os.WriteFile(file, []byte("{}"), 0o600))

	cli := &fakeCLI{runID: "cid-copy"}
	s := &Source{cli: cli, urlForContainer: func(_, _ string) string { return srv.URL }}
	_, closer, err := s.Open(context.Background(), map[string]any{
		"image": "x",
		"mounts": []any{
			map[string]any{"source": dir, "target": "/workspace", "mode": "copy"},
			map[string]any{"source": file, "target": "/data/data.json", "mode": "copy"},
			map[string]any{"source": dir, "target": "/eval/scripts", "mode": "copy", "when": "eval"},
		},
	})
	require.NoError(t, err)
	defer func() { _ = closer.Close() }()

	assert.Empty(t, cli.runCalls[0].Mounts, "copies are not bind mounts")
	assert.Equal(t, []string{
		"exec:id -u", "exec:id -g",
		"root:mkdir -p /workspace", "cp:" + dir + "/.->/workspace", "root:chown -R 100:101 /workspace",
		"root:mkdir -p /data", "cp:" + file + "->/data/data.json", "root:chown -R 100:101 /data/data.json",
	}, cli.calls, "start copies land as the container's user; eval copies wait")

	deliverer, ok := closer.(mcpsource.EvalDeliverer)
	require.True(t, ok, "a source holding eval copies exposes DeliverEval")
	cli.calls = nil
	require.NoError(t, deliverer.DeliverEval(context.Background()))
	assert.Equal(t, []string{
		"root:mkdir -p /eval/scripts", "cp:" + dir + "/.->/eval/scripts", "root:chown -R 100:101 /eval/scripts",
	}, cli.calls)

	cli.calls = nil
	require.NoError(t, deliverer.DeliverEval(context.Background()))
	assert.Empty(t, cli.calls, "eval files are delivered once")
}

func TestSource_Open_FailedCopyStopsTheContainer(t *testing.T) {
	cli := &fakeCLI{runID: "cid-bad", copyErr: errors.New("no space")}
	s := &Source{cli: cli, urlForContainer: func(_, _ string) string { return "http://x" }}
	_, _, err := s.Open(context.Background(), map[string]any{
		"image":  "x",
		"mounts": []any{map[string]any{"source": t.TempDir(), "target": "/w", "mode": "copy"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copy")
	assert.Contains(t, err.Error(), "no space")
	assert.Equal(t, []string{"cid-bad"}, cli.stopCalls)
}

// The kernel can hand a just-released ephemeral port straight back, so a
// collision with an already-taken port must be retried, not returned.
func TestPickDistinctFreePort_SkipsTakenPorts(t *testing.T) {
	seq := []int{5000, 5000, 5001}
	pick := func() (int, error) {
		p := seq[0]
		seq = seq[1:]
		return p, nil
	}
	used := map[int]bool{5000: true}
	p, err := pickDistinctFreePort(pick, used)
	require.NoError(t, err)
	assert.Equal(t, 5001, p)
	assert.True(t, used[5001])
}

func TestPickDistinctFreePort_GivesUpAfterRepeatedCollisions(t *testing.T) {
	pick := func() (int, error) { return 5000, nil }
	_, err := pickDistinctFreePort(pick, map[int]bool{5000: true})
	require.Error(t, err)
}

func TestPickDistinctFreePort_PropagatesPickError(t *testing.T) {
	boom := errors.New("boom")
	pick := func() (int, error) { return 0, boom }
	_, err := pickDistinctFreePort(pick, map[int]bool{})
	require.ErrorIs(t, err, boom)
}

func sseHealthServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSource_Open_PublishesExtraPortsAndReportsURLs(t *testing.T) {
	health := sseHealthServer(t)
	cli := &fakeCLI{runID: "cid1"}
	s := &Source{cli: cli, urlForContainer: func(_, _ string) string { return health.URL }}

	conn, closer, err := s.Open(context.Background(), map[string]any{
		"image":   "x",
		"ports":   []any{3000, float64(9229), "5432"},
		"network": "arena-egress",
		"cpus":    2,
		"memory":  "2g",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = closer.Close() })

	require.Len(t, cli.runCalls, 1)
	spec := cli.runCalls[0]
	require.Len(t, spec.ExtraPorts, 3)
	ctrPorts := []int{}
	for _, p := range spec.ExtraPorts {
		assert.Positive(t, p.Host)
		assert.NotEqual(t, spec.PortHost, p.Host, "extra port must not reuse the MCP host port")
		ctrPorts = append(ctrPorts, p.Ctr)
	}
	assert.Equal(t, []int{3000, 9229, 5432}, ctrPorts)
	assert.Equal(t, "arena-egress", spec.Network)
	assert.Equal(t, "2", spec.CPUs)
	assert.Equal(t, "2g", spec.Memory)

	assert.Len(t, conn.Ports, 3)
	for _, key := range []string{"3000", "9229", "5432"} {
		assert.Contains(t, conn.Ports, key)
	}
}

func TestSource_Open_PortURLsUseHostPort(t *testing.T) {
	s := &Source{}
	urls := s.portURLs("cid", []PortMapping{{Host: 54321, Ctr: 3000}})
	assert.Equal(t, map[string]string{"3000": "http://localhost:54321"}, urls)
	assert.Nil(t, s.portURLs("cid", nil))
}

func TestSource_Open_NoExtraPortsLeavesConnPortsNil(t *testing.T) {
	health := sseHealthServer(t)
	s := &Source{cli: &fakeCLI{runID: "cid1"}, urlForContainer: func(_, _ string) string { return health.URL }}
	conn, closer, err := s.Open(context.Background(), map[string]any{"image": "x"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = closer.Close() })
	assert.Nil(t, conn.Ports)
}

func TestSource_Open_RejectsInvalidRuntimeArgs(t *testing.T) {
	cases := map[string]struct {
		args map[string]any
		want string
	}{
		"ports not a list":  {map[string]any{"ports": 3000}, "must be a list"},
		"mcp port":          {map[string]any{"ports": []any{8080}}, "MCP port"},
		"duplicate port":    {map[string]any{"ports": []any{3000, 3000}}, "listed twice"},
		"out of range":      {map[string]any{"ports": []any{70000}}, "out of range"},
		"zero port":         {map[string]any{"ports": []any{0}}, "out of range"},
		"fractional port":   {map[string]any{"ports": []any{3000.5}}, "whole number"},
		"non-numeric port":  {map[string]any{"ports": []any{"http"}}, "not a port number"},
		"unsupported type":  {map[string]any{"ports": []any{true}}, "unsupported value"},
		"network none":      {map[string]any{"network": "none"}, "not supported"},
		"network host":      {map[string]any{"network": "host"}, "not supported"},
		"network not text":  {map[string]any{"network": []any{"a"}}, "args.network"},
		"memory not scalar": {map[string]any{"memory": map[string]any{}}, "args.memory"},
		"cpus not scalar":   {map[string]any{"cpus": true}, "args.cpus"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cli := &fakeCLI{runID: "cid1"}
			s := &Source{cli: cli}
			tc.args["image"] = "x"
			_, _, err := s.Open(context.Background(), tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, cli.runCalls, "container must not start on invalid args")
		})
	}
}

func TestStringArg_FormatsNumbers(t *testing.T) {
	got, err := stringArg(map[string]any{"cpus": 1.5}, "cpus")
	require.NoError(t, err)
	assert.Equal(t, "1.5", got)
	got, err = stringArg(map[string]any{}, "cpus")
	require.NoError(t, err)
	assert.Empty(t, got)
}
