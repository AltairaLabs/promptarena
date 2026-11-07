package flow

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/AltairaLabs/promptarena/v2/deploy"
)

// InstallCommand is the exact CLI command that installs a provider's adapter.
func InstallCommand(provider string) string {
	return fmt.Sprintf("promptarena deploy adapter install %s", provider)
}

// AdapterInstalled reports whether the provider's adapter binary is discoverable
// (project-local, user-level, then $PATH) and its resolved path.
func AdapterInstalled(provider, projectDir string) (string, bool) {
	path, err := deploy.NewAdapterManager(projectDir).Discover(provider)
	if err != nil {
		return "", false
	}
	return path, true
}

var (
	adapterStderrMu sync.RWMutex
	adapterStderr   io.Writer = os.Stderr
)

// SetAdapterStderr sets where adapters started by Connect write their stderr
// (default os.Stderr; nil discards it). The TUI points it at its log panel,
// since raw writes to the terminal would paint over the alt-screen. It
// returns the previous writer so the caller can restore it.
func SetAdapterStderr(w io.Writer) io.Writer {
	adapterStderrMu.Lock()
	defer adapterStderrMu.Unlock()
	prev := adapterStderr
	adapterStderr = w
	return prev
}

func currentAdapterStderr() io.Writer {
	adapterStderrMu.RLock()
	defer adapterStderrMu.RUnlock()
	return adapterStderr
}

// Connect discovers and starts the adapter subprocess for provider. The returned
// client's lifetime is bound to ctx; call Close when done.
func Connect(ctx context.Context, provider, projectDir string) (*deploy.AdapterClient, error) {
	path, err := deploy.NewAdapterManager(projectDir).Discover(provider)
	if err != nil {
		return nil, fmt.Errorf("adapter not found for provider %q: %w\nInstall it with: %s",
			provider, err, InstallCommand(provider))
	}
	return deploy.NewAdapterClientWithStderr(ctx, path, currentAdapterStderr())
}

// Lock acquires the deploy file lock for the default environment of
// projectDir, the one a deploy command targets when no --env is given, and
// returns a release func.
//
// Deprecated: locks are per environment; use LockEnv.
func Lock(projectDir string) (func(), error) {
	return LockEnv(projectDir, DefaultEnv)
}

// LockEnv acquires the deploy file lock for one environment of projectDir and
// returns a release func. Environments lock independently, so a staging
// deploy never waits on (or races) a production one.
func LockEnv(projectDir, env string) (func(), error) {
	locker := deploy.NewEnvLocker(projectDir, env)
	if err := locker.Lock(); err != nil {
		return nil, err
	}
	return func() { _ = locker.Unlock() }, nil
}
