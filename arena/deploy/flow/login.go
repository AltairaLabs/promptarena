package flow

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pkg/browser"
	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

const (
	// LoginTimeout bounds the whole browser round-trip.
	LoginTimeout              = 5 * time.Minute
	loginStateBytes           = 16
	loopbackReadHeaderTimeout = 10 * time.Second

	// configFilePerms is the fallback permission used when writing a config
	// file that does not yet exist. The profile may carry a scoped token, so
	// default to owner-only.
	configFilePerms = 0o600
	// yamlIndentSpaces matches the repo's 2-space YAML convention.
	yamlIndentSpaces = 2

	yamlTagMap = "!!map"
	yamlTagStr = "!!str"
)

// LoginHooks lets the caller (TUI or CLI) observe login progress and control the
// browser. Nil fields fall back to sane defaults.
type LoginHooks struct {
	OnStatus       func(string)       // human-readable progress ("Waiting for authorization…")
	OnAuthorizeURL func(string)       // the URL to visit (for headless / manual paste)
	OpenBrowser    func(string) error // defaults to browser.OpenURL
}

func (h LoginHooks) status(msg string) {
	if h.OnStatus != nil {
		h.OnStatus(msg)
	}
}

func (h LoginHooks) openBrowser(url string) error {
	if h.OpenBrowser != nil {
		return h.OpenBrowser(url)
	}
	return browser.OpenURL(url)
}

// loginClient is the slice of the adapter login needs.
type loginClient interface {
	GetProviderInfo(context.Context) (*deploy.ProviderInfo, error)
	deploy.LoginProvider
}

type writeResultFn func(provider, configPath string, resp *deploy.CompleteLoginResponse) error

// Login connects the adapter and runs the interactive browser login for provider.
func Login(ctx context.Context, provider string, opts Options, hooks LoginHooks) error {
	ctx, cancel := context.WithTimeout(ctx, LoginTimeout)
	defer cancel()

	dir, err := opts.dir()
	if err != nil {
		return err
	}
	client, err := Connect(ctx, provider, dir)
	if err != nil {
		return err
	}
	defer client.Close()

	_, dep, err := LoadConfig(opts)
	if err != nil {
		return err
	}
	cfgJSON, _ := MergedConfigJSON(dep, ResolveEnv(opts), opts.config())
	return runLoginFlow(ctx, client, provider, opts.config(), cfgJSON, hooks, writeLoginResult)
}

func runLoginFlow(ctx context.Context, client loginClient, provider, configPath, configJSON string,
	hooks LoginHooks, write writeResultFn) error {
	if err := requireLoginCapability(ctx, client, provider); err != nil {
		return err
	}
	state, err := newLoginState()
	if err != nil {
		return err
	}
	callbackURL, resultCh, shutdown, err := startLoopbackServer(ctx, state)
	if err != nil {
		return err
	}
	defer shutdown()

	urlResp, err := client.GetLoginURL(ctx, &deploy.LoginURLRequest{
		CallbackURL: callbackURL, State: state, Config: configJSON,
	})
	if err != nil {
		return fmt.Errorf("failed to get login URL: %w", err)
	}
	hooks.status("Opening your browser to authenticate…")
	if hooks.OnAuthorizeURL != nil {
		hooks.OnAuthorizeURL(urlResp.AuthorizeURL)
	}
	if browserErr := hooks.openBrowser(urlResp.AuthorizeURL); browserErr != nil {
		hooks.status("Could not open a browser automatically — visit the URL above.")
	}
	hooks.status("Waiting for authorization…")

	var params map[string]string
	select {
	case params = <-resultCh:
	case <-ctx.Done():
		return fmt.Errorf("login timed out waiting for the browser callback")
	}

	resp, err := client.CompleteLogin(ctx, &deploy.CompleteLoginRequest{Params: params, Config: configJSON})
	if err != nil {
		return fmt.Errorf("failed to complete login: %w", err)
	}
	return write(provider, configPath, resp)
}

func requireLoginCapability(ctx context.Context, client loginClient, provider string) error {
	info, err := client.GetProviderInfo(ctx)
	if err != nil {
		return err
	}
	for _, c := range info.Capabilities {
		if c == deploy.LoginCapability {
			return nil
		}
	}
	return fmt.Errorf("adapter %q does not support login; configure manually — see %s", provider, ConfigureDocsURL)
}

