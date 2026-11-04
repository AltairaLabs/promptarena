package selfplay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// MetaPersonaOriginal is the user-message metadata key holding the persona's
// message as it was generated, when transforms rewrote what the agent was
// sent.
const MetaPersonaOriginal = "persona_original"

// TransformRequest is what an exec transform reads on stdin.
type TransformRequest struct {
	// Message is the message to rewrite: the persona's, or the previous
	// transform's output.
	Message string `json:"message"`
	// Persona is the persona's id.
	Persona string `json:"persona"`
	// Scenario is the scenario's id.
	Scenario string `json:"scenario"`
	// Exchange is the 1-based self-play exchange the message is for.
	Exchange int `json:"exchange"`
}

// transformResponse is what an exec transform writes on stdout.
type transformResponse struct {
	Message *string `json:"message"`
}

// ApplyTransforms runs a turn's transforms over the persona's message, in
// order, and returns the message to send. configDir resolves a relative exec
// command. A transform that fails, or produces an empty message, is an error:
// sending the message untransformed would test something else.
func ApplyTransforms(
	ctx context.Context, transforms []arenaconfig.MessageTransform, configDir string, req TransformRequest,
) (string, error) {
	msg := req.Message
	for i := range transforms {
		t := &transforms[i]
		var err error
		switch {
		case t.Template != "":
			msg = strings.ReplaceAll(t.Template, arenaconfig.TransformMessagePlaceholder, msg)
		case t.Exec != nil:
			req.Message = msg
			msg, err = runExecTransform(ctx, t.Exec, configDir, req)
		}
		if err != nil {
			return "", fmt.Errorf("transforms[%d]: %w", i, err)
		}
		if strings.TrimSpace(msg) == "" {
			return "", fmt.Errorf("transforms[%d] produced an empty message", i)
		}
	}
	return msg, nil
}

// runExecTransform runs one exec transform.
func runExecTransform(
	ctx context.Context, cfg *arenaconfig.TransformExec, configDir string, req TransformRequest,
) (string, error) {
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if cfg.TimeoutMs == 0 {
		timeout = arenaconfig.DefaultTransformTimeoutMs * time.Millisecond
	}
	if configDir != "" {
		// Absolute, so a relative command isn't resolved against Dir twice.
		if abs, err := filepath.Abs(configDir); err == nil {
			configDir = abs
		}
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	input, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	//nolint:gosec // the program is the operator's, from their own config
	cmd := exec.CommandContext(ctx, transformCommand(configDir, cfg.Command), cfg.Args...)
	// The program runs in the config directory, so its own relative paths,
	// such as a script passed in args, resolve as the config reads.
	cmd.Dir = configDir
	// A child the program leaves running can hold the pipes open; stop
	// waiting for them shortly after the program itself is killed.
	cmd.WaitDelay = transformWaitDelay
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	if err := cmd.Run(); err != nil {
		if parent.Err() != nil {
			return "", parent.Err()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("%s timed out after %s", cfg.Command, timeout)
		}
		return "", fmt.Errorf("%s: %w: %s", cfg.Command, err, strings.TrimSpace(stderr.String()))
	}
	var resp transformResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil || resp.Message == nil {
		return "", fmt.Errorf("%s must write {\"message\": \"...\"} on stdout, got %q", cfg.Command,
			truncate(stdout.String(), 200)) //nolint:mnd // enough of the output to recognize it
	}
	return *resp.Message, nil
}

// transformWaitDelay is how long to wait for a killed transform's output
// pipes to close.
const transformWaitDelay = 2 * time.Second

// transformCommand resolves a command path containing a separator against
// the config directory; a bare name is left for PATH lookup.
func transformCommand(configDir, command string) string {
	if configDir == "" || filepath.IsAbs(command) || !strings.ContainsRune(command, filepath.Separator) {
		return command
	}
	return filepath.Join(configDir, command)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// PersonaText returns a user message's text as the persona wrote it: the
// original when transforms rewrote what the agent was sent.
func PersonaText(msg *types.Message) string {
	if original, ok := msg.Meta[MetaPersonaOriginal].(string); ok {
		return original
	}
	return msg.GetContent()
}

// restorePersonaOriginals gives the persona back its own messages as it wrote
// them, so a transform such as an encoding doesn't feed back into what it
// writes next.
func restorePersonaOriginals(history []types.Message) []types.Message {
	out := history
	copied := false
	for i := range history {
		original, ok := history[i].Meta[MetaPersonaOriginal].(string)
		if !ok || history[i].Role != personaRole {
			continue
		}
		if !copied {
			out = append([]types.Message(nil), history...)
			copied = true
		}
		out[i].Content = original
		out[i].Parts = nil
	}
	return out
}
