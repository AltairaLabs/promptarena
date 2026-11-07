package deploy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// AdapterClient launches an adapter binary and communicates with it via
// JSON-RPC 2.0 over stdio. It implements the Provider interface so callers
// can use it interchangeably with an in-process provider.
type AdapterClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	// stderrTail keeps the end of the adapter's stderr so a crash can be
	// reported with what the adapter said on the way down. Nil when the
	// client is not backed by a subprocess.
	stderrTail *tailBuffer
	// stderrDone closes once the adapter's stderr has been fully copied (the
	// process has exited or closed it).
	stderrDone chan struct{}

	mu     sync.Mutex
	nextID int
	closed bool
}

// jsonRPC constants matching the adapter SDK.
const (
	jsonRPCVersion     = "2.0"
	methodProviderInfo = "get_provider_info"
	methodValidate     = "validate_config"
	methodPlan         = "plan"
	methodApply        = "apply"
	methodDestroy      = "destroy"
	methodStatus       = "status"
	methodImport       = "import"
	methodGetLoginURL  = "get_login_url"
	methodCompleteLgin = "complete_login"
	methodListSessions = "list_sessions"
	methodGetSession   = "get_session"
)

// rpcRequest is a JSON-RPC 2.0 request envelope.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int    `json:"id"`
}

// rpcResponse is a JSON-RPC 2.0 response envelope.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("adapter error %d: %s", e.Code, e.Message)
}

// codeMethodNotFound is the JSON-RPC 2.0 code for a method the peer does not
// implement. The adaptersdk returns it for any RPC an adapter does not serve —
// notably the optional login pair, which only some providers implement.
const codeMethodNotFound = -32601

// ErrMethodNotSupported reports that the adapter does not implement the method
// that was called.
//
// Optional RPCs are the reason this exists. There is no version handshake
// between the CLI and an adapter binary: an adapter built against an older
// PromptKit simply answers method-not-found, and the only way for a caller to
// tell that apart from a genuine failure was to match on the error text. Match
// with errors.Is instead:
//
//	if errors.Is(err, deploy.ErrMethodNotSupported) {
//		// degrade: this adapter does not offer the capability
//	}
//
// Callers that treat this as fatal are still correct — it remains an error, and
// the message is unchanged.
var ErrMethodNotSupported = errors.New("adapter does not implement this method")

// Unwrap exposes ErrMethodNotSupported behind a method-not-found response so
// callers can branch on the condition rather than on the message.
func (e *rpcError) Unwrap() error {
	if e.Code == codeMethodNotFound {
		return ErrMethodNotSupported
	}
	return nil
}

const (
	// maxScanSize is the maximum size of a single JSON-RPC response line (10 MB).
	maxScanSize = 10 * 1024 * 1024
	// initialBufSize is the initial buffer size for the scanner (64 KB).
	initialBufSize = 64 * 1024
)

// NewAdapterClient starts the adapter binary at the given path and returns
// a client ready for JSON-RPC calls. The process is kept alive for the
// lifetime of the client; call Close when done.
//
// Deprecated: Use [NewAdapterClientWithContext] to pass a context that
// controls the subprocess lifetime.
func NewAdapterClient(binaryPath string) (*AdapterClient, error) {
	return NewAdapterClientWithContext(context.Background(), binaryPath)
}

// NewAdapterClientWithContext starts the adapter binary at the given path,
// using ctx to control the subprocess lifetime. If ctx is canceled, the
// subprocess is killed. Call Close when done.
//
// The adapter's stderr is forwarded to the process's stderr, so adapter debug
// logs and panics are visible. Use [NewAdapterClientWithStderr] to send them
// elsewhere (the TUI routes them into its log panel).
func NewAdapterClientWithContext(ctx context.Context, binaryPath string) (*AdapterClient, error) {
	return NewAdapterClientWithStderr(ctx, binaryPath, os.Stderr)
}

// NewAdapterClientWithStderr is [NewAdapterClientWithContext] with the
// adapter's stderr written to stderr instead of the process's stderr. A nil
// stderr discards it. Either way the last few KiB are kept and appended to the
// error when the adapter exits mid-call.
func NewAdapterClientWithStderr(ctx context.Context, binaryPath string, stderr io.Writer) (*AdapterClient, error) {
	cmd := exec.CommandContext(ctx, binaryPath)
	return newAdapterClientWithStderr(cmd, stderr)
}

// newAdapterClient starts the given exec.Cmd and wires up stdio pipes, with
// the adapter's stderr kept only as a tail for error messages.
func newAdapterClient(cmd *exec.Cmd) (*AdapterClient, error) {
	return newAdapterClientWithStderr(cmd, nil)
}

