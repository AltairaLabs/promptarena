package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AltairaLabs/promptarena/arena/generate"
	"github.com/AltairaLabs/promptarena/deploy"
)

// sessionsClient is the slice of the adapter session sourcing needs.
type sessionsClient interface {
	GetProviderInfo(context.Context) (*deploy.ProviderInfo, error)
	deploy.SessionSourceProvider
	Close() error
}

// SessionSource adapts a connected deploy adapter's sessions capability to
// generate.SessionSourceAdapter, so `promptarena generate --source <adapter>`
// and any in-process caller can pull sessions from the platform the adapter
// deploys to. Close releases the adapter subprocess.
type SessionSource struct {
	provider   string
	version    string
	env        string
	configJSON string
	client     sessionsClient
}

// ErrSessionsUnsupported is the error for an installed adapter that does not
// offer session sourcing: the fix is to upgrade it, and the message says how.
func ErrSessionsUnsupported(provider, version string) error {
	v := version
	if v != "" {
		v = " v" + v
	}
	return fmt.Errorf("adapter %q%s does not support session sourcing; upgrade with: %s",
		provider, v, InstallCommand(provider))
}

// OpenSessionSource connects the installed adapter for provider, checks it
// advertises the sessions capability, and merges the deploy config (with the
// stored login token) it will be handed on every call. The arena config's
// deploy: section supplies the profile even when its provider differs from
// the one named here — the caller chose the adapter explicitly. workspace,
// when non-empty, overrides the profile's workspace.
func OpenSessionSource(ctx context.Context, provider string, opts Options, workspace string) (*SessionSource, error) {
	dir, err := opts.dir()
	if err != nil {
		return nil, err
	}
	client, err := Connect(ctx, provider, dir)
	if err != nil {
		return nil, err
	}
	_, dep, err := LoadConfig(opts)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	env := ResolveEnv(opts)
	cfgJSON, err := MergedConfigJSON(dep, env, opts.config())
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	if cfgJSON, err = withWorkspace(cfgJSON, workspace); err != nil {
		_ = client.Close()
		return nil, err
	}
	return newSessionSource(ctx, client, provider, env, cfgJSON)
}

// newSessionSource performs the capability check and wraps the client. It
// closes the client on failure.
func newSessionSource(
	ctx context.Context, client sessionsClient, provider, env, configJSON string,
) (*SessionSource, error) {
	info, err := client.GetProviderInfo(ctx)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	if !hasCapability(info, deploy.SessionsCapability) {
		_ = client.Close()
		return nil, ErrSessionsUnsupported(provider, info.Version)
	}
	return &SessionSource{provider: provider, version: info.Version, env: env, configJSON: configJSON, client: client}, nil
}

func hasCapability(info *deploy.ProviderInfo, want string) bool {
	if info == nil {
		return false
	}
	for _, c := range info.Capabilities {
		if c == want {
			return true
		}
	}
	return false
}

