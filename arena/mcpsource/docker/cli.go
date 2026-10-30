// Package docker implements an MCPSource that provisions MCP servers in
// Docker containers. Uses the local `docker` CLI (no Docker SDK dep).
package docker

import (
	"context"
	"fmt"
)

// RunSpec is the arguments to `docker run`.
type RunSpec struct {
	Image    string
	PortHost int
	PortCtr  int
	Env      map[string]string
	Mounts   []Mount
	// ExtraPorts are additional container ports published on host ports
	// (e.g. a web service the agent starts inside the sandbox).
	ExtraPorts []PortMapping
	Network    string // --network; empty means Docker's default
	CPUs       string // --cpus; empty means unlimited
	Memory     string // --memory; empty means unlimited
}

// PortMapping publishes container port Ctr on host port Host.
type PortMapping struct {
	Host int
	Ctr  int
}

// Mount describes one -v flag on a container.
type Mount struct {
	Source   string // host path
	Target   string // container path
	ReadOnly bool
}

// CLI is the minimal docker-CLI surface the source needs. Production code
// uses realCLI; tests supply a fake.
type CLI interface {
	Run(ctx context.Context, spec RunSpec) (containerID string, err error)
	Stop(ctx context.Context, containerID string) error
	Exec(ctx context.Context, containerID string, argv []string) (stdout string, err error)
}

// NewCLI returns a CLI backed by the local docker binary.
func NewCLI() CLI { return &realCLI{} }

// realCLI shells out to the docker binary. Its methods live in
// cli_interactive.go so they're not counted against unit-test coverage —
// they exec the real `docker` binary and can only be exercised by the
// integration test (see integration_test.go, build tag integration_docker).
type realCLI struct{}

// buildRunArgs converts a RunSpec into the argv for `docker run`. Factored
// out so unit tests can exercise arg construction without a running daemon.
func buildRunArgs(spec RunSpec) []string {
	args := []string{"run", "-d", "--rm"}
	if spec.PortHost > 0 && spec.PortCtr > 0 {
		args = append(args, "-p", fmt.Sprintf("%d:%d", spec.PortHost, spec.PortCtr))
	}
	for _, p := range spec.ExtraPorts {
		args = append(args, "-p", fmt.Sprintf("%d:%d", p.Host, p.Ctr))
	}
	if spec.Network != "" {
		args = append(args, "--network", spec.Network)
	}
	if spec.CPUs != "" {
		args = append(args, "--cpus", spec.CPUs)
	}
	if spec.Memory != "" {
		args = append(args, "--memory", spec.Memory)
	}
	for k, v := range spec.Env {
		args = append(args, "-e", k+"="+v)
	}
	for _, m := range spec.Mounts {
		v := m.Source + ":" + m.Target
		if m.ReadOnly {
			v += ":ro"
		}
		args = append(args, "-v", v)
	}
	args = append(args, spec.Image)
	return args
}
