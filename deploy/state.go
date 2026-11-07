// Package deploy provides deployment state persistence for PromptKit arenas.
package deploy

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	stateDir      = ".promptarena"
	envsDir       = "envs"
	stateFile     = "deploy.state"
	planFile      = "deploy.plan"
	stateVersion  = 1
	stateDirPerm  = 0o750
	stateFilePerm = 0o600
)

// envNamePattern restricts environment names to characters that are safe as a
// single path segment. The name becomes a directory under .promptarena/envs,
// so a separator or ".." would let one environment's state escape into (or
// overwrite) another's.
var envNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateEnvironmentName reports whether env can scope deploy state, plans
// and locks.
func ValidateEnvironmentName(env string) error {
	if !envNamePattern.MatchString(env) {
		return fmt.Errorf("invalid deploy environment name %q: use letters, digits, '.', '_' or '-', "+
			"starting with a letter or digit", env)
	}
	return nil
}

// State represents the persisted deployment state.
type State struct {
	Version        int    `json:"version"`
	Provider       string `json:"provider"`
	Environment    string `json:"environment"`
	LastDeployed   string `json:"last_deployed"`
	PackVersion    string `json:"pack_version"`
	PackChecksum   string `json:"pack_checksum"`
	AdapterVersion string `json:"adapter_version"`
	LastRefreshed  string `json:"last_refreshed,omitempty"`
	State          string `json:"state,omitempty"` // Opaque base64 adapter state
}

// StateStore manages deploy state persistence for one environment.
//
// State and saved plans live under .promptarena/envs/<env>/, so deploying,
// destroying or refreshing one environment can never read or overwrite
// another's adapter state. Earlier releases kept a single
// .promptarena/deploy.state per directory whatever the environment; Load
// adopts that legacy file only when its recorded environment matches this
// store's, and refuses (rather than silently ignoring it) when it does not.
type StateStore struct {
	baseDir string
	env     string
}

// NewStateStore creates a StateStore rooted at baseDir for environment env.
// An invalid environment name is reported by the first operation.
func NewStateStore(baseDir, env string) *StateStore {
	return &StateStore{baseDir: baseDir, env: env}
}

// Environment returns the environment this store is scoped to.
func (s *StateStore) Environment() string { return s.env }

// envDir returns .promptarena/envs/<env>.
func (s *StateStore) envDir() string {
	return envDirFor(s.baseDir, s.env)
}

func envDirFor(baseDir, env string) string {
	return filepath.Join(baseDir, stateDir, envsDir, env)
}

// statePath returns the full path to the state file.
func (s *StateStore) statePath() string {
	return filepath.Join(s.envDir(), stateFile)
}

// legacyStatePath is the pre-environment-scoping state file.
func (s *StateStore) legacyStatePath() string {
	return filepath.Join(s.baseDir, stateDir, stateFile)
}

// legacyPlanPath is the pre-environment-scoping plan file.
func (s *StateStore) legacyPlanPath() string {
	return filepath.Join(s.baseDir, stateDir, planFile)
}

// LegacyStateMismatchError reports a legacy, unscoped state file that belongs
// to a different environment from the one being operated on.
type LegacyStateMismatchError struct {
	Path        string // the legacy state file
	Environment string // the environment it records
	Requested   string // the environment that was asked for
}

func (e *LegacyStateMismatchError) Error() string {
	return fmt.Sprintf(
		"legacy deploy state %s belongs to environment %q, not %q; refusing to use it. "+
			"Run 'promptarena deploy status --env %s' once to move it to its per-environment location, "+
			"then retry",
		e.Path, e.Environment, e.Requested, e.Environment)
}

// Load reads the deploy state for this store's environment.
// Returns nil, nil if no state exists for it.
func (s *StateStore) Load() (*State, error) {
	if err := ValidateEnvironmentName(s.env); err != nil {
		return nil, err
	}
	st, err := readState(s.statePath())
	if err != nil || st != nil {
		return st, err
	}
	return s.adoptLegacy()
}

// adoptLegacy migrates a legacy single-file state into this environment's
// location when it was recorded for this environment.
func (s *StateStore) adoptLegacy() (*State, error) {
	legacy := s.legacyStatePath()
	st, err := readState(legacy)
	if err != nil || st == nil {
		return nil, err
	}
	if st.Environment != s.env {
		return nil, &LegacyStateMismatchError{Path: legacy, Environment: st.Environment, Requested: s.env}
	}
	if err := os.MkdirAll(s.envDir(), stateDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}
	if err := os.Rename(legacy, s.statePath()); err != nil {
		return nil, fmt.Errorf("failed to migrate legacy state file: %w", err)
	}
	// A legacy plan was computed against the legacy state; it is never fresh
	// once state is per-environment, so drop it rather than leave it orphaned.
	_ = os.Remove(s.legacyPlanPath())
	return st, nil
}

