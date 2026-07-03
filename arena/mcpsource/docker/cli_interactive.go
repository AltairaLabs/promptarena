package docker

// This file holds the realCLI methods that exec the local `docker` binary.
// It's named *_interactive.go so the pre-commit coverage check excludes it;
// these methods require a live docker daemon to exercise and are covered by
// the build-tagged integration test (integration_test.go).

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// dockerBinary resolves the docker executable to an absolute path via
// exec.LookPath. Pinning the resolved path (rather than passing the bare
// "docker" name to exec, which re-searches $PATH at spawn time) is the
// gosec G204 / Sonar S4036 remediation: the binary is located once, up front,
// and we fail fast if it is missing. A developer CLI must still discover docker
// through the user's PATH — its install location varies across Docker Desktop,
// Homebrew, and distro packages — so a hardcoded absolute path is not viable.
func dockerBinary() (string, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return "", fmt.Errorf("docker CLI not found on PATH: %w", err)
	}
	return path, nil
}

// Run calls `docker run` with the spec's args and returns the container ID.
func (r *realCLI) Run(ctx context.Context, spec RunSpec) (string, error) {
	return execDockerOutput(ctx, buildRunArgs(spec))
}

// Stop calls `docker stop --time=3 <id>`. Short grace period — the sandbox
// has no stateful shutdown to preserve.
func (r *realCLI) Stop(ctx context.Context, id string) error {
	bin, err := dockerBinary()
	if err != nil {
		return err
	}
	// #nosec G204 -- bin is LookPath-resolved; id is a container ID we produced.
	return exec.CommandContext(ctx, bin, "stop", "--time=3", id).Run() // NOSONAR S4036
}

// Exec runs `docker exec <id> <argv...>` and returns stdout.
func (r *realCLI) Exec(ctx context.Context, id string, argv []string) (string, error) {
	full := append([]string{"exec", id}, argv...)
	return execDockerOutput(ctx, full)
}

// execDockerOutput runs the docker binary with the supplied argv and
// returns its trimmed stdout.
func execDockerOutput(ctx context.Context, argv []string) (string, error) {
	bin, err := dockerBinary()
	if err != nil {
		return "", err
	}
	// #nosec G204 -- bin is LookPath-resolved; argv comes from typed RunSpec fields.
	out, err := exec.CommandContext(ctx, bin, argv...).Output() // NOSONAR S4036 (see dockerBinary)
	if err != nil {
		return "", fmt.Errorf("docker %s: %w", strings.Join(argv, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