// newAdapterClientWithStderr starts cmd with stdin/stdout as the JSON-RPC
// channel and stderr teed to stderr (when non-nil) and the crash-report tail.
func newAdapterClientWithStderr(cmd *exec.Cmd, stderr io.Writer) (*AdapterClient, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start adapter: %w", err)
	}

	// Copy stderr ourselves (rather than setting cmd.Stderr) so a call that
	// sees the adapter hang up can wait for everything it wrote first.
	tail := &tailBuffer{limit: stderrTailBytes}
	var dst io.Writer = tail
	if stderr != nil {
		dst = io.MultiWriter(stderr, tail)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(dst, errPipe)
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, initialBufSize), maxScanSize)

	return &AdapterClient{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     scanner,
		stderrTail: tail,
		stderrDone: done,
		nextID:     1,
	}, nil
}

// NewAdapterClientIO creates an AdapterClient backed by the given reader and
// writer instead of a subprocess. Useful for testing.
func NewAdapterClientIO(r io.Reader, w io.WriteCloser) *AdapterClient {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, initialBufSize), maxScanSize)
	return &AdapterClient{
		stdin:  w,
		stdout: scanner,
		nextID: 1,
	}
}

// Close shuts down the adapter process. It closes stdin (signaling EOF)
// and waits for the process to exit.
func (c *AdapterClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	_ = c.stdin.Close()

	// Wait closes the stderr pipe, so let the copier finish first; bounded in
	// case the adapter leaves a child process holding stderr open.
	if c.stderrDone != nil {
		select {
		case <-c.stderrDone:
		case <-time.After(stderrCloseTimeout):
		}
	}

	if c.cmd != nil {
		return c.cmd.Wait()
	}
	return nil
}

// stderrTailBytes bounds how much adapter stderr is kept for error messages.
const stderrTailBytes = 4 * 1024

// stderrDrainTimeout bounds the wait for an exiting adapter's stderr.
const stderrDrainTimeout = time.Second

// stderrCloseTimeout bounds how long Close waits for the adapter to exit and
// release stderr after stdin is closed.
const stderrCloseTimeout = 5 * time.Second

