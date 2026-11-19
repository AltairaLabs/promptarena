package adaptersdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/deploy"
)

// fakeProvider is a minimal deploy.Provider for testing.
type fakeProvider struct {
	info      *deploy.ProviderInfo
	planResp  *deploy.PlanResponse
	status    *deploy.StatusResponse
	applyErr  error
	importErr error

	// partialState and partialEvents are what Apply reports before failing
	// with applyErr, as an adapter does when it fails partway.
	partialState  string
	partialEvents []*deploy.ApplyEvent
}

func (f *fakeProvider) GetProviderInfo(_ context.Context) (*deploy.ProviderInfo, error) {
	return f.info, nil
}

func (f *fakeProvider) ValidateConfig(
	_ context.Context, req *deploy.ValidateRequest,
) (*deploy.ValidateResponse, error) {
	if req.Config == "" {
		return &deploy.ValidateResponse{Valid: false, Errors: []string{"empty config"}}, nil
	}
	return &deploy.ValidateResponse{Valid: true}, nil
}

func (f *fakeProvider) Plan(
	_ context.Context, _ *deploy.PlanRequest,
) (*deploy.PlanResponse, error) {
	return f.planResp, nil
}

func (f *fakeProvider) Apply(
	_ context.Context, _ *deploy.PlanRequest, cb deploy.ApplyCallback,
) (string, error) {
	if f.applyErr != nil {
		for _, e := range f.partialEvents {
			_ = cb(e)
		}
		return f.partialState, f.applyErr
	}
	_ = cb(&deploy.ApplyEvent{Type: "progress", Message: "deploying"})
	return "state-123", nil
}

func (f *fakeProvider) Destroy(
	_ context.Context, _ *deploy.DestroyRequest, cb deploy.DestroyCallback,
) error {
	_ = cb(&deploy.DestroyEvent{Type: "progress", Message: "destroying"})
	return nil
}

func (f *fakeProvider) Status(
	_ context.Context, _ *deploy.StatusRequest,
) (*deploy.StatusResponse, error) {
	return f.status, nil
}

func (f *fakeProvider) Import(
	_ context.Context, req *deploy.ImportRequest,
) (*deploy.ImportResponse, error) {
	if f.importErr != nil {
		return nil, f.importErr
	}
	return &deploy.ImportResponse{
		Resource: deploy.ResourceStatus{
			Type:   req.ResourceType,
			Name:   req.ResourceName,
			Status: "healthy",
		},
		State: "imported-state",
	}, nil
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		info: &deploy.ProviderInfo{
			Name:    "test-adapter",
			Version: "1.0.0",
		},
		planResp: &deploy.PlanResponse{
			Summary: "1 resource to create",
			Changes: []deploy.ResourceChange{
				{Type: "agent_runtime", Name: "main", Action: deploy.ActionCreate},
			},
		},
		status: &deploy.StatusResponse{
			Status: "deployed",
		},
	}
}

func makeRequest(method string, params any, id int) string {
	p, _ := json.Marshal(params)
	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  json.RawMessage(p),
		"id":      id,
	}
	b, _ := json.Marshal(req)
	return string(b)
}

func TestServeIO_GetProviderInfo(t *testing.T) {
	provider := newFakeProvider()
	input := makeRequest("get_provider_info", nil, 1) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", resp.JSONRPC)
	}
	result, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(result), "test-adapter") {
		t.Errorf("expected provider name in result, got %s", string(result))
	}
}

func TestServeIO_ValidateConfig(t *testing.T) {
	provider := newFakeProvider()
	input := makeRequest("validate_config", deploy.ValidateRequest{Config: "ok"}, 2) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestServeIO_Plan(t *testing.T) {
	provider := newFakeProvider()
	params := deploy.PlanRequest{PackJSON: "{}", DeployConfig: "{}"}
	input := makeRequest("plan", params, 3) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(result), "agent_runtime") {
		t.Errorf("expected resource change in result, got %s", string(result))
	}
}