// withWorkspace sets "workspace" in the merged deploy config JSON. An empty
// workspace leaves the document untouched.
func withWorkspace(configJSON, workspace string) (string, error) {
	if workspace == "" {
		return configJSON, nil
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", fmt.Errorf("deploy config is not a JSON object: %w", err)
	}
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	cfg["workspace"] = workspace
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Name returns the adapter's provider name.
func (s *SessionSource) Name() string { return s.provider }

// Close releases the adapter subprocess.
func (s *SessionSource) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

// List pages through the adapter's sessions until it reports no more or Limit
// is reached. Filters are passed through; the adapter decides whether it
// applies them server-side, and the library re-checks client-side anyway.
func (s *SessionSource) List(ctx context.Context, opts generate.ListOptions) ([]generate.SessionSummary, error) {
	req := &deploy.ListSessionsRequest{
		DeployConfig:   s.configJSON,
		Environment:    s.env,
		FilterPassed:   opts.FilterPassed,
		FilterEvalType: opts.FilterEvalType,
		Expectations:   toWireExpectations(opts.Expectations),
		Limit:          opts.Limit,
	}
	var out []generate.SessionSummary
	for {
		resp, err := s.client.ListSessions(ctx, req)
		if err != nil {
			return nil, s.wrapErr("listing sessions", err)
		}
		for i := range resp.Sessions {
			out = append(out, toGenerateSummary(&resp.Sessions[i]))
			if opts.Limit > 0 && len(out) >= opts.Limit {
				return out, nil
			}
		}
		if resp.NextCursor == "" {
			return out, nil
		}
		req.Cursor = resp.NextCursor
	}
}

// Get fetches one session and maps it onto the generate model.
func (s *SessionSource) Get(ctx context.Context, id string) (*generate.SessionDetail, error) {
	resp, err := s.client.GetSession(ctx, &deploy.GetSessionRequest{
		DeployConfig: s.configJSON, Environment: s.env, SessionID: id,
	})
	if err != nil {
		return nil, s.wrapErr(fmt.Sprintf("fetching session %q", id), err)
	}
	return toGenerateDetail(&resp.Session)
}

// wrapErr turns a method-not-found from an adapter that advertised the
// capability but does not serve the method into the same upgrade guidance.
func (s *SessionSource) wrapErr(what string, err error) error {
	if errors.Is(err, deploy.ErrMethodNotSupported) {
		return ErrSessionsUnsupported(s.provider, s.version)
	}
	return fmt.Errorf("%s from adapter %q: %w", what, s.provider, err)
}

func toWireExpectations(in []generate.Expectation) []deploy.SessionExpectation {
	if len(in) == 0 {
		return nil
	}
	out := make([]deploy.SessionExpectation, len(in))
	for i, e := range in {
		out[i] = deploy.SessionExpectation{EvalID: e.EvalID, Min: e.Min, Max: e.Max}
	}
	return out
}

func toGenerateSummary(w *deploy.SessionSummary) generate.SessionSummary {
	return generate.SessionSummary{
		ID:          w.ID,
		Source:      w.ID,
		ScenarioID:  w.ScenarioID,
		ProviderID:  w.ProviderID,
		Timestamp:   w.Timestamp,
		TurnCount:   w.TurnCount,
		HasFailures: w.HasFailures,
		Tags:        w.Tags,
		Metadata:    w.Metadata,
	}
}

// toGenerateDetail decodes the wire session into the generate model. An
// absent or null messages array is an empty conversation, not an error.
func toGenerateDetail(w *deploy.SessionDetail) (*generate.SessionDetail, error) {
	d := &generate.SessionDetail{
		SessionSummary: toGenerateSummary(&w.SessionSummary),
		Variables:      w.Variables,
	}
	if len(w.Messages) > 0 && string(w.Messages) != "null" {
		if err := json.Unmarshal(w.Messages, &d.Messages); err != nil {
			return nil, fmt.Errorf("session %q: decoding messages: %w", w.ID, err)
		}
	}
	if w.Pack != nil {
		d.Pack = &generate.PackRef{Name: w.Pack.Name, Version: w.Pack.Version, Digest: w.Pack.Digest}
	}
	if w.Workflow != nil {
		d.Workflow = &generate.WorkflowTrace{EntryState: w.Workflow.EntryState, EntryPromptTask: w.Workflow.EntryPromptTask}
		for _, t := range w.Workflow.Transitions {
			d.Workflow.Transitions = append(d.Workflow.Transitions, generate.WorkflowTransition{
				From: t.From, To: t.To, Event: t.Event, PromptTask: t.PromptTask, MessageIndex: t.MessageIndex,
			})
		}
	}
	for i := range w.Evals {
		d.Evals = append(d.Evals, toGenerateEval(&w.Evals[i]))
	}
	return d, nil
}

func toGenerateEval(w *deploy.SessionEval) generate.EvalResult {
	ev := generate.EvalResult{
		ID: w.ID, Type: w.Type, Kind: w.Kind, Score: w.Score, Passed: w.Passed,
		Params: w.Params, Message: w.Message, Details: w.Details, Turn: w.Turn,
	}
	if w.Threshold != nil {
		ev.Threshold = &generate.EvalThreshold{Operator: w.Threshold.Operator, Value: w.Threshold.Value}
	}
	return ev
}

// Ensure the wrapper satisfies the generate contract at compile time.
var _ generate.SessionSourceAdapter = (*SessionSource)(nil)
