package adaptersdk

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/deploy"
)

// RuntimeOverrideSupport declares which parts of a deploy.RuntimeOverride an
// adapter's platform can honor. Image, Env and the handoff contract are
// required of every adapter that advertises deploy.RuntimeOverrideCapability;
// the fields here are the ones a platform may genuinely lack.
type RuntimeOverrideSupport struct {
	// Command is true when the platform can replace the container entrypoint.
	Command bool
	// Args is true when the platform can replace the container arguments.
	Args bool
	// SecretEnv is true when the platform can source an environment variable
	// from its secret store.
	SecretEnv bool
	// ReservedEnvPrefixes are name prefixes the adapter sets itself. An
	// override may not set a variable under one, so it can never displace
	// anything the adapter needs the container to see. The handoff names
	// (deploy.EnvPackJSON, EnvPackURI, EnvAgent) are always reserved.
	ReservedEnvPrefixes []string
	// ReservedEnvNames are exact names the adapter or the platform sets.
	ReservedEnvNames []string
}

// envNamePattern is the portable environment variable name: what POSIX shells
// accept and every container platform passes through unchanged.
var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateRuntimeOverride checks an override against what the adapter can
// honor and returns one error per problem, in a stable order. A nil override
// is valid.
//
// It refuses rather than drops: an override field the platform cannot apply is
// an error, because deploying without it would run something other than what
// the config describes.
func ValidateRuntimeOverride(o *deploy.RuntimeOverride, s RuntimeOverrideSupport) []string {
	if o == nil {
		return nil
	}
	const key = deploy.RuntimeOverrideConfigKey
	var errs []string
	if len(o.Command) > 0 && !s.Command {
		errs = append(errs, key+".command is not supported by this platform: "+
			"build an image whose entrypoint runs what you need")
	}
	if len(o.Args) > 0 && !s.Args {
		errs = append(errs, key+".args is not supported by this platform: "+
			"configure the runtime through env instead")
	}
	if len(o.SecretEnv) > 0 && !s.SecretEnv {
		errs = append(errs, key+".secret_env is not supported by this platform")
	}

	for _, name := range sortedEnvNames(o.Env) {
		errs = append(errs, checkEnvName(key+".env", name, s)...)
	}
	for _, name := range sortedSecretNames(o.SecretEnv) {
		errs = append(errs, checkEnvName(key+".secret_env", name, s)...)
		if _, dup := o.Env[name]; dup {
			errs = append(errs, fmt.Sprintf(
				"%s.secret_env %q is also set in %s.env: set it in one place", key, name, key))
		}
		if strings.TrimSpace(o.SecretEnv[name].Name) == "" {
			errs = append(errs, fmt.Sprintf("%s.secret_env %q needs a secret name", key, name))
		}
	}
	return errs
}

// checkEnvName validates one variable name against the portable form and the
// adapter's reserved names.
func checkEnvName(field, name string, s RuntimeOverrideSupport) []string {
	if !envNamePattern.MatchString(name) {
		return []string{fmt.Sprintf("%s %q is not a valid environment variable name", field, name)}
	}
	for _, prefix := range s.ReservedEnvPrefixes {
		if strings.HasPrefix(name, prefix) {
			return []string{fmt.Sprintf("%s %q is reserved: %s* names are set by the adapter", field, name, prefix)}
		}
	}
	for _, reserved := range append(handoffEnvNames(), s.ReservedEnvNames...) {
		if name == reserved {
			return []string{fmt.Sprintf("%s %q is reserved: the adapter or platform sets it", field, name)}
		}
	}
	return nil
}

// handoffEnvNames are the variables every adapter sets for the pack handoff,
// reserved whatever the adapter declares.
func handoffEnvNames() []string {
	return []string{deploy.EnvPackJSON, deploy.EnvPackURI, deploy.EnvAgent}
}

func sortedEnvNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedSecretNames(m map[string]deploy.SecretRef) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RuntimeOverrideSchema is the JSON Schema for the runtime_override block, for
// an adapter to place under "properties" in its own ProviderInfo.ConfigSchema
// so every adapter describes the block the same way.
func RuntimeOverrideSchema() map[string]any {
	const (
		kType       = "type"
		kProps      = "properties"
		kAdditional = "additionalProperties"
		tObject     = "object"
		fName       = "name"
	)
	str := map[string]any{kType: "string"}
	strList := map[string]any{kType: "array", "items": str}
	secretRef := map[string]any{
		kType:       tObject,
		"required":  []string{fName},
		kAdditional: false,
		kProps:      map[string]any{fName: str, "version": str},
	}
	return map[string]any{
		kType:       tObject,
		kAdditional: false,
		"description": "Run another container in place of the adapter's runtime. " +
			"The adapter still delivers the pack under " + deploy.EnvPackJSON + " or " +
			deploy.EnvPackURI + ", and the agent name under " + deploy.EnvAgent + ".",
		kProps: map[string]any{
			"image":      str,
			"command":    strList,
			"args":       strList,
			"env":        map[string]any{kType: tObject, kAdditional: str},
			"secret_env": map[string]any{kType: tObject, kAdditional: secretRef},
		},
	}
}