func TestServeIO_Plan_Warnings(t *testing.T) {
	provider := newFakeProvider()
	provider.planResp.Warnings = []string{"no binding named default"}
	params := deploy.PlanRequest{PackJSON: "{}", DeployConfig: "{}"}
	input := makeRequest("plan", params, 7) + "\n"
	var out bytes.Buffer

	if err := ServeIO(provider, strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// Round-trip the result back into a typed PlanResponse — the same path the
	// AdapterClient takes — and confirm the warning survived the protocol.
	raw, _ := json.Marshal(resp.Result)
	var plan deploy.PlanResponse
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	if len(plan.Warnings) != 1 || plan.Warnings[0] != "no binding named default" {
		t.Errorf("warnings did not pass through: %+v", plan.Warnings)
	}
}

// fakeLoginProvider is a fakeProvider that also implements deploy.LoginProvider.
type fakeLoginProvider struct {
	*fakeProvider
}

func (f *fakeLoginProvider) GetLoginURL(
	_ context.Context, req *deploy.LoginURLRequest,
) (*deploy.LoginURLResponse, error) {
	return &deploy.LoginURLResponse{
		AuthorizeURL: "https://omnia.example.com/cli/authorize?callback=" +
			req.CallbackURL + "&state=" + req.State,
	}, nil
}

func (f *fakeLoginProvider) CompleteLogin(
	_ context.Context, req *deploy.CompleteLoginRequest,
) (*deploy.CompleteLoginResponse, error) {
	return &deploy.CompleteLoginResponse{
		Profile: map[string]interface{}{"workspace": req.Params["code"]},
		Token:   "omnia_sk_minted",
	}, nil
}

func TestServeIO_Login_Supported(t *testing.T) {
	provider := &fakeLoginProvider{fakeProvider: newFakeProvider()}

	urlIn := makeRequest("get_login_url",
		deploy.LoginURLRequest{CallbackURL: "http://127.0.0.1:5000/cb", State: "xyz"}, 8) + "\n"
	var urlOut bytes.Buffer
	if err := ServeIO(provider, strings.NewReader(urlIn), &urlOut); err != nil {
		t.Fatalf("ServeIO get_login_url: %v", err)
	}
	if !strings.Contains(urlOut.String(), "authorize_url") || !strings.Contains(urlOut.String(), "state=xyz") {
		t.Errorf("expected authorize_url with state, got %s", urlOut.String())
	}

	doneIn := makeRequest("complete_login",
		deploy.CompleteLoginRequest{Params: map[string]string{"code": "demo"}}, 9) + "\n"
	var doneOut bytes.Buffer
	if err := ServeIO(provider, strings.NewReader(doneIn), &doneOut); err != nil {
		t.Fatalf("ServeIO complete_login: %v", err)
	}
	if !strings.Contains(doneOut.String(), "omnia_sk_minted") || !strings.Contains(doneOut.String(), "demo") {
		t.Errorf("expected token + profile, got %s", doneOut.String())
	}
}

func TestServeIO_Login_Unsupported(t *testing.T) {
	// The plain fakeProvider does not implement deploy.LoginProvider.
	provider := newFakeProvider()
	in := makeRequest("get_login_url", deploy.LoginURLRequest{}, 10) + "\n"
	var out bytes.Buffer
	if err := ServeIO(provider, strings.NewReader(in), &out); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}
	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != CodeMethodNotFound {
		t.Errorf("expected method-not-found for login-less provider, got %+v", resp.Error)
	}
}

func TestServeIO_Apply(t *testing.T) {
	provider := newFakeProvider()
	params := deploy.PlanRequest{PackJSON: "{}", DeployConfig: "{}"}
	input := makeRequest("apply", params, 4) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(result), "state-123") {
		t.Errorf("expected adapter_state in result, got %s", string(result))
	}
	// Apply must return the collected events so the client can replay them.
	if !strings.Contains(string(result), "deploying") {
		t.Errorf("expected streamed apply events in result, got %s", string(result))
	}
}

func TestServeIO_ApplyError(t *testing.T) {
	provider := newFakeProvider()
	provider.applyErr = fmt.Errorf("deploy failed")
	params := deploy.PlanRequest{PackJSON: "{}", DeployConfig: "{}"}
	input := makeRequest("apply", params, 5) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != CodeInternalError {
		t.Errorf("expected code %d, got %d", CodeInternalError, resp.Error.Code)
	}
	// An apply that failed before changing anything reports no partial result.
	if strings.Contains(out.String(), `"data"`) {
		t.Errorf("a failure with no state or events must carry no error data, got %s", out.String())
	}
}