func newLoginState() (string, error) {
	b := make([]byte, loginStateBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func startLoopbackServer(ctx context.Context, state string) (string, <-chan map[string]string, func(), error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, err
	}
	resultCh := make(chan map[string]string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		params := map[string]string{}
		for k := range q {
			params[k] = q.Get(k)
		}
		_, _ = w.Write([]byte("<html><body>Login complete. You can close this tab.</body></html>"))
		select {
		case resultCh <- params:
		default:
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: loopbackReadHeaderTimeout}
	go srv.Serve(ln) //nolint:errcheck
	callbackURL := fmt.Sprintf("http://%s/callback", ln.Addr().String())
	return callbackURL, resultCh, func() { _ = srv.Close() }, nil
}

// writeLoginResult merges the profile into the config doc and stores the token.
func writeLoginResult(provider, configPath string, resp *deploy.CompleteLoginResponse) error {
	if err := MergeProfileIntoConfigFile(configPath, resp.Profile, provider); err != nil {
		return err
	}
	return writeTokenOnly(provider, configPath, resp)
}

// writeTokenOnly stores only the credential (used in tests and when no doc rewrite is needed).
func writeTokenOnly(provider, configPath string, resp *deploy.CompleteLoginResponse) error {
	if resp.Token == "" {
		return nil
	}
	cred := Credential{Token: resp.Token}
	if ep, ok := resp.Profile["api_endpoint"].(string); ok {
		cred.Endpoint = ep
	}
	if ws, ok := resp.Profile["workspace"].(string); ok {
		cred.Workspace = ws
	}
	return StoreCredential(provider, configPath, cred)
}

// MergeProfileIntoConfigFile deep-merges the profile fragment into the
// spec.deploy.config mapping of the arena config file at configPath, preserving
// the rest of the document (comments, key order, unrelated sections), then
// writes the result back with the file's existing permissions (or
// configFilePerms for a config that does not yet exist).
func MergeProfileIntoConfigFile(configPath string, profile map[string]interface{}, provider string) error {
	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		return fmt.Errorf(
			"config file not found: %s\nCreate one first (e.g. 'promptarena init'), then re-run login",
			configPath,
		)
	}
	doc, err := os.ReadFile(configPath) //nolint:gosec // configPath is the user-specified arena config path
	if err != nil {
		return fmt.Errorf("failed to read config %s: %w", configPath, err)
	}
	merged, err := mergeProfileIntoConfigDoc(doc, profile, provider)
	if err != nil {
		return err
	}
	mode := os.FileMode(configFilePerms)
	if fi, statErr := os.Stat(configPath); statErr == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.WriteFile(configPath, merged, mode); err != nil {
		return fmt.Errorf("failed to write config %s: %w", configPath, err)
	}
	return nil
}

// mergeProfileIntoConfigDoc deep-merges the profile fragment into the
// spec.deploy.config mapping of the arena config YAML document, preserving the
// rest of the document (comments, key order, unrelated sections). The arena
// config is a Kubernetes-style manifest, so the deploy section lives under
// spec. If the deploy section or its config mapping is absent it is created,
// and provider sets spec.deploy.provider when it is otherwise empty.
func mergeProfileIntoConfigDoc(doc []byte, profile map[string]interface{}, provider string) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	rootMap := documentMapping(&root)

	specNode := mappingGet(rootMap, "spec")
	if specNode == nil || specNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf(
			"config is not a valid arena manifest: missing or malformed spec: section",
		)
	}

	deployNode := mappingGet(specNode, "deploy")
	if deployNode == nil {
		deployNode = &yaml.Node{Kind: yaml.MappingNode, Tag: yamlTagMap}
		mappingSet(specNode, "deploy", deployNode)
	}
	if deployNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("deploy section is not a mapping")
	}

	if provider != "" {
		provNode := mappingGet(deployNode, "provider")
		switch {
		case provNode == nil:
			mappingSet(deployNode, "provider", scalarNode(provider))
		case strings.TrimSpace(provNode.Value) == "":
			provNode.Kind = yaml.ScalarNode
			provNode.Tag = yamlTagStr
			provNode.Value = provider
		}
	}

	cfgNode := mappingGet(deployNode, "config")
	if cfgNode == nil {
		cfgNode = &yaml.Node{Kind: yaml.MappingNode, Tag: yamlTagMap}
		mappingSet(deployNode, "config", cfgNode)
	}
	if cfgNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("deploy.config is not a mapping")
	}

	if err := mergeMapIntoNode(cfgNode, profile); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	// Match the repo's 2-space YAML convention; yaml.Marshal would otherwise
	// reindent the entire document to 4 spaces.
	enc.SetIndent(yamlIndentSpaces)
	if err := enc.Encode(&root); err != nil {
		return nil, fmt.Errorf("failed to marshal merged config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("failed to flush merged config: %w", err)
	}
	return buf.Bytes(), nil
}

// documentMapping returns the root mapping node of a YAML document, creating an
// empty mapping when the document is empty or not a mapping.
func documentMapping(root *yaml.Node) *yaml.Node {
	if root.Kind != yaml.DocumentNode {
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: yamlTagMap}
		root.Kind = yaml.DocumentNode
		root.Content = []*yaml.Node{m}
		return m
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: yamlTagMap}
		root.Content = []*yaml.Node{m}
		return m
	}
	return root.Content[0]
}

// mappingGet returns the value node for key in a mapping node, or nil.
func mappingGet(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mappingSet replaces the value for key in a mapping node, appending a new
// key/value pair when the key is absent.
func mappingSet(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, scalarNode(key), val)
}

// scalarNode builds a string scalar node.
func scalarNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

// mergeMapIntoNode deep-merges src into the target mapping node. Keys are
// applied in sorted order for deterministic output. Nested mappings recurse;
// every other value type replaces the existing node.
func mergeMapIntoNode(target *yaml.Node, src map[string]interface{}) error {
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := src[k]
		existing := mappingGet(target, k)
		if sub, ok := v.(map[string]interface{}); ok && existing != nil && existing.Kind == yaml.MappingNode {
			if err := mergeMapIntoNode(existing, sub); err != nil {
				return err
			}
			continue
		}
		node := &yaml.Node{}
		if err := node.Encode(v); err != nil {
			return fmt.Errorf("failed to encode profile value for key %q: %w", k, err)
		}
		mappingSet(target, k, node)
	}
	return nil
}
