package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployAdapterParseRegistry(t *testing.T) {
	data := []byte(`{
		"adapters": {
			"agentcore": {
				"repo": "AltairaLabs/promptarena-deploy-agentcore",
				"description": "AWS Bedrock AgentCore",
				"latest": "0.2.0",
				"maintained_by": "AltairaLabs"
			},
			"cloudrun": {
				"repo": "AltairaLabs/promptarena-deploy-cloudrun",
				"description": "Google Cloud Run",
				"latest": "1.0.0",
				"maintained_by": "AltairaLabs"
			}
		}
	}`)

	reg, err := parseRegistry(data)
	if err != nil {
		t.Fatalf("parseRegistry() error: %v", err)
	}

	if len(reg.Adapters) != 2 {
		t.Fatalf("expected 2 adapters, got %d", len(reg.Adapters))
	}

	ac, ok := reg.Adapters["agentcore"]
	if !ok {
		t.Fatal("expected agentcore adapter in registry")
	}
	if ac.Latest != "0.2.0" {
		t.Errorf("agentcore latest = %q, want %q", ac.Latest, "0.2.0")
	}
	if ac.Repo != "AltairaLabs/promptarena-deploy-agentcore" {
		t.Errorf("agentcore repo = %q, want AltairaLabs/...", ac.Repo)
	}

}

// The embedded registry is what `promptarena deploy adapter install <name>`
// resolves against, so an adapter missing from it cannot be installed at all.
func TestDeployAdapterDefaultRegistryAdapters(t *testing.T) {
	reg, err := loadDefaultRegistry()
	if err != nil {
		t.Fatalf("loadDefaultRegistry: %v", err)
	}

	want := map[string]string{
		"agentcore": "AltairaLabs/promptarena-deploy-agentcore",
		"foundry":   "AltairaLabs/promptarena-deploy-foundry",
		"omnia":     "AltairaLabs/PromptArena-deploy-omnia",
		"vertex":    "AltairaLabs/promptarena-deploy-vertex",
	}

	for name, repo := range want {
		entry, ok := reg.Adapters[name]
		if !ok {
			t.Errorf("adapter %q is missing from the default registry", name)
			continue
		}
		if entry.Repo != repo {
			t.Errorf("%s repo = %q, want %q", name, entry.Repo, repo)
		}
		if entry.Latest == "" {
			t.Errorf("%s has no latest version, so install would have nothing to fetch", name)
		}
	}
}

func TestDeployAdapterParseRegistryInvalid(t *testing.T) {
	_, err := parseRegistry([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDeployAdapterBinaryName(t *testing.T) {
	tests := []struct {
		provider string
		goos     string
		goarch   string
		want     string
	}{
		{
			"agentcore", "darwin", "arm64",
			"promptarena-deploy-agentcore_darwin_arm64",
		},
		{
			"agentcore", "linux", "amd64",
			"promptarena-deploy-agentcore_linux_amd64",
		},
		{
			"cloudrun", "windows", "amd64",
			"promptarena-deploy-cloudrun_windows_amd64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := adapterBinaryName(
				tt.provider, tt.goos, tt.goarch,
			)
			if got != tt.want {
				t.Errorf(
					"adapterBinaryName(%q, %q, %q) = %q, want %q",
					tt.provider, tt.goos, tt.goarch,
					got, tt.want,
				)
			}
		})
	}
}

func TestDeployAdapterDownloadURL(t *testing.T) {
	url := adapterDownloadURL(
		"AltairaLabs/promptarena-deploy-agentcore",
		"0.2.0", "agentcore", "darwin", "arm64",
	)
	want := "https://github.com/AltairaLabs/" +
		"promptarena-deploy-agentcore/releases/download/" +
		"v0.2.0/promptarena-deploy-agentcore_darwin_arm64"
	if url != want {
		t.Errorf("adapterDownloadURL() = %q, want %q", url, want)
	}
}

func TestDeployAdapterParseProviderVersion(t *testing.T) {
	tests := []struct {
		input       string
		wantProv    string
		wantVersion string
	}{
		{"agentcore", "agentcore", ""},
		{"agentcore@0.2.0", "agentcore", "0.2.0"},
		{"cloudrun@1.0.0-rc1", "cloudrun", "1.0.0-rc1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p, v := parseProviderVersion(tt.input)
			if p != tt.wantProv || v != tt.wantVersion {
				t.Errorf(
					"parseProviderVersion(%q) = (%q, %q), "+
						"want (%q, %q)",
					tt.input, p, v,
					tt.wantProv, tt.wantVersion,
				)
			}
		})
	}
}