// TestServeIO_ApplyPartialFailureKeepsStateAndEvents pins the fix for a failed
// apply losing what it had created: the state and events the adapter reported
// before the error travel in the JSON-RPC error's data, where the client can
// persist them. Dropping them made the next apply create the same resources
// again and fail with "already exists".
func TestServeIO_ApplyPartialFailureKeepsStateAndEvents(t *testing.T) {
	provider := newFakeProvider()
	provider.applyErr = fmt.Errorf("create endpoint: quota exceeded")
	provider.partialState = `{"agent":"created"}`
	provider.partialEvents = []*deploy.ApplyEvent{{
		Type: "resource",
		Resource: &deploy.ResourceResult{
			Type: "agent", Name: "a1", Action: deploy.ActionCreate, Status: "created",
		},
	}}
	input := makeRequest("apply", deploy.PlanRequest{PackJSON: "{}", DeployConfig: "{}"}, 5) + "\n"
	var out bytes.Buffer

	if err := ServeIO(provider, strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    *struct {
				AdapterState string               `json:"adapter_state"`
				Events       []*deploy.ApplyEvent `json:"events"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != CodeInternalError {
		t.Fatalf("expected an internal error response, got %s", out.String())
	}
	if !strings.Contains(resp.Error.Message, "quota exceeded") {
		t.Errorf("the adapter's error must still be reported, got %q", resp.Error.Message)
	}
	if resp.Error.Data == nil {
		t.Fatalf("expected the partial result in error.data, got %s", out.String())
	}
	if resp.Error.Data.AdapterState != `{"agent":"created"}` {
		t.Errorf("adapter_state = %q, want the state returned with the error", resp.Error.Data.AdapterState)
	}
	if len(resp.Error.Data.Events) != 1 || resp.Error.Data.Events[0].Resource.Name != "a1" {
		t.Errorf("events = %+v, want the resource event reported before the failure", resp.Error.Data.Events)
	}
}

// TestApplyPartialFailure_RoundTripsThroughAdapterClient runs the SDK server
// against the real deploy.AdapterClient, so the two halves of the protocol
// are held to the same shape: a failed apply hands the client the adapter's
// partial state and replays its events, together with the error.
func TestApplyPartialFailure_RoundTripsThroughAdapterClient(t *testing.T) {
	provider := newFakeProvider()
	provider.applyErr = fmt.Errorf("create endpoint: quota exceeded")
	provider.partialState = "partial-state"
	provider.partialEvents = []*deploy.ApplyEvent{{Type: "progress", Message: "agent created"}}

	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()
	go func() {
		_ = ServeIO(provider, serverIn, serverOut)
		_ = serverOut.Close()
	}()
	client := deploy.NewAdapterClientIO(clientIn, clientOut)
	t.Cleanup(func() { _ = client.Close() })

	var seen []string
	state, err := client.Apply(context.Background(), &deploy.PlanRequest{}, func(e *deploy.ApplyEvent) error {
		seen = append(seen, e.Message)
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("expected the apply error, got %v", err)
	}
	if state != "partial-state" {
		t.Errorf("state = %q, want the partial state returned with the error", state)
	}
	if len(seen) != 1 || seen[0] != "agent created" {
		t.Errorf("events replayed = %v, want the event reported before the failure", seen)
	}
}

func TestServeIO_Destroy(t *testing.T) {
	provider := newFakeProvider()
	params := deploy.DestroyRequest{DeployConfig: "{}"}
	input := makeRequest("destroy", params, 6) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	// Destroy must return the collected events so the client can replay them.
	result, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(result), "destroying") {
		t.Errorf("expected streamed destroy events in result, got %s", string(result))
	}
}

func TestServeIO_Status(t *testing.T) {
	provider := newFakeProvider()
	params := deploy.StatusRequest{DeployConfig: "{}"}
	input := makeRequest("status", params, 7) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(result), "deployed") {
		t.Errorf("expected deployed status, got %s", string(result))
	}
}

func TestServeIO_MethodNotFound(t *testing.T) {
	provider := newFakeProvider()
	input := makeRequest("unknown_method", nil, 8) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != CodeMethodNotFound {
		t.Errorf("expected code %d, got %d", CodeMethodNotFound, resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "unknown_method") {
		t.Errorf("expected method name in error, got %s", resp.Error.Message)
	}
}

func TestServeIO_ParseError(t *testing.T) {
	provider := newFakeProvider()
	input := "not valid json\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != CodeParseError {
		t.Errorf("expected code %d, got %d", CodeParseError, resp.Error.Code)
	}
}

func TestServeIO_InvalidParams(t *testing.T) {
	provider := newFakeProvider()
	// Send plan with invalid params (string instead of object)
	req := `{"jsonrpc":"2.0","method":"plan","params":"bad","id":9}` + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(req), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error response for invalid params")
	}
	if resp.Error.Code != CodeParseError {
		t.Errorf("expected code %d, got %d", CodeParseError, resp.Error.Code)
	}
}

// invalidParamsRequest is a helper that yields a JSON-RPC request whose
// `params` is a bare string — deliberately malformed so dispatch's
// json.Unmarshal on the handler-specific params struct fails.
func invalidParamsRequest(method string, id int) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","method":%q,"params":"bad","id":%d}`+"\n", method, id)
}

func expectInvalidParamsResponse(t *testing.T, out *bytes.Buffer) {
	t.Helper()
	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error response for invalid params")
	}
	if resp.Error.Code != CodeParseError {
		t.Errorf("expected code %d, got %d", CodeParseError, resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "invalid params:") {
		t.Errorf("expected message to contain %q, got %q", "invalid params:", resp.Error.Message)
	}
}

func TestServeIO_ValidateConfig_InvalidParams(t *testing.T) {
	var out bytes.Buffer
	if err := ServeIO(newFakeProvider(), strings.NewReader(invalidParamsRequest("validate_config", 11)), &out); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}
	expectInvalidParamsResponse(t, &out)
}