// tailBuffer is an io.Writer that keeps the last limit bytes written to it.
// exec's stderr-copying goroutine writes it while callers read it, so it is
// mutex-guarded.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.limit; over > 0 {
		t.buf = append([]byte(nil), t.buf[over:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// withAdapterStderr appends the tail of the adapter's stderr (usually a panic
// trace or a fatal log line) to err, for an adapter that hung up mid-call.
func (c *AdapterClient) withAdapterStderr(err error) error {
	if c.stderrDone != nil {
		// stdout closing usually means the process exited; give the stderr
		// copier a moment to drain what it wrote on the way out.
		select {
		case <-c.stderrDone:
		case <-time.After(stderrDrainTimeout):
		}
	}
	if c.stderrTail != nil {
		if tail := strings.TrimSpace(c.stderrTail.String()); tail != "" {
			return fmt.Errorf("%w; adapter stderr:\n%s", err, tail)
		}
	}
	return err
}

// callCtx sends a JSON-RPC request and reads the response with context support.
// The context controls the read timeout — if no response arrives before the
// context deadline, the call returns the context error.
func (c *AdapterClient) callCtx(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("adapter client is closed")
	}

	id := c.nextID
	c.nextID++

	req := rpcRequest{
		JSONRPC: jsonRPCVersion,
		Method:  method,
		Params:  params,
		ID:      id,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')

	if _, err := c.stdin.Write(data); err != nil {
		return c.withAdapterStderr(fmt.Errorf("write request: %w", err))
	}

	// Read response with context-based timeout.
	type scanResult struct {
		line []byte
		ok   bool
		err  error
	}
	ch := make(chan scanResult, 1)
	done := make(chan struct{})
	go func() {
		ok := c.stdout.Scan()
		ch <- scanResult{
			line: append([]byte(nil), c.stdout.Bytes()...),
			ok:   ok,
			err:  c.stdout.Err(),
		}
	}()

	var sr scanResult
	select {
	case <-ctx.Done():
		// Close stdin to unblock the scanner goroutine.
		_ = c.stdin.Close()
		close(done)
		return fmt.Errorf("read response: %w", ctx.Err())
	case sr = <-ch:
		close(done)
	}

	if !sr.ok {
		if sr.err != nil {
			return fmt.Errorf("read response: %w", sr.err)
		}
		return c.withAdapterStderr(errors.New("adapter closed connection unexpectedly"))
	}

	var resp rpcResponse
	if err := json.Unmarshal(sr.line, &resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return resp.Error
	}

	if result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}

	return nil
}

// GetProviderInfo returns metadata about the adapter.
func (c *AdapterClient) GetProviderInfo(ctx context.Context) (*ProviderInfo, error) {
	var info ProviderInfo
	if err := c.callCtx(ctx, methodProviderInfo, nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ValidateConfig validates provider-specific configuration.
func (c *AdapterClient) ValidateConfig(ctx context.Context, req *ValidateRequest) (*ValidateResponse, error) {
	var resp ValidateResponse
	if err := c.callCtx(ctx, methodValidate, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Plan generates a deployment plan showing what would change.
func (c *AdapterClient) Plan(ctx context.Context, req *PlanRequest) (*PlanResponse, error) {
	var resp PlanResponse
	if err := c.callCtx(ctx, methodPlan, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// applyResult matches the adapter SDK's response shape for Apply.
type applyResult struct {
	AdapterState string        `json:"adapter_state"`
	Events       []*ApplyEvent `json:"events,omitempty"`
}

type destroyResult struct {
	Status string          `json:"status"`
	Events []*DestroyEvent `json:"events,omitempty"`
}

// Apply executes the deployment. The adapter protocol returns all events in
// the final response rather than streaming them; this method replays those
// events to the callback (if non-nil) in order before returning. The returned
// string is the opaque adapter state.
func (c *AdapterClient) Apply(ctx context.Context, req *PlanRequest, callback ApplyCallback) (string, error) {
	var result applyResult
	if err := c.callCtx(ctx, methodApply, req, &result); err != nil {
		return "", err
	}
	// The adapter collects streamed events and returns them in the response;
	// replay them to the caller's callback in order.
	if callback != nil {
		for _, event := range result.Events {
			if err := callback(event); err != nil {
				return result.AdapterState, err
			}
		}
	}
	return result.AdapterState, nil
}

// Destroy tears down the deployment.
func (c *AdapterClient) Destroy(ctx context.Context, req *DestroyRequest, callback DestroyCallback) error {
	var result destroyResult
	if err := c.callCtx(ctx, methodDestroy, req, &result); err != nil {
		return err
	}
	// Replay the adapter's collected events to the caller's callback in order.
	if callback != nil {
		for _, event := range result.Events {
			if err := callback(event); err != nil {
				return err
			}
		}
	}
	return nil
}

// Status returns the current deployment status.
func (c *AdapterClient) Status(ctx context.Context, req *StatusRequest) (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.callCtx(ctx, methodStatus, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Import imports a pre-existing resource into the deployment state.
func (c *AdapterClient) Import(ctx context.Context, req *ImportRequest) (*ImportResponse, error) {
	var resp ImportResponse
	if err := c.callCtx(ctx, methodImport, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetLoginURL asks the adapter for the provider's browser authorize URL.
//
// An adapter that does not implement login answers method-not-found, which
// matches errors.Is(err, ErrMethodNotSupported). Prefer checking
// ProviderInfo.Capabilities for LoginCapability first — this is the backstop
// for an adapter that advertises the capability without serving the method.
func (c *AdapterClient) GetLoginURL(ctx context.Context, req *LoginURLRequest) (*LoginURLResponse, error) {
	var resp LoginURLResponse
	if err := c.callCtx(ctx, methodGetLoginURL, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CompleteLogin hands the adapter the captured callback params and returns the
// resolved deploy profile and scoped token.
func (c *AdapterClient) CompleteLogin(ctx context.Context, req *CompleteLoginRequest) (*CompleteLoginResponse, error) {
	var resp CompleteLoginResponse
	if err := c.callCtx(ctx, methodCompleteLgin, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListSessions asks the adapter for a page of recorded sessions.
//
// An adapter that does not implement session sourcing answers method-not-found,
// which matches errors.Is(err, ErrMethodNotSupported). Prefer checking
// ProviderInfo.Capabilities for SessionsCapability first.
func (c *AdapterClient) ListSessions(ctx context.Context, req *ListSessionsRequest) (*ListSessionsResponse, error) {
	var resp ListSessionsResponse
	if err := c.callCtx(ctx, methodListSessions, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetSession fetches one recorded session in full.
func (c *AdapterClient) GetSession(ctx context.Context, req *GetSessionRequest) (*GetSessionResponse, error) {
	var resp GetSessionResponse
	if err := c.callCtx(ctx, methodGetSession, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Verify AdapterClient implements Provider and the optional capabilities at
// compile time.
var (
	_ Provider              = (*AdapterClient)(nil)
	_ LoginProvider         = (*AdapterClient)(nil)
	_ SessionSourceProvider = (*AdapterClient)(nil)
)
