package mcpsource

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// MountMode is how a mount's files reach the container.
type MountMode string

// Mount modes. A bind mount is live and shared with the host; a copy is a
// snapshot inside the container, writable there, that never touches the host.
const (
	MountBind MountMode = "bind"
	MountCopy MountMode = "copy"
)

// MountWhen is when a mount's files are present in the container.
type MountWhen string

// Mount timings. MountAtEval files are copied in after the agent's last turn,
// just before evaluation, so the agent never sees them.
const (
	MountAtStart MountWhen = "start"
	MountAtEval  MountWhen = "eval"
)

// Mount is one resolved, validated entry of source_args.mounts.
type Mount struct {
	Source   string // absolute host path inside the config directory
	Target   string // absolute container path
	ReadOnly bool   // bind mounts only
	Mode     MountMode
	When     MountWhen
}

// source_args.mounts keys.
const (
	keySource   = "source"
	keyTarget   = "target"
	keyReadOnly = "readonly"
	keyMode     = "mode"
	keyWhen     = "when"
)

var mountKeys = map[string]bool{keySource: true, keyTarget: true, keyReadOnly: true, keyMode: true, keyWhen: true}

// ResolveMounts validates a raw source_args.mounts value and resolves each
// source against baseDir, the config directory. scope is the entry's
// mcp_servers scope, which decides whether when: eval is allowed.
//
// A relative source used to reach `docker run -v` as written, where Docker
// reads it as the name of a new, empty volume. Every source is now an
// absolute path that exists inside baseDir, symlinks included.
func ResolveMounts(raw any, baseDir, scope string) ([]Mount, error) {
	if raw == nil {
		return nil, nil
	}
	entries, err := mountEntries(raw)
	if err != nil {
		return nil, err
	}
	base, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return nil, fmt.Errorf("mounts: resolve config directory %q: %w", baseDir, err)
	}

	var out []Mount
	targets := map[string]int{}
	for i, entry := range entries {
		m, err := resolveMount(entry, base, scope)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", mountLabel(i, entry), err)
		}
		if prev, dup := targets[m.Target]; dup {
			return nil, fmt.Errorf("%s: target already used by mounts[%d]", mountLabel(i, entry), prev)
		}
		targets[m.Target] = i
		out = append(out, m)
	}
	return out, nil
}

// MountArgs renders resolved mounts back into the source_args form, with
// every key set, so a source never has to apply a default of its own.
func MountArgs(ms []Mount) []any {
	out := make([]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{
			keySource: m.Source, keyTarget: m.Target, keyReadOnly: m.ReadOnly,
			keyMode: string(m.Mode), keyWhen: string(m.When),
		})
	}
	return out
}

func mountEntries(raw any) ([]any, error) {
	switch x := raw.(type) {
	case []any:
		return x, nil
	case []map[string]any:
		out := make([]any, len(x))
		for i, m := range x {
			out[i] = m
		}
		return out, nil
	default:
		return nil, fmt.Errorf("mounts: must be a list, got %T", raw)
	}
}

func mountLabel(i int, entry any) string {
	if m, ok := entry.(map[string]any); ok {
		if target, _ := m[keyTarget].(string); target != "" {
			return fmt.Sprintf("mounts[%d] (%s)", i, target)
		}
	}
	return fmt.Sprintf("mounts[%d]", i)
}

func resolveMount(entry any, base, scope string) (Mount, error) {
	m, ok := entry.(map[string]any)
	if !ok {
		return Mount{}, fmt.Errorf("must be a map with source and target, got %T", entry)
	}
	if err := checkMountKeys(m); err != nil {
		return Mount{}, err
	}
	out, err := mountOptions(m)
	if err != nil {
		return Mount{}, err
	}
	if timingErr := checkEvalTiming(out, scope); timingErr != nil {
		return Mount{}, timingErr
	}

	target, _ := m[keyTarget].(string)
	if target == "" {
		return Mount{}, errors.New("target is required")
	}
	if !path.IsAbs(target) {
		return Mount{}, fmt.Errorf("target must be an absolute path, got %q", target)
	}
	out.Target = path.Clean(target)

	src, _ := m[keySource].(string)
	if src == "" {
		return Mount{}, errors.New("source is required")
	}
	if out.Source, err = resolveMountSource(src, base); err != nil {
		return Mount{}, err
	}
	return out, nil
}