func TestServeIO_Apply_InvalidParams(t *testing.T) {
	var out bytes.Buffer
	if err := ServeIO(newFakeProvider(), strings.NewReader(invalidParamsRequest("apply", 12)), &out); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}
	expectInvalidParamsResponse(t, &out)
}

func TestServeIO_Destroy_InvalidParams(t *testing.T) {
	var out bytes.Buffer
	if err := ServeIO(newFakeProvider(), strings.NewReader(invalidParamsRequest("destroy", 13)), &out); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}
	expectInvalidParamsResponse(t, &out)
}

func TestServeIO_Status_InvalidParams(t *testing.T) {
	var out bytes.Buffer
	if err := ServeIO(newFakeProvider(), strings.NewReader(invalidParamsRequest("status", 14)), &out); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}
	expectInvalidParamsResponse(t, &out)
}

func TestServeIO_MultipleRequests(t *testing.T) {
	provider := newFakeProvider()
	input := makeRequest("get_provider_info", nil, 1) + "\n" +
		makeRequest("status", deploy.StatusRequest{DeployConfig: "{}"}, 2) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 response lines, got %d", len(lines))
	}

	for i, line := range lines {
		var resp response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("line %d: unmarshal error: %v", i, err)
		}
		if resp.Error != nil {
			t.Errorf("line %d: unexpected error: %+v", i, resp.Error)
		}
	}
}

func TestServeIO_EmptyLines(t *testing.T) {
	provider := newFakeProvider()
	input := "\n\n" + makeRequest("get_provider_info", nil, 1) + "\n\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}
}

func TestServeIO_NilReaderWriter(t *testing.T) {
	provider := newFakeProvider()
	err := ServeIO(provider, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil reader/writer")
	}
}

func TestServeIO_Import(t *testing.T) {
	provider := newFakeProvider()
	params := deploy.ImportRequest{
		ResourceType: "agent_runtime",
		ResourceName: "my-agent",
		Identifier:   "container-abc",
		DeployConfig: "{}",
	}
	input := makeRequest("import", params, 10) + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(result), "imported-state") {
		t.Errorf("expected imported-state in result, got %s", string(result))
	}
	if !strings.Contains(string(result), "my-agent") {
		t.Errorf("expected resource name in result, got %s", string(result))
	}
}

func TestServeIO_Import_InvalidParams(t *testing.T) {
	provider := newFakeProvider()
	req := `{"jsonrpc":"2.0","method":"import","params":"bad","id":11}` + "\n"
	var out bytes.Buffer

	err := ServeIO(provider, strings.NewReader(req), &out)
	if err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error response for invalid params")
	}
	if resp.Error.Code != CodeParseError {
		t.Errorf("expected code %d, got %d", CodeParseError, resp.Error.Code)
	}
}

