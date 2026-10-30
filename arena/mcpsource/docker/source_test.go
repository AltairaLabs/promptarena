package docker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	return f.execOut, f.execErr
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
	assert.Equal(t, health.URL, conn.URL)
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

	_, closer, err := s.Open(context.Background(), map[string]any{
		"image": "x",
		"env":   map[string]any{"FOO": "bar"},
		"mounts": []any{
			map[string]any{"source": "/host/skills", "target": "/skills/codegen", "readonly": true},
		},
	})
	require.NoError(t, err)
	defer func() { _ = closer.Close() }()

	require.Len(t, cli.runCalls, 1)
	spec := cli.runCalls[0]
	assert.Equal(t, "bar", spec.Env["FOO"])
	require.Len(t, spec.Mounts, 1)
	assert.Equal(t, "/host/skills", spec.Mounts[0].Source)
	assert.Equal(t, "/skills/codegen", spec.Mounts[0].Target)
	assert.True(t, spec.Mounts[0].ReadOnly)
}

func TestMountsFromArgs_TypedSlice(t *testing.T) {
	// buildSkillMounts in engine returns []map[string]any directly (not []any).
	mounts := mountsFromArgs([]map[string]any{
		{"source": "/a", "target": "/b", "readonly": true},
	})
	require.Len(t, mounts, 1)
	assert.Equal(t, "/a", mounts[0].Source)
	assert.True(t, mounts[0].ReadOnly)
}

func TestMountsFromArgs_Nil(t *testing.T) {
	assert.Nil(t, mountsFromArgs(nil))
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
