package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/AltairaLabs/promptarena/v2/arena/mcpsource"
)

const (
	defaultSandboxPort = 8080
	defaultHealthWait  = 20 * time.Second
	healthPollInterval = 200 * time.Millisecond
)

// Source is the Docker-backed MCPSource.
type Source struct {
	cli CLI
	// urlForContainer is overridable for tests; production returns
	// http://localhost:<hostPort>.
	urlForContainer func(containerID, hostPort string) string
	// healthTimeoutOverride shortens the wait in tests. Zero means
	// "use defaultHealthWait".
	healthTimeoutOverride time.Duration
}

// New returns a Docker source using the local docker CLI.
func New() *Source { return &Source{cli: NewCLI()} }

// Open validates args, starts the container, optionally clones a repo
// into /workspace, health-polls, and returns the MCP URL + closer.
func (s *Source) Open(ctx context.Context, args map[string]any) (mcpsource.MCPConn, io.Closer, error) {
	image, ok := args["image"].(string)
	if !ok || image == "" {
		return mcpsource.MCPConn{}, nil, errors.New("docker source: args.image is required")
	}

	hostPort, err := pickFreePort()
	if err != nil {
		return mcpsource.MCPConn{}, nil, fmt.Errorf("docker source: pick free port: %w", err)
	}

	spec := RunSpec{
		Image:    image,
		PortHost: hostPort,
		PortCtr:  defaultSandboxPort,
		Env:      stringMap(args["env"]),
		Mounts:   mountsFromArgs(args["mounts"]),
	}
	if argErr := applyRuntimeArgs(&spec, args); argErr != nil {
		return mcpsource.MCPConn{}, nil, argErr
	}

	cid, err := s.cli.Run(ctx, spec)
	if err != nil {
		return mcpsource.MCPConn{}, nil, fmt.Errorf("docker source: run: %w", err)
	}

	closer := closerFunc(func() error {
		return s.cli.Stop(context.Background(), cid)
	})

	url := s.resolveURL(cid, strconv.Itoa(hostPort))

	if repo, _ := args["repo"].(string); repo != "" {
		branch, _ := args["branch"].(string)
		if err := s.cloneRepo(ctx, cid, repo, branch); err != nil {
			_ = closer.Close()
			return mcpsource.MCPConn{}, nil, err
		}
	}

	timeout := s.healthTimeoutOverride
	if timeout == 0 {
		timeout = defaultHealthWait
	}
	if err := waitForHealth(ctx, url, timeout); err != nil {
		_ = closer.Close()
		return mcpsource.MCPConn{}, nil, fmt.Errorf("docker source: container not healthy: %w", err)
	}

	return mcpsource.MCPConn{URL: url, ContainerID: cid, Ports: s.portURLs(cid, spec.ExtraPorts)}, closer, nil
}

// portURLs maps each extra published container port to the host URL that
// reaches it. Nil when no extra ports were requested.
func (s *Source) portURLs(cid string, ports []PortMapping) map[string]string {
	if len(ports) == 0 {
		return nil
	}
	out := make(map[string]string, len(ports))
	for _, p := range ports {
		out[strconv.Itoa(p.Ctr)] = s.resolveURL(cid, strconv.Itoa(p.Host))
	}
	return out
}

// applyRuntimeArgs reads the optional ports / network / cpus / memory args
// into spec. Each extra port is published on a free host port.
func applyRuntimeArgs(spec *RunSpec, args map[string]any) error {
	ports, err := portsFromArgs(args["ports"])
	if err != nil {
		return err
	}
	for _, ctr := range ports {
		host, pickErr := pickFreePort()
		if pickErr != nil {
			return fmt.Errorf("docker source: pick free port for %d: %w", ctr, pickErr)
		}
		spec.ExtraPorts = append(spec.ExtraPorts, PortMapping{Host: host, Ctr: ctr})
	}
	if spec.Network, err = stringArg(args, "network"); err != nil {
		return err
	}
	// --network none/host drop published ports, so the MCP endpoint would be
	// unreachable. Egress control belongs on a user-defined bridge network.
	if spec.Network == "none" || spec.Network == "host" {
		return fmt.Errorf("docker source: args.network %q is not supported: the MCP port must be published; "+
			"use a user-defined bridge network (e.g. one routed through a filtering proxy)", spec.Network)
	}
	if spec.CPUs, err = stringArg(args, "cpus"); err != nil {
		return err
	}
	if spec.Memory, err = stringArg(args, "memory"); err != nil {
		return err
	}
	return nil
}

