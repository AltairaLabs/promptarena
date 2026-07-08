package flow

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

var errGetInfoBoom = errors.New("provider info unavailable")

type fakeLogin struct {
	authURL string
	profile map[string]interface{}
	token   string

	getURLErr   error // if set, GetLoginURL fails immediately (no callback fired)
	completeErr error // if set, CompleteLogin fails
	noCallback  bool  // if set, never hits the loopback callback (exercises the ctx-timeout branch)
}

func (f *fakeLogin) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return &deploy.ProviderInfo{Capabilities: []string{deploy.LoginCapability}}, nil
}
func (f *fakeLogin) GetLoginURL(_ context.Context, req *deploy.LoginURLRequest) (*deploy.LoginURLResponse, error) {
	if f.getURLErr != nil {
		return nil, f.getURLErr
	}
	if f.noCallback {
		return &deploy.LoginURLResponse{AuthorizeURL: f.authURL}, nil
	}
	// Simulate the browser hitting the callback with the CSRF state.
	go func() {
		time.Sleep(20 * time.Millisecond)
		http.Get(req.CallbackURL + "?state=" + req.State + "&code=abc") //nolint:errcheck
	}()
	return &deploy.LoginURLResponse{AuthorizeURL: f.authURL}, nil
}
func (f *fakeLogin) CompleteLogin(context.Context, *deploy.CompleteLoginRequest) (*deploy.CompleteLoginResponse, error) {
	if f.completeErr != nil {
		return nil, f.completeErr
	}
	return &deploy.CompleteLoginResponse{Profile: f.profile, Token: f.token}, nil
}

func TestRunLoginFlow_HappyPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fl := &fakeLogin{authURL: "https://auth.example/x", token: "tok-xyz"}

	var opened string
	var statuses []string
	var authURL string
	hooks := LoginHooks{
		OpenBrowser:    func(u string) error { opened = u; return nil },
		OnStatus:       func(msg string) { statuses = append(statuses, msg) },
		OnAuthorizeURL: func(u string) { authURL = u },
	}
	err := runLoginFlow(context.Background(), fl, "omnia", "arena.yaml", "{}", hooks, writeTokenOnly)
	if err != nil {
		t.Fatal(err)
	}
	if opened != fl.authURL {
		t.Fatalf("browser opened %q, want %q", opened, fl.authURL)
	}
	if authURL != fl.authURL {
		t.Fatalf("OnAuthorizeURL got %q, want %q", authURL, fl.authURL)
	}
	if len(statuses) == 0 {
		t.Fatal("expected OnStatus to be invoked with progress messages")
	}
	if tok, ok := LookupCredential("omnia", "arena.yaml"); !ok || tok != "tok-xyz" {
		t.Fatalf("credential not stored: %q %v", tok, ok)
	}
}

// TestRunLoginFlow_GetLoginURLError covers the branch where the adapter fails
// to produce an authorize URL: runLoginFlow must wrap and return the error
// before ever touching the browser.
func TestRunLoginFlow_GetLoginURLError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fl := &fakeLogin{getURLErr: errors.New("adapter unreachable")}

	opened := false
	hooks := LoginHooks{OpenBrowser: func(string) error { opened = true; return nil }}
	err := runLoginFlow(context.Background(), fl, "omnia", "arena.yaml", "{}", hooks, writeTokenOnly)
	if err == nil || !strings.Contains(err.Error(), "failed to get login URL") {
		t.Fatalf("expected a get-login-URL error, got %v", err)
	}
	if opened {
		t.Fatal("browser must not be opened when GetLoginURL fails")
	}
}

