package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/statestore"
	"github.com/AltairaLabs/promptarena/arena/tui"
)

// fakeDrillStore mimics the arena store mid-run: GetResult fails (the run's
// metadata isn't finalized until the run ends) but GetArenaState returns the
// conversation exchanged so far.
type fakeDrillStore struct {
	live []types.Message
}

func (f *fakeDrillStore) GetResult(context.Context, string) (*statestore.RunResult, error) {
	return nil, errors.New("run metadata not found")
}

func (f *fakeDrillStore) GetArenaState(context.Context, string) (*statestore.ArenaConversationState, error) {
	st := &statestore.ArenaConversationState{}
	st.Messages = f.live
	return st, nil
}

// TestDrillDown_LiveRunSeedsFromConversationState guards that stepping into a
// still-running conversation shows the turns already exchanged — not just the
// next streamed message. GetResult is empty mid-run, so the seed must come from
// the in-progress conversation state.
func TestDrillDown_LiveRunSeedsFromConversationState(t *testing.T) {
	store := &fakeDrillStore{live: []types.Message{
		{Role: "user", Content: "PASTUSERMSG"},
		{Role: "assistant", Content: "PASTAGENTMSG"},
	}}
	p := &RunPage{store: store}

	cmd := p.drillDownCmd(&tui.RunInfo{
		RunID:    "run-1",
		Status:   tui.StatusRunning,
		Scenario: "s",
		Provider: "prov",
	})
	require.NotNil(t, cmd, "drilling into a running run should push a page")

	push, ok := cmd().(PushPageMsg)
	require.True(t, ok, "expected a PushPageMsg")
	cvp, ok := push.Page.(*ConversationViewPage)
	require.True(t, ok, "expected a *ConversationViewPage")

	cvp.SetSize(120, 40)
	out := stripANSI(cvp.View())
	require.True(t, strings.Contains(out, "PASTUSERMSG"),
		"live drill-in must show the already-exchanged user turn; got:\n%s", out)
	require.True(t, strings.Contains(out, "PASTAGENTMSG"),
		"live drill-in must show the already-exchanged assistant turn; got:\n%s", out)
}
