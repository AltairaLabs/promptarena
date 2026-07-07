package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	promptarenaDotDir  = ".promptarena"
	credentialsFile    = "credentials"
	credentialFilePerm = 0o600
	credentialDirPerm  = 0o700
)

// Credential is a stored deploy login credential.
type Credential struct {
	Token     string `json:"token"`
	Endpoint  string `json:"endpoint,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

// CredentialsPath returns ~/.promptarena/credentials.
func CredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, promptarenaDotDir, credentialsFile), nil
}

// credentialKey identifies a stored credential by the adapter provider and
// the absolute config file it configured — generic (no assumptions about the
// provider's config field names) and unambiguous per config.
func credentialKey(provider, configPath string) string {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		abs = configPath
	}
	return provider + "|" + abs
}

// LoadCredentials returns the credential map; a missing file yields an empty map.
func LoadCredentials() (map[string]Credential, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the user's home dir
	if os.IsNotExist(err) {
		return map[string]Credential{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials %s: %w", path, err)
	}
	out := map[string]Credential{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("failed to parse credentials %s: %w", path, err)
		}
	}
	return out, nil
}

// StoreCredential upserts a credential keyed by provider + absolute config path.
func StoreCredential(provider, configPath string, cred Credential) error {
	store, err := LoadCredentials()
	if err != nil {
		return err
	}
	store[credentialKey(provider, configPath)] = cred
	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), credentialDirPerm); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	if err := os.WriteFile(path, data, credentialFilePerm); err != nil {
		return fmt.Errorf("failed to write credentials %s: %w", path, err)
	}
	return nil
}

// LookupCredential returns the stored token for provider+config, if any.
func LookupCredential(provider, configPath string) (string, bool) {
	store, err := LoadCredentials()
	if err != nil {
		return "", false
	}
	cred, ok := store[credentialKey(provider, configPath)]
	if !ok || cred.Token == "" {
		return "", false
	}
	return cred.Token, true
}
