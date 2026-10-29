package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRunArgs_Basic(t *testing.T) {
	args := buildRunArgs(RunSpec{
		Image:    "ghcr.io/altairalabs/codegen-sandbox:latest",
		PortHost: 38081,
		PortCtr:  8080,
	})
	require.NotEmpty(t, args)
	assert.Equal(t, "run", args[0])
	assert.Contains(t, args, "-d")
	assert.Contains(t, args, "--rm")
	assert.Contains(t, args, "-p")
	assert.Contains(t, args, "38081:8080")
	assert.Equal(t, "ghcr.io/altairalabs/codegen-sandbox:latest", args[len(args)-1])
}

func TestBuildRunArgs_EnvAndMounts(t *testing.T) {
	args := buildRunArgs(RunSpec{
		Image:    "x",
		PortHost: 9000,
		PortCtr:  8080,
		Env:      map[string]string{"FOO": "bar"},
		Mounts: []Mount{
			{Source: "/a", Target: "/b", ReadOnly: true},
			{Source: "/c", Target: "/d", ReadOnly: false},
		},
	})
	assert.Contains(t, args, "-e")
	assert.Contains(t, args, "FOO=bar")
	assert.Contains(t, args, "-v")
	assert.Contains(t, args, "/a:/b:ro")
	assert.Contains(t, args, "/c:/d")
}

func TestBuildRunArgs_NoPortMapping(t *testing.T) {
	args := buildRunArgs(RunSpec{Image: "x"})
	for _, a := range args {
		assert.NotEqual(t, "-p", a, "port flag should be absent when port is 0")
	}
}

func TestBuildRunArgs_ExtraPortsNetworkAndLimits(t *testing.T) {
	args := buildRunArgs(RunSpec{
		Image:      "x",
		PortHost:   9000,
		PortCtr:    8080,
		ExtraPorts: []PortMapping{{Host: 54321, Ctr: 3000}},
		Network:    "arena-egress",
		CPUs:       "2",
		Memory:     "2g",
	})
	assert.Contains(t, args, "9000:8080")
	assert.Contains(t, args, "54321:3000")
	assert.Equal(t, "x", args[len(args)-1])
	assertFlagValue(t, args, "--network", "arena-egress")
	assertFlagValue(t, args, "--cpus", "2")
	assertFlagValue(t, args, "--memory", "2g")
}

func TestBuildRunArgs_OmitsUnsetRuntimeFlags(t *testing.T) {
	args := buildRunArgs(RunSpec{Image: "x"})
	for _, flag := range []string{"--network", "--cpus", "--memory"} {
		assert.NotContains(t, args, flag)
	}
}

func assertFlagValue(t *testing.T, args []string, flag, want string) {
	t.Helper()
	for i, a := range args {
		if a == flag {
			require.Less(t, i+1, len(args), "%s has no value", flag)
			assert.Equal(t, want, args[i+1])
			return
		}
	}
	t.Fatalf("flag %s not found in %v", flag, args)
}