// portsFromArgs parses args.ports: a list of container ports. YAML yields
// ints; JSON yields float64s; numeric strings are accepted too.
func portsFromArgs(v any) ([]int, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("docker source: args.ports must be a list of container ports, got %T", v)
	}
	seen := make(map[int]bool, len(list))
	out := make([]int, 0, len(list))
	for _, item := range list {
		port, err := toPort(item)
		if err != nil {
			return nil, err
		}
		if port == defaultSandboxPort {
			return nil, fmt.Errorf("docker source: args.ports: %d is the sandbox's MCP port", port)
		}
		if seen[port] {
			return nil, fmt.Errorf("docker source: args.ports: %d listed twice", port)
		}
		seen[port] = true
		out = append(out, port)
	}
	return out, nil
}

func toPort(v any) (int, error) {
	var port int
	switch n := v.(type) {
	case int:
		port = n
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("docker source: args.ports: %v is not a whole number", n)
		}
		port = int(n)
	case string:
		p, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("docker source: args.ports: %q is not a port number", n)
		}
		port = p
	default:
		return 0, fmt.Errorf("docker source: args.ports: unsupported value %v (%T)", v, v)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("docker source: args.ports: %d is out of range 1-65535", port)
	}
	return port, nil
}

// stringArg reads an optional scalar arg as a string. Numbers are accepted
// (YAML turns `cpus: 2` into an int) and formatted without trailing zeros.
func stringArg(args map[string]any, key string) (string, error) {
	switch v := args[key].(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("docker source: args.%s must be a string or number, got %T", key, v)
	}
}

func (s *Source) resolveURL(cid, port string) string {
	if s.urlForContainer != nil {
		return s.urlForContainer(cid, port)
	}
	return "http://localhost:" + port
}

func (s *Source) cloneRepo(ctx context.Context, cid, repo, branch string) error {
	argv := []string{"git", "clone"}
	if branch != "" {
		argv = append(argv, "--branch", branch)
	}
	argv = append(argv, repo, "/workspace")
	if _, err := s.cli.Exec(ctx, cid, argv); err != nil {
		return fmt.Errorf("docker source: clone %s: %w", repo, err)
	}
	return nil
}

type closerFunc func() error

// Close invokes the underlying function; satisfies io.Closer.
func (f closerFunc) Close() error { return f() }

func stringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, raw := range m {
		if s, ok := raw.(string); ok {
			out[k] = s
		}
	}
	return out
}

func mountsFromArgs(v any) []Mount {
	list, ok := v.([]map[string]any)
	if !ok {
		// Accept the []any shape that YAML unmarshal produces for lists.
		generic, gok := v.([]any)
		if !gok {
			return nil
		}
		list = make([]map[string]any, 0, len(generic))
		for _, item := range generic {
			if m, ok := item.(map[string]any); ok {
				list = append(list, m)
			}
		}
	}
	var out []Mount
	for _, entry := range list {
		src, _ := entry["source"].(string)
		target, _ := entry["target"].(string)
		ro, _ := entry["readonly"].(bool)
		if src != "" && target != "" {
			out = append(out, Mount{Source: src, Target: target, ReadOnly: ro})
		}
	}
	return out
}

func pickFreePort() (int, error) {
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// waitForHealth polls baseURL+/sse until the server returns 2xx or the
// timeout elapses. The mcp-go SSE server (used by codegen-sandbox and other
// SSE-MCP images) does not expose a separate /health endpoint — /sse itself
// is the readiness signal: it returns 200 the moment the server can accept
// MCP clients, then begins streaming events. We close the body immediately
// once headers are received; the streaming body is irrelevant to the probe.
func waitForHealth(ctx context.Context, baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/sse", http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthPollInterval):
		}
	}
	return fmt.Errorf("health timeout after %v", timeout)
}
