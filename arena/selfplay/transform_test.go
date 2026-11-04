package selfplay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// TestTransformHelper is the exec transform the tests run: this test binary,
// re-invoked with TRANSFORM_HELPER set to what it should do.
func TestTransformHelper(t *testing.T) {
	mode := os.Getenv("TRANSFORM_HELPER")
	if mode == "" {
		t.Skip("run as a transform by the other tests")
	}
	var req TransformRequest
	_ = json.NewDecoder(os.Stdin).Decode(&req)
	switch mode {
	case "upper":
		out, _ := json.Marshal(map[string]string{
			"message": fmt.Sprintf("%s [%s/%s/%d]", strings.ToUpper(req.Message), req.Persona, req.Scenario, req.Exchange),
		})
		_, _ = os.Stdout.Write(out)
	case "fail":
		_, _ = io.WriteString(os.Stderr, "no dictionary")
		os.Exit(3)
	case "garbage":
		_, _ = io.WriteString(os.Stdout, "not json")
	case "empty":
		_, _ = io.WriteString(os.Stdout, `{"message": "  "}`)
	case "hang":
		select {}
	}
	os.Exit(0)
}

func helperExec(mode string, timeoutMs int) arenaconfig.MessageTransform {
	return arenaconfig.MessageTransform{Exec: &arenaconfig.TransformExec{
		Command: os.Args[0], Args: []string{"-test.run=^TestTransformHelper$"},
		Env: map[string]string{"TRANSFORM_HELPER": mode}, TimeoutMs: timeoutMs,
	}}
}

func TestApplyTransforms(t *testing.T) {
	req := TransformRequest{Message: "refund please", Persona: "p", Scenario: "s", Exchange: 2}
	got, err := ApplyTransforms(context.Background(), []arenaconfig.MessageTransform{
		{Template: "Hi. {{message}}"}, helperExec("upper", 0), {Template: "{{message}} -- sent from my phone"},
	}, "", req)
	require.NoError(t, err)
	assert.Equal(t, "HI. REFUND PLEASE [p/s/2] -- sent from my phone", got, "transforms run in order")

	for mode, want := range map[string]string{
		"fail":    "no dictionary",
		"garbage": `must write {"message": "..."} on stdout, got "not json"`,
		"empty":   "transforms[0] produced an empty message",
	} {
		_, err := ApplyTransforms(context.Background(), []arenaconfig.MessageTransform{helperExec(mode, 0)}, "", req)
		require.Error(t, err, mode)
		assert.Contains(t, err.Error(), want, mode)
	}
	_, err = ApplyTransforms(context.Background(), []arenaconfig.MessageTransform{helperExec("hang", 200)}, "", req)
	assert.ErrorContains(t, err, "timed out after 200ms")
}

func TestTransformCommand(t *testing.T) {
	assert.Equal(t, "python3", transformCommand("/kit", "python3"))
	assert.Equal(t, "/kit/transforms/typos.py", transformCommand("/kit", "transforms/typos.py"))
	assert.Equal(t, "/abs/t.sh", transformCommand("/kit", "/abs/t.sh"))
	assert.Equal(t, "transforms/t.py", transformCommand("", "transforms/t.py"))
}

func TestPersonaText(t *testing.T) {
	sent := types.Message{Role: "user", Content: "SENT", Meta: map[string]interface{}{MetaPersonaOriginal: "wrote"}}
	assert.Equal(t, "wrote", PersonaText(&sent))
	plain := types.Message{Role: "user", Content: "hi"}
	assert.Equal(t, "hi", PersonaText(&plain))

	history := []types.Message{sent, {Role: "assistant", Content: "ok"}, plain}
	restored := restorePersonaOriginals(history)
	assert.Equal(t, "wrote", restored[0].Content)
	assert.Equal(t, "SENT", history[0].Content, "the stored conversation is left as sent")
	assert.Equal(t, "hi", restored[2].Content)
}

func TestApplyTransforms_RelativeConfigDir(t *testing.T) {
	// The test binary as a relative path from a relative config dir: the
	// command must resolve once, not against Dir a second time.
	wd, err := os.Getwd()
	require.NoError(t, err)
	exe, err := filepath.Rel(wd, os.Args[0])
	require.NoError(t, err)
	tr := helperExec("upper", 0)
	tr.Exec.Command = "./" + filepath.Base(exe)
	got, err := ApplyTransforms(context.Background(), []arenaconfig.MessageTransform{tr},
		filepath.Dir(exe), TransformRequest{Message: "hi", Persona: "p", Scenario: "s", Exchange: 1})
	require.NoError(t, err)
	assert.Equal(t, "HI [p/s/1]", got)
}

func TestApplyTransforms_ParentCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ApplyTransforms(ctx, []arenaconfig.MessageTransform{helperExec("hang", 0)}, "", TransformRequest{Message: "hi"})
	assert.ErrorIs(t, err, context.Canceled, "a canceled run is not reported as a transform timeout")
}

func TestConversationText_ReadsThePersonasOriginals(t *testing.T) {
	msgs := []types.Message{{Role: "user", Content: "hi Ref ZX-9001", Meta: map[string]interface{}{MetaPersonaOriginal: "hi"}}}
	assert.Equal(t, "hi\n", ConversationText(msgs))
}