func TestDeployAdapterListAdaptersInDir(t *testing.T) {
	dir := t.TempDir()

	// Create some adapter binaries.
	names := []string{
		"promptarena-deploy-agentcore",
		"promptarena-deploy-cloudrun",
	}
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(
			path, []byte("binary"), adapterBinaryPerms,
		); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	// Create a non-adapter file (should be excluded).
	other := filepath.Join(dir, "some-other-binary")
	if err := os.WriteFile(
		other, []byte("other"), adapterBinaryPerms,
	); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create a subdirectory (should be excluded).
	subdir := filepath.Join(dir, "promptarena-deploy-subdir")
	if err := os.Mkdir(subdir, adapterBinaryPerms); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}

	adapters := listAdaptersInDir(dir)
	if len(adapters) != 2 {
		t.Fatalf("expected 2 adapters, got %d: %v", len(adapters), adapters)
	}

	// Verify both expected adapters are found.
	found := map[string]bool{}
	for _, a := range adapters {
		found[a] = true
	}
	for _, name := range names {
		if !found[name] {
			t.Errorf("expected adapter %q in list", name)
		}
	}
}

func TestDeployAdapterListAdaptersInNonexistentDir(t *testing.T) {
	adapters := listAdaptersInDir("/nonexistent/path/adapters")
	if len(adapters) != 0 {
		t.Errorf("expected 0 adapters for nonexistent dir, got %d", len(adapters))
	}
}

func TestDeployAdapterRegistryProviderList(t *testing.T) {
	reg := &adapterRegistry{
		Adapters: map[string]adapterRegistryEntry{
			"alpha": {Latest: "1.0.0"},
		},
	}
	got := registryProviderList(reg)
	if got != "alpha" {
		t.Errorf("registryProviderList() = %q, want %q", got, "alpha")
	}
}

func TestDeployAdapterLoadDefaultRegistry(t *testing.T) {
	reg, err := loadDefaultRegistry()
	if err != nil {
		t.Fatalf("loadDefaultRegistry() error: %v", err)
	}
	if _, ok := reg.Adapters["agentcore"]; !ok {
		t.Error("expected agentcore in default registry")
	}
	if _, ok := reg.Adapters["omnia"]; !ok {
		t.Error("expected omnia in default registry")
	}
}

