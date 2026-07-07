package flow

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

type fakeLogin struct {
	authURL string
	profile map[string]interface{}
	token   string
}

func (f *fakeLogin) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return &deploy.ProviderInfo{Capabilities: []string{deploy.LoginCapability}}, nil
}
func (f *fakeLogin) GetLoginURL(_ context.Context, req *deploy.LoginURLRequest) (*deploy.LoginURLResponse, error) {
	// Simulate the browser hitting the callback with the CSRF state.
	go func() {
		time.Sleep(20 * time.Millisecond)
		http.Get(req.CallbackURL + "?state=" + req.State + "&code=abc") //nolint:errcheck
	}()
	return &deploy.LoginURLResponse{AuthorizeURL: f.authURL}, nil
}
func (f *fakeLogin) CompleteLogin(context.Context, *deploy.CompleteLoginRequest) (*deploy.CompleteLoginResponse, error) {
	return &deploy.CompleteLoginResponse{Profile: f.profile, Token: f.token}, nil
}

func TestRunLoginFlow_HappyPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fl := &fakeLogin{authURL: "https://auth.example/x", token: "tok-xyz"}

	var opened string
	hooks := LoginHooks{
		OpenBrowser: func(u string) error { opened = u; return nil },
	}
	err := runLoginFlow(context.Background(), fl, "omnia", "arena.yaml", "{}", hooks, writeTokenOnly)
	if err != nil {
		t.Fatal(err)
	}
	if opened != fl.authURL {
		t.Fatalf("browser opened %q, want %q", opened, fl.authURL)
	}
	if tok, ok := LookupCredential("omnia", "arena.yaml"); !ok || tok != "tok-xyz" {
		t.Fatalf("credential not stored: %q %v", tok, ok)
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