// TestRunLoginFlow_OpenBrowserFailure covers the soft-fail branch where the
// browser cannot be opened automatically: the flow must still proceed (the
// URL was already surfaced via OnAuthorizeURL) and report the fallback status.
func TestRunLoginFlow_OpenBrowserFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fl := &fakeLogin{authURL: "https://auth.example/x", token: "tok-xyz"}

	var statuses []string
	hooks := LoginHooks{
		OpenBrowser: func(string) error { return errors.New("no display") },
		OnStatus:    func(msg string) { statuses = append(statuses, msg) },
	}
	if err := runLoginFlow(context.Background(), fl, "omnia", "arena.yaml", "{}", hooks, writeTokenOnly); err != nil {
		t.Fatalf("runLoginFlow: %v", err)
	}
	found := false
	for _, s := range statuses {
		if strings.Contains(s, "Could not open a browser") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a fallback status message when the browser fails to open, got %v", statuses)
	}
}

// TestRunLoginFlow_Timeout covers the ctx.Done() branch: if the browser
// callback never arrives before the context deadline, runLoginFlow must
// return a timeout error rather than blocking forever.
func TestRunLoginFlow_Timeout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fl := &fakeLogin{authURL: "https://auth.example/x", noCallback: true}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := runLoginFlow(ctx, fl, "omnia", "arena.yaml", "{}", LoginHooks{}, writeTokenOnly)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout error, got %v", err)
	}
}

// TestRunLoginFlow_CompleteLoginError covers the branch where the adapter
// fails to complete the login exchange after a successful callback.
func TestRunLoginFlow_CompleteLoginError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fl := &fakeLogin{authURL: "https://auth.example/x", completeErr: errors.New("token exchange failed")}

	hooks := LoginHooks{OpenBrowser: func(string) error { return nil }}
	err := runLoginFlow(context.Background(), fl, "omnia", "arena.yaml", "{}", hooks, writeTokenOnly)
	if err == nil || !strings.Contains(err.Error(), "failed to complete login") {
		t.Fatalf("expected a complete-login error, got %v", err)
	}
}

// TestRunLoginFlow_WriteError covers the branch where the write callback
// (persisting the result) fails: that error must propagate to the caller.
func TestRunLoginFlow_WriteError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fl := &fakeLogin{authURL: "https://auth.example/x", token: "tok-xyz"}

	hooks := LoginHooks{OpenBrowser: func(string) error { return nil }}
	wantErr := errors.New("disk full")
	failWrite := func(string, string, *deploy.CompleteLoginResponse) error { return wantErr }

	err := runLoginFlow(context.Background(), fl, "omnia", "arena.yaml", "{}", hooks, failWrite)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the write error to propagate, got %v", err)
	}
}

// unsupportedLogin is a loginClient whose adapter does not advertise the login
// capability, used to exercise the capability-gate failure path.
type unsupportedLogin struct{ fakeLogin }

func (u *unsupportedLogin) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return &deploy.ProviderInfo{Capabilities: nil}, nil
}

func TestRunLoginFlow_Unsupported(t *testing.T) {
	// An adapter without the login capability fails fast, before any browser work.
	fl := &unsupportedLogin{}
	err := runLoginFlow(context.Background(), fl, "omnia", "arena.yaml", "{}", LoginHooks{}, writeTokenOnly)
	if err == nil || !strings.Contains(err.Error(), "does not support login") {
		t.Fatalf("expected unsupported-login error, got %v", err)
	}
}

func TestRequireLoginCapability(t *testing.T) {
	if err := requireLoginCapability(context.Background(), &fakeLogin{}, "omnia"); err != nil {
		t.Errorf("expected supported, got %v", err)
	}
	if err := requireLoginCapability(context.Background(), &unsupportedLogin{}, "omnia"); err == nil {
		t.Error("expected error for adapter without the login capability")
	}
}

// erroringInfoLogin fails GetProviderInfo, exercising the branch in
// requireLoginCapability where the info call itself errors (distinct from a
// successful call that simply lacks the login capability).
type erroringInfoLogin struct{ fakeLogin }

func (e *erroringInfoLogin) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return nil, errGetInfoBoom
}

func TestRequireLoginCapability_GetProviderInfoError(t *testing.T) {
	err := requireLoginCapability(context.Background(), &erroringInfoLogin{}, "omnia")
	if !errors.Is(err, errGetInfoBoom) {
		t.Fatalf("expected the GetProviderInfo error to propagate, got %v", err)
	}
}