// mountOptions reads readonly, mode and when, applying the defaults
// (read-only, bind, start).
func mountOptions(m map[string]any) (Mount, error) {
	out := Mount{ReadOnly: true, Mode: MountBind, When: MountAtStart}
	if v, set := m[keyReadOnly]; set {
		ro, isBool := v.(bool)
		if !isBool {
			return Mount{}, fmt.Errorf("readonly must be true or false, got %v", v)
		}
		out.ReadOnly = ro
	}
	if v, set := m[keyMode]; set {
		mode, _ := v.(string)
		if mode != string(MountBind) && mode != string(MountCopy) {
			return Mount{}, fmt.Errorf(`mode must be "bind" or "copy", got %v`, v)
		}
		out.Mode = MountMode(mode)
	}
	if v, set := m[keyWhen]; set {
		when, _ := v.(string)
		if when != string(MountAtStart) && when != string(MountAtEval) {
			return Mount{}, fmt.Errorf(`when must be "start" or "eval", got %v`, v)
		}
		out.When = MountWhen(when)
	}
	return out, nil
}

// checkEvalTiming rejects when: eval where it cannot hide files from the agent.
func checkEvalTiming(m Mount, scope string) error {
	if m.When != MountAtEval {
		return nil
	}
	if m.Mode != MountCopy {
		return errors.New("when: eval needs mode: copy (a bind mount cannot be added to a running container)")
	}
	if scope != string(ScopeSession) {
		return fmt.Errorf("when: eval needs scope: session, got %q "+
			"(a longer-lived container would keep the files for the next session's agent)", scope)
	}
	return nil
}

func checkMountKeys(m map[string]any) error {
	var unknown []string
	for k := range m {
		if !mountKeys[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("unknown key %q (allowed: source, target, readonly, mode, when)", unknown[0])
}

// resolveMountSource makes src absolute against base and checks it exists and,
// after following symlinks, is still inside base.
func resolveMountSource(src, base string) (string, error) {
	abs := src
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(base, abs)
	}
	abs = filepath.Clean(abs)
	if !within(abs, base) {
		return "", fmt.Errorf("source %q is outside the config directory %s", src, base)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("source %q does not exist (resolved to %s)", src, abs)
	}
	if err != nil {
		return "", fmt.Errorf("source %q: %w", src, err)
	}
	if !within(resolved, base) {
		return "", fmt.Errorf("source %q is outside the config directory %s (it resolves to %s)", src, base, resolved)
	}
	return resolved, nil
}

func within(p, base string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ExpandArgs returns a copy of args with every {{scenario.<name>}} in its
// string values, at any depth, replaced from vars.
func ExpandArgs(args map[string]any, vars map[string]string) map[string]any {
	if args == nil {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = expandArgValue(v, vars)
	}
	return out
}

func expandArgValue(v any, vars map[string]string) any {
	switch x := v.(type) {
	case string:
		for k, val := range vars {
			x = strings.ReplaceAll(x, "{{scenario."+k+"}}", val)
		}
		return x
	case map[string]any:
		return ExpandArgs(x, vars)
	case []any:
		out := make([]any, len(x))
		for i, elem := range x {
			out[i] = expandArgValue(elem, vars)
		}
		return out
	case []map[string]any:
		out := make([]any, len(x))
		for i, elem := range x {
			out[i] = ExpandArgs(elem, vars)
		}
		return out
	default:
		return v
	}
}
