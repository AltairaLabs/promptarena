package deploy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// RuntimeOverrideCapability is the ProviderInfo capability string an adapter
// advertises when it honors a runtime_override block in its config.
//
// The CLI refuses to plan a config that carries runtime_override against an
// adapter that does not advertise it. An adapter that predates the block would
// otherwise ignore it and deploy its normal runtime, which is the one outcome
// the operator asked not to get.
const RuntimeOverrideCapability = "runtime_override"

// RuntimeOverrideConfigKey is the top-level deploy config key that holds a
// RuntimeOverride. It is the only key in an adapter's config the SDK and the
// CLI interpret; every other key belongs to the adapter.
const RuntimeOverrideConfigKey = "runtime_override"

// Runtime handoff environment. An adapter that honors runtime_override still
// delivers the pack to the container it deploys, and does so under these
// names, so a replacement runtime can find the pack without knowing which
// adapter deployed it.
const (
	// EnvPackJSON carries the compiled pack inline, as JSON.
	EnvPackJSON = "PROMPTPACK_PACK_JSON"
	// EnvPackURI carries a URI the pack was staged to, when it is too large to
	// pass inline. The scheme is the platform's own (gs://, s3://, https://).
	EnvPackURI = "PROMPTPACK_PACK_URI"
	// EnvAgent names the pack prompt (agent) this container serves.
	EnvAgent = "PROMPTPACK_AGENT"
)

// RuntimeOverride replaces the runtime an adapter deploys with another
// container, or wraps it with another entrypoint, without the adapter knowing
// what that runtime is.
//
// Every field is optional. An empty field keeps what the adapter would have
// deployed: no Image keeps the adapter's runtime image, no Command keeps the
// image's entrypoint. Env and SecretEnv add to the adapter's environment; they
// never replace a variable the adapter sets.
//
// An adapter whose platform cannot honor a field — a substrate with no way to
// set a container command, say — must refuse a config that sets it rather than
// deploy without it. adaptersdk.ValidateRuntimeOverride does that check.
type RuntimeOverride struct {
	// Image is the container image to run instead of the adapter's runtime.
	// Its form is whatever the platform pulls from (for example an Artifact
	// Registry, ECR or ACR reference).
	Image string `json:"image,omitempty"`
	// Command replaces the image's entrypoint.
	Command []string `json:"command,omitempty"`
	// Args replaces the image's default arguments.
	Args []string `json:"args,omitempty"`
	// Env adds plain environment variables to the container.
	Env map[string]string `json:"env,omitempty"`
	// SecretEnv adds environment variables whose values the platform reads
	// from its own secret store at run time, so the value never appears in
	// the deploy config, the plan or the adapter's state.
	SecretEnv map[string]SecretRef `json:"secret_env,omitempty"`
}

// SecretRef names a secret in the platform's secret store. Name is in the
// form the platform uses (a Secret Manager secret id, a Secrets Manager ARN, a
// Key Vault secret name); Version is optional and defaults to the platform's
// notion of "latest".
type SecretRef struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// IsZero reports whether the override sets nothing.
func (o *RuntimeOverride) IsZero() bool {
	return o == nil || (o.Image == "" && len(o.Command) == 0 && len(o.Args) == 0 &&
		len(o.Env) == 0 && len(o.SecretEnv) == 0)
}

// RuntimeOverrideFromConfig reads the runtime_override block from a deploy
// config. It returns nil, with no error, when the config has no such block or
// the block is empty.
func RuntimeOverrideFromConfig(configJSON string) (*RuntimeOverride, error) {
	if configJSON == "" {
		return nil, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid config JSON: %w", err)
	}
	block, ok := raw[RuntimeOverrideConfigKey]
	if !ok || string(block) == "null" {
		return nil, nil
	}
	// Unknown keys are refused: a misspelled "image" would otherwise decode
	// to an empty override, and the adapter would deploy its own runtime.
	dec := json.NewDecoder(bytes.NewReader(block))
	dec.DisallowUnknownFields()
	var o RuntimeOverride
	if err := dec.Decode(&o); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Value == "number" {
			return nil, fmt.Errorf("%s: %w (quote numeric values: env values are strings)",
				RuntimeOverrideConfigKey, err)
		}
		return nil, fmt.Errorf("%s: %w", RuntimeOverrideConfigKey, err)
	}
	if o.IsZero() {
		return nil, nil
	}
	return &o, nil
}