func readState(path string) (*State, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is built from the project dir and a validated env name
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file %s: %w", path, err)
	}
	return &state, nil
}

// writeJSON marshals v into the environment directory, creating it as needed.
func (s *StateStore) writeJSON(path, what string, v any) error {
	if err := ValidateEnvironmentName(s.env); err != nil {
		return err
	}
	if err := os.MkdirAll(s.envDir(), stateDirPerm); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", what, err)
	}
	if err := os.WriteFile(path, data, stateFilePerm); err != nil {
		return fmt.Errorf("failed to write %s file: %w", what, err)
	}
	return nil
}

// Save writes the deploy state to disk, creating directories as needed.
func (s *StateStore) Save(state *State) error {
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}
	state.Version = stateVersion
	return s.writeJSON(s.statePath(), "state", state)
}

// removeIfExists removes path, treating "already gone" as success.
func (s *StateStore) removeIfExists(path string) error {
	if err := ValidateEnvironmentName(s.env); err != nil {
		return err
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Delete removes the state file.
func (s *StateStore) Delete() error {
	return s.removeIfExists(s.statePath())
}

// ComputePackChecksum computes a sha256 checksum of pack data.
func ComputePackChecksum(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}

// PackIdentityChecksum returns a checksum of the pack's content that ignores
// compilation.created_at. Every compile stamps a new created_at, so a checksum
// over the raw bytes changes even when nothing in the pack did — which made
// every saved plan stale and pack_checksum useless for spotting an unchanged
// pack. The pack is re-encoded canonically (sorted keys) without that field;
// data that is not a JSON object falls back to the raw-bytes checksum.
func PackIdentityChecksum(data []byte) string {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil || doc == nil {
		return ComputePackChecksum(data)
	}
	if comp, ok := doc["compilation"].(map[string]any); ok {
		delete(comp, "created_at")
	}
	canonical, err := json.Marshal(doc)
	if err != nil {
		return ComputePackChecksum(data)
	}
	return ComputePackChecksum(canonical)
}

// PackVersion returns the pack's top-level "version", or "" when the data is
// not a pack object or carries none.
func PackVersion(data []byte) string {
	var head struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return ""
	}
	return head.Version
}

// SavedPlan is a serialized deployment plan that can be applied later.
type SavedPlan struct {
	Version      int           `json:"version"`
	CreatedAt    string        `json:"created_at"`
	Provider     string        `json:"provider"`
	Environment  string        `json:"environment"`
	PackChecksum string        `json:"pack_checksum"`
	Plan         *PlanResponse `json:"plan"`
	Request      *PlanRequest  `json:"request"`
}

// planPath returns the full path to the plan file.
func (s *StateStore) planPath() string {
	return filepath.Join(s.envDir(), planFile)
}

// SavePlan persists a deployment plan to disk.
func (s *StateStore) SavePlan(plan *SavedPlan) error {
	if plan == nil {
		return fmt.Errorf("plan cannot be nil")
	}
	plan.Version = stateVersion
	return s.writeJSON(s.planPath(), "plan", plan)
}

// LoadPlan reads this environment's saved plan. Returns nil, nil if not found.
func (s *StateStore) LoadPlan() (*SavedPlan, error) {
	if err := ValidateEnvironmentName(s.env); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.planPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read plan file: %w", err)
	}

	var plan SavedPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan file: %w", err)
	}

	return &plan, nil
}

// DeletePlan removes the saved plan file. Returns nil if not found.
func (s *StateStore) DeletePlan() error {
	return s.removeIfExists(s.planPath())
}

// NewSavedPlan creates a SavedPlan with current timestamp.
func NewSavedPlan(provider, environment, packChecksum string, plan *PlanResponse, request *PlanRequest) *SavedPlan {
	return &SavedPlan{
		Version:      stateVersion,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Provider:     provider,
		Environment:  environment,
		PackChecksum: packChecksum,
		Plan:         plan,
		Request:      request,
	}
}

// NewState creates a new State with the given parameters and current timestamp.
func NewState(provider, environment, packVersion, packChecksum, adapterVersion string) *State {
	return &State{
		Version:        stateVersion,
		Provider:       provider,
		Environment:    environment,
		LastDeployed:   time.Now().UTC().Format(time.RFC3339),
		PackVersion:    packVersion,
		PackChecksum:   packChecksum,
		AdapterVersion: adapterVersion,
	}
}