func TestServe_ReturnsErrorForNilIO(t *testing.T) {
	// Serve() calls ServeIO with nil, which should return error.
	// We can't easily test Serve() itself since it uses os.Stdin/Stdout,
	// but we verify ServeIO nil handling.
	err := ServeIO(newFakeProvider(), nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for nil reader")
	}
}

// fakeSessionsProvider is a fakeProvider that also implements
// deploy.SessionSourceProvider.
type fakeSessionsProvider struct {
	*fakeProvider
	gotList *deploy.ListSessionsRequest
}

func (f *fakeSessionsProvider) ListSessions(
	_ context.Context, req *deploy.ListSessionsRequest,
) (*deploy.ListSessionsResponse, error) {
	f.gotList = req
	return &deploy.ListSessionsResponse{
		Sessions:   []deploy.SessionSummary{{ID: "s-1", HasFailures: true}, {ID: "s-2"}},
		NextCursor: "page-2",
	}, nil
}

func (f *fakeSessionsProvider) GetSession(
	_ context.Context, req *deploy.GetSessionRequest,
) (*deploy.GetSessionResponse, error) {
	return &deploy.GetSessionResponse{Session: deploy.SessionDetail{
		SessionSummary: deploy.SessionSummary{ID: req.SessionID},
		Messages:       json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}}, nil
}

func TestServeIO_Sessions_Supported(t *testing.T) {
	provider := &fakeSessionsProvider{fakeProvider: newFakeProvider()}

	no := false
	listIn := makeRequest("list_sessions",
		deploy.ListSessionsRequest{DeployConfig: "{}", Environment: "default", FilterPassed: &no, Limit: 2}, 11) + "\n"
	var listOut bytes.Buffer
	if err := ServeIO(provider, strings.NewReader(listIn), &listOut); err != nil {
		t.Fatalf("ServeIO list_sessions: %v", err)
	}
	if !strings.Contains(listOut.String(), `"s-1"`) || !strings.Contains(listOut.String(), `"next_cursor":"page-2"`) {
		t.Errorf("expected two summaries and a cursor, got %s", listOut.String())
	}
	if provider.gotList == nil || provider.gotList.FilterPassed == nil || *provider.gotList.FilterPassed || provider.gotList.Limit != 2 {
		t.Errorf("filters did not reach the provider: %+v", provider.gotList)
	}

	getIn := makeRequest("get_session", deploy.GetSessionRequest{DeployConfig: "{}", SessionID: "s-1"}, 12) + "\n"
	var getOut bytes.Buffer
	if err := ServeIO(provider, strings.NewReader(getIn), &getOut); err != nil {
		t.Fatalf("ServeIO get_session: %v", err)
	}
	if !strings.Contains(getOut.String(), `"id":"s-1"`) || !strings.Contains(getOut.String(), `"content":"hi"`) {
		t.Errorf("expected the session with its raw messages, got %s", getOut.String())
	}
}

func TestServeIO_Sessions_Unsupported(t *testing.T) {
	provider := newFakeProvider() // no SessionSourceProvider
	for _, method := range []string{"list_sessions", "get_session"} {
		in := makeRequest(method, map[string]any{}, 13) + "\n"
		var out bytes.Buffer
		if err := ServeIO(provider, strings.NewReader(in), &out); err != nil {
			t.Fatalf("ServeIO %s: %v", method, err)
		}
		var resp response
		if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Error == nil || resp.Error.Code != CodeMethodNotFound {
			t.Errorf("%s: expected method-not-found for a sessions-less provider, got %+v", method, resp.Error)
		}
	}
}

func TestServeIO_Sessions_InvalidParams(t *testing.T) {
	provider := &fakeSessionsProvider{fakeProvider: newFakeProvider()}
	in := `{"jsonrpc":"2.0","id":14,"method":"list_sessions","params":"not-an-object"}` + "\n"
	var out bytes.Buffer
	if err := ServeIO(provider, strings.NewReader(in), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}
	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != CodeParseError || !strings.HasPrefix(resp.Error.Message, invalidParamsPrefix) {
		t.Errorf("expected invalid-params parse error, got %+v", resp.Error)
	}
}
