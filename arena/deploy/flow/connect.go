package flow

import (
	"context"
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
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

// Connect discovers and starts the adapter subprocess for provider. The returned
// client's lifetime is bound to ctx; call Close when done.
func Connect(ctx context.Context, provider, projectDir string) (*deploy.AdapterClient, error) {
	path, err := deploy.NewAdapterManager(projectDir).Discover(provider)
	if err != nil {
		return nil, fmt.Errorf("adapter not found for provider %q: %w\nInstall it with: %s",
			provider, err, InstallCommand(provider))
	}
	return deploy.NewAdapterClientWithContext(ctx, path)
}

// Lock acquires the deploy file lock for projectDir and returns a release func.
func Lock(projectDir string) (func(), error) {
	locker := deploy.NewLocker(projectDir)
	if err := locker.Lock(); err != nil {
		return nil, err
	}
	return func() { _ = locker.Unlock() }, nil
}
