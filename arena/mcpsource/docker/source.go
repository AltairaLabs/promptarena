package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AltairaLabs/promptarena/v2/arena/mcpsource"
)

const (
	defaultSandboxPort = 8080
	defaultHealthWait  = 20 * time.Second
	healthPollInterval = 200 * time.Millisecond
	// ssePath is the SSE endpoint of the sandbox's MCP server (mcp-go's SSE
	// server). It is both the readiness probe and the URL handed to the MCP
	// client, which connects to an SSE endpoint as written.
	ssePath = "/sse"
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
// into /workspace, health-polls, and returns the MCP SSE endpoint URL
// (http://localhost:<port>/sse) + closer.
func (s *Source) Open(ctx context.Context, args map[string]any) (mcpsource.MCPConn, io.Closer, error) {
	image, ok := args["image"].(string)
	if !ok || image == "" {
		return mcpsource.MCPConn{}, nil, errors.New("docker source: args.image is required")
	}

	hostPort, err := pickFreePort()
	if err != nil {
		return mcpsource.MCPConn{}, nil, fmt.Errorf("docker source: pick free port: %w", err)
	}

	env, err := envFromArgs(args["env"])
	if err != nil {
		return mcpsource.MCPConn{}, nil, err
	}
	mounts, err := mountsFromArgs(args["mounts"])
	if err != nil {
		return mcpsource.MCPConn{}, nil, err
	}
	spec := RunSpec{
		Image:    image,
		PortHost: hostPort,
		PortCtr:  defaultSandboxPort,
		Env:      env,
		Mounts:   mounts.binds,
	}
	if argErr := applyRuntimeArgs(&spec, args); argErr != nil {
		return mcpsource.MCPConn{}, nil, argErr
	}

	cid, err := s.cli.Run(ctx, spec)
	if err != nil {
		return mcpsource.MCPConn{}, nil, fmt.Errorf("docker source: run: %w", err)
	}

	closer := &containerCloser{source: s, cid: cid, atEval: mounts.atEval}

	url := s.resolveURL(cid, strconv.Itoa(hostPort))

	if err := s.copyIn(ctx, closer, mounts.atStart); err != nil {
		_ = closer.Close()
		return mcpsource.MCPConn{}, nil, err
	}

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

	conn := mcpsource.MCPConn{URL: url + ssePath, ContainerID: cid, Ports: s.portURLs(cid, spec.ExtraPorts)}
	if len(mounts.atEval) == 0 {
		// Only a closer with something to deliver advertises EvalDeliverer.
		return conn, closerFunc(closer.Close), nil
	}
	return conn, closer, nil
}

// sandboxMounts is source_args.mounts split by how each entry is delivered.
type sandboxMounts struct {
	binds   []Mount           // docker run -v
	atStart []mcpsource.Mount // copied in after docker run
	atEval  []mcpsource.Mount // copied in by DeliverEval
}

// containerCloser stops the container, and holds the eval-time copies until
// DeliverEval runs them.
type containerCloser struct {
	source    *Source
	cid       string
	owner     string // "uid:gid" of the container's user, looked up once
	atEval    []mcpsource.Mount
	delivered bool
}

// Close stops the container.
func (c *containerCloser) Close() error {
	return c.source.cli.Stop(context.Background(), c.cid)
}

// DeliverEval copies the when: eval mounts in. Later calls are no-ops.
func (c *containerCloser) DeliverEval(ctx context.Context) error {
	if c.delivered {
		return nil
	}
	c.delivered = true
	return c.source.copyIn(ctx, c, c.atEval)
}

// copyIn copies each mount into the container as root, then hands it to the
// container's own user: the sandbox image runs unprivileged, so a copy left
// owned by root would be read-only to the agent, and a target under / could
// not be created at all.
func (s *Source) copyIn(ctx context.Context, c *containerCloser, ms []mcpsource.Mount) error {
	if len(ms) == 0 {
		return nil
	}
	cp, ok := s.cli.(CopyCLI)
	if !ok {
		return fmt.Errorf("docker source: mode: copy mounts need a CLI that implements CopyCLI, got %T", s.cli)
	}
	if c.owner == "" {
		owner, err := s.containerOwner(ctx, c.cid)
		if err != nil {
			return err
		}
		c.owner = owner
	}
	for _, m := range ms {
		if err := copyOne(ctx, cp, c.cid, c.owner, m); err != nil {
			return fmt.Errorf("docker source: copy %s to %s: %w", m.Source, m.Target, err)
		}
	}
	return nil
}

func copyOne(ctx context.Context, cli CopyCLI, cid, owner string, m mcpsource.Mount) error {
	info, err := os.Stat(m.Source)
	if err != nil {
		return err
	}
	src, dir := m.Source, m.Target
	if info.IsDir() {
		// "<dir>/." copies the directory's contents into the target.
		src += string(filepath.Separator) + "."
	} else {
		dir = path.Dir(m.Target)
	}
	if _, mkErr := cli.ExecRoot(ctx, cid, []string{"mkdir", "-p", dir}); mkErr != nil {
		return mkErr
	}
	if cpErr := cli.Copy(ctx, cid, src, m.Target); cpErr != nil {
		return cpErr
	}
	_, err = cli.ExecRoot(ctx, cid, []string{"chown", "-R", owner, m.Target})
	return err
}

// containerOwner returns "uid:gid" of the user the container runs as.
func (s *Source) containerOwner(ctx context.Context, cid string) (string, error) {
	uid, err := s.cli.Exec(ctx, cid, []string{"id", "-u"})
	if err != nil {
		return "", fmt.Errorf("docker source: container user: %w", err)
	}
	gid, err := s.cli.Exec(ctx, cid, []string{"id", "-g"})
	if err != nil {
		return "", fmt.Errorf("docker source: container group: %w", err)
	}
	return strings.TrimSpace(uid) + ":" + strings.TrimSpace(gid), nil
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
	// Each pick closes its probe listener before the next, so the kernel may
	// hand the same ephemeral port back. Track what is taken so the MCP port
	// and every extra port stay distinct.
	used := map[int]bool{spec.PortHost: true}
	for _, ctr := range ports {
		host, pickErr := pickDistinctFreePort(pickFreePort, used)
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

// envFromArgs parses args.env: a map of variable name to value. Scalars are
// formatted as strings, since YAML turns `DEV_MODE: 1` into an int and
// `DEBUG: true` into a bool; a list or map value is an error rather than
// being dropped.
func envFromArgs(v any) (map[string]string, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("docker source: args.env must be a map of variable to value, got %T", v)
	}
	out := make(map[string]string, len(m))
	for k, raw := range m {
		switch x := raw.(type) {
		case string:
			out[k] = x
		case int:
			out[k] = strconv.Itoa(x)
		case float64:
			out[k] = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			out[k] = strconv.FormatBool(x)
		case nil:
			out[k] = ""
		default:
			return nil, fmt.Errorf("docker source: args.env.%s must be a string, number or boolean, got %T", k, raw)
		}
	}
	return out, nil
}

// mountsFromArgs parses args.mounts. Arena hands sources already resolved
// against the kit, so a relative source here is an error rather than being
// passed to `docker run -v`, which would read it as a volume name. Entries
// otherwise get the resolver's checks and defaults (read-only, bind, start).
func mountsFromArgs(v any) (sandboxMounts, error) {
	var out sandboxMounts
	if v == nil {
		return out, nil
	}
	if err := requireAbsoluteSources(v); err != nil {
		return out, err
	}
	ms, err := mcpsource.ResolveMounts(v, string(filepath.Separator), string(mcpsource.ScopeSession))
	if err != nil {
		return out, fmt.Errorf("docker source: args.%w", err)
	}
	for _, m := range ms {
		switch {
		case m.Mode == mcpsource.MountBind:
			out.binds = append(out.binds, Mount{Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly})
		case m.When == mcpsource.MountAtEval:
			out.atEval = append(out.atEval, m)
		default:
			out.atStart = append(out.atStart, m)
		}
	}
	return out, nil
}

func requireAbsoluteSources(v any) error {
	var entries []map[string]any
	switch x := v.(type) {
	case []map[string]any:
		entries = x
	case []any:
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				entries = append(entries, m)
			}
		}
	}
	for i, m := range entries {
		if src, _ := m["source"].(string); src != "" && !filepath.IsAbs(src) {
			return fmt.Errorf("docker source: args.mounts[%d]: source %q must be an absolute path", i, src)
		}
	}
	return nil
}

// maxPortPicks bounds how many times pickDistinctFreePort retries a collision.
const maxPortPicks = 16

// pickDistinctFreePort picks a free port not already in used, records it, and
// returns it.
func pickDistinctFreePort(pick func() (int, error), used map[int]bool) (int, error) {
	for range maxPortPicks {
		p, err := pick()
		if err != nil {
			return 0, err
		}
		if !used[p] {
			used[p] = true
			return p, nil
		}
	}
	return 0, errors.New("no distinct free port found")
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
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+ssePath, http.NoBody)
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