func TestResolveInstallVersion(t *testing.T) {
	t.Run("prefers live latest over embedded default", func(t *testing.T) {
		orig := latestVersionFunc
		t.Cleanup(func() { latestVersionFunc = orig })
		latestVersionFunc = func(_ string) (string, error) { return "1.1.0", nil }

		got, src := resolveInstallVersion("omnia", adapterRegistryEntry{
			Repo: "AltairaLabs/PromptArena-deploy-omnia", Latest: "1.0.0",
		})
		if got != "1.1.0" {
			t.Errorf("got %q, want live latest 1.1.0", got)
		}
		if src != versionLive {
			t.Errorf("source = %v, want versionLive", src)
		}
	})

	t.Run("falls back to embedded default when lookup fails", func(t *testing.T) {
		orig := latestVersionFunc
		t.Cleanup(func() { latestVersionFunc = orig })
		latestVersionFunc = func(_ string) (string, error) {
			return "", errTestNoNetwork
		}

		got, src := resolveInstallVersion("omnia", adapterRegistryEntry{
			Repo: "AltairaLabs/PromptArena-deploy-omnia", Latest: "1.0.0",
		})
		if got != "1.0.0" {
			t.Errorf("got %q, want embedded fallback 1.0.0", got)
		}
		if src != versionEmbedded {
			t.Errorf("source = %v, want versionEmbedded", src)
		}
	})

	t.Run("returns empty when neither source yields a version", func(t *testing.T) {
		orig := latestVersionFunc
		t.Cleanup(func() { latestVersionFunc = orig })
		latestVersionFunc = func(_ string) (string, error) {
			return "", errTestNoNetwork
		}

		got, _ := resolveInstallVersion("omnia", adapterRegistryEntry{
			Repo: "AltairaLabs/PromptArena-deploy-omnia", Latest: "",
		})
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

// stubAdapterInstall points the install command at a temp HOME and stubs both
// network calls, returning the recorded download URLs.
func stubAdapterInstall(
	t *testing.T,
	latest func(string) (string, error),
	download func(string) ([]byte, error),
) *[]string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	origLatest, origDownload := latestVersionFunc, httpDownloadFunc
	t.Cleanup(func() { latestVersionFunc, httpDownloadFunc = origLatest, origDownload })

	var urls []string
	latestVersionFunc = latest
	httpDownloadFunc = func(url string) ([]byte, error) {
		urls = append(urls, url)
		return download(url)
	}
	return &urls
}

func TestRunAdapterInstall(t *testing.T) {
	t.Run("installs the live latest version", func(t *testing.T) {
		urls := stubAdapterInstall(t,
			func(string) (string, error) { return "1.4.0", nil },
			func(string) ([]byte, error) { return []byte("binary"), nil },
		)

		if err := runAdapterInstall(nil, []string{"omnia"}); err != nil {
			t.Fatalf("runAdapterInstall() error: %v", err)
		}

		if len(*urls) != 1 || !strings.Contains((*urls)[0], "/download/v1.4.0/") {
			t.Errorf("downloaded %v, want a v1.4.0 URL", *urls)
		}

		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir() error: %v", err)
		}
		installed := filepath.Join(home, promptarenaDotDir, adaptersDirName, "promptarena-deploy-omnia")
		if _, statErr := os.Stat(installed); statErr != nil {
			t.Errorf("adapter not installed at %s: %v", installed, statErr)
		}
	})

	t.Run("a 404 on the live latest names the missing platform asset", func(t *testing.T) {
		stubAdapterInstall(t,
			func(string) (string, error) { return "1.4.0", nil },
			func(url string) ([]byte, error) {
				return nil, &httpStatusError{Code: 404, URL: url}
			},
		)

		err := runAdapterInstall(nil, []string{"omnia"})
		if err == nil {
			t.Fatal("expected an error for a 404 download")
		}
		if !strings.Contains(err.Error(), "no build for") {
			t.Errorf("error %q should report a missing platform build", err)
		}
	})

	t.Run("a 404 on a registry fallback blames the built-in registry", func(t *testing.T) {
		stubAdapterInstall(t,
			func(string) (string, error) { return "", errTestNoNetwork },
			func(url string) ([]byte, error) {
				return nil, &httpStatusError{Code: 404, URL: url}
			},
		)

		err := runAdapterInstall(nil, []string{"omnia"})
		if err == nil {
			t.Fatal("expected an error for a 404 download")
		}
		if !strings.Contains(err.Error(), "built-in registry") {
			t.Errorf("error %q should blame the built-in registry", err)
		}
	})

	t.Run("rejects an unknown adapter", func(t *testing.T) {
		stubAdapterInstall(t,
			func(string) (string, error) { return "1.0.0", nil },
			func(string) ([]byte, error) { return []byte("binary"), nil },
		)

		err := runAdapterInstall(nil, []string{"nope"})
		if err == nil || !strings.Contains(err.Error(), `unknown adapter "nope"`) {
			t.Errorf("got %v, want an unknown-adapter error", err)
		}
	})
}

func TestHTTPStatusError(t *testing.T) {
	err := &httpStatusError{Code: 503, URL: "https://example.invalid/x"}
	want := "download failed: HTTP 503 from https://example.invalid/x"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestAdapterDownloadError(t *testing.T) {
	const repo = "AltairaLabs/promptarena-deploy-omnia"
	notFound := &httpStatusError{
		Code: 404,
		URL:  "https://github.com/" + repo + "/releases/download/v9.9.9/x",
	}

	t.Run("pinned version points at the newest release instead", func(t *testing.T) {
		err := adapterDownloadError(
			notFound, "omnia", "9.9.9", repo, "darwin", "arm64", versionPinned,
		)
		for _, want := range []string{
			"no release v9.9.9",
			"promptarena deploy adapter install omnia",
			"https://github.com/" + repo + "/releases",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q should mention %q", err, want)
			}
		}
	})

	t.Run("embedded fallback blames the stale registry", func(t *testing.T) {
		err := adapterDownloadError(
			notFound, "omnia", "1.0.0", repo, "linux", "amd64", versionEmbedded,
		)
		if !strings.Contains(err.Error(), "built-in registry") {
			t.Errorf("error %q should blame the built-in registry", err)
		}
		if !strings.Contains(err.Error(), "install omnia@<version>") {
			t.Errorf("error %q should suggest pinning a version", err)
		}
	})

	t.Run("live latest blames the missing platform asset", func(t *testing.T) {
		err := adapterDownloadError(
			notFound, "omnia", "1.4.0", repo, "windows", "amd64", versionLive,
		)
		if !strings.Contains(err.Error(), "promptarena-deploy-omnia_windows_amd64") {
			t.Errorf("error %q should name the missing asset", err)
		}
		if !strings.Contains(err.Error(), "no build for windows/amd64") {
			t.Errorf("error %q should say there is no build for the platform", err)
		}
	})

	t.Run("non-404 errors pass through untouched", func(t *testing.T) {
		orig := &httpStatusError{Code: 500, URL: "https://example.invalid/x"}
		err := adapterDownloadError(
			orig, "omnia", "1.4.0", repo, "linux", "amd64", versionLive,
		)
		if !errors.Is(err, error(orig)) {
			t.Errorf("got %v, want the original error unchanged", err)
		}
	})

	t.Run("non-HTTP errors pass through untouched", func(t *testing.T) {
		err := adapterDownloadError(
			errTestNoNetwork, "omnia", "1.4.0", repo, "linux", "amd64", versionLive,
		)
		if !errors.Is(err, errTestNoNetwork) {
			t.Errorf("got %v, want the original error unchanged", err)
		}
	})
}

var errTestNoNetwork = errTestErr("network unavailable")

type errTestErr string

func (e errTestErr) Error() string { return string(e) }
