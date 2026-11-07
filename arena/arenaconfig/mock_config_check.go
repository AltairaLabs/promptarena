package arenaconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
)

const (
	providerTypeMock = "mock"
	mockConfigKey    = "mock_config"
)

// validateMockConfigs loads every mock provider's mock_config file at config
// load, with the runtime's own strict decoder, so a misspelled key
// (`default_response` for `defaultResponse`, a typo in `scenarios:`) fails
// `promptarena validate` as well as a run. The runtime reads the file only
// when it builds the provider, which validate never does.
func (c *Config) validateMockConfigs() error {
	for _, id := range sortedKeys(c.LoadedProviders) {
		p := c.LoadedProviders[id]
		if p == nil || p.Type != providerTypeMock {
			continue
		}
		path, _ := p.AdditionalConfig[mockConfigKey].(string)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) && c.ConfigDir != "" {
			path = filepath.Join(c.ConfigDir, path)
		}
		if err := checkMockConfigFile(path); err != nil {
			return fmt.Errorf("provider %s: %w", id, err)
		}
	}
	return nil
}

// checkMockConfigFile reports a mock response file the runtime would reject.
// A file that cannot be read is left to the provider, which reports it.
func checkMockConfigFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	_, err := mock.NewFileMockRepository(path)
	return err
}