func TestStartLoopbackServer(t *testing.T) {
	t.Run("good state delivers params", func(t *testing.T) {
		cbURL, ch, shutdown, err := startLoopbackServer(context.Background(), "st8")
		if err != nil {
			t.Fatalf("startLoopbackServer: %v", err)
		}
		defer shutdown()
		resp, err := http.Get(cbURL + "?state=st8&code=cc") //nolint:noctx // test
		if err != nil {
			t.Fatalf("GET callback: %v", err)
		}
		_ = resp.Body.Close()
		select {
		case params := <-ch:
			if params["code"] != "cc" {
				t.Errorf("code = %q, want cc", params["code"])
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no callback received")
		}
	})

	t.Run("state mismatch is rejected", func(t *testing.T) {
		cbURL, ch, shutdown, err := startLoopbackServer(context.Background(), "right")
		if err != nil {
			t.Fatalf("startLoopbackServer: %v", err)
		}
		defer shutdown()
		resp, err := http.Get(cbURL + "?state=wrong&code=cc") //nolint:noctx // test
		if err != nil {
			t.Fatalf("GET callback: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
		_ = resp.Body.Close()
		select {
		case <-ch:
			t.Error("should not deliver a callback on state mismatch")
		case <-time.After(200 * time.Millisecond):
		}
	})
}

// TestWriteLoginResult_MergesProfileAndStoresCredential exercises the happy
// path: writeLoginResult must both rewrite the config file's deploy.config
// with the completed profile and persist the credential, in that order.
func TestWriteLoginResult_MergesProfileAndStoresCredential(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	configPath := filepath.Join(dir, "arena.yaml")
	manifest := arenaManifest(`  deploy:
    provider: omnia
    config:
      region: us-east-1
`)
	if err := os.WriteFile(configPath, manifest, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	resp := &deploy.CompleteLoginResponse{
		Token: "tok-999",
		Profile: map[string]interface{}{
			"workspace":    "demo-ws",
			"api_endpoint": "https://omnia.example.com",
		},
	}
	if err := writeLoginResult("omnia", configPath, resp); err != nil {
		t.Fatalf("writeLoginResult: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read merged config: %v", err)
	}
	cfg := parseDeployConfig(t, data)
	if cfg["workspace"] != "demo-ws" {
		t.Errorf("workspace = %v, want demo-ws (merged from profile)", cfg["workspace"])
	}
	if cfg["api_endpoint"] != "https://omnia.example.com" {
		t.Errorf("api_endpoint = %v, want merged in", cfg["api_endpoint"])
	}
	if cfg["region"] != "us-east-1" {
		t.Errorf("region = %v, want us-east-1 (preserved)", cfg["region"])
	}

	tok, ok := LookupCredential("omnia", configPath)
	if !ok || tok != "tok-999" {
		t.Fatalf("LookupCredential = %q,%v; want tok-999,true", tok, ok)
	}
}

// TestWriteLoginResult_PropagatesMergeError ensures a failed config rewrite
// (e.g. the config file vanished between preflight and login completion)
// short-circuits before the credential is ever stored.
func TestWriteLoginResult_PropagatesMergeError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	missing := filepath.Join(t.TempDir(), "absent.yaml")
	err := writeLoginResult("omnia", missing, &deploy.CompleteLoginResponse{Token: "tok"})
	if err == nil || !strings.Contains(err.Error(), "config file not found") {
		t.Fatalf("expected config-not-found error, got %v", err)
	}
	if _, ok := LookupCredential("omnia", missing); ok {
		t.Error("credential must not be stored when the config merge fails")
	}
}

func TestWriteTokenOnly_NoToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := writeTokenOnly("omnia", "arena.yaml", &deploy.CompleteLoginResponse{
		Profile: map[string]interface{}{"workspace": "demo"},
	}); err != nil {
		t.Fatalf("writeTokenOnly: %v", err)
	}
	if _, ok := LookupCredential("omnia", "arena.yaml"); ok {
		t.Error("no credential should be stored when there is no token")
	}
}
