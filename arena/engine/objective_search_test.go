package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/evals/handlers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// treeExecutor is a self-play executor that drafts "d<n>" for the nth draft
// (or what draft returns) and has the agent reply with reply(message).
type treeExecutor struct {
	draft   func(n int) string
	reply   func(msg string) string
	drafts  int
	vars    []map[string]string
	seeds   []*int
	sent    []string
	history [][]string // the persona messages already in the conversation when each was sent
	genErr  error
	sendErr error
}

func (e *treeExecutor) GenerateUserMessage(_ context.Context, req turnexecutors.TurnRequest) (types.Message, error) {
	if e.genErr != nil {
		return types.Message{}, e.genErr
	}
	e.drafts++
	e.vars = append(e.vars, req.SelfPlayVars)
	e.seeds = append(e.seeds, req.SelfPlaySeed)
	content := fmt.Sprintf("d%d", e.drafts)
	if e.draft != nil {
		content = e.draft(e.drafts)
	}
	return types.Message{Role: "user", Content: content, CostInfo: &types.CostInfo{TotalCost: 0.001}}, nil
}

func (e *treeExecutor) ExecuteTurn(ctx context.Context, req turnexecutors.TurnRequest) error {
	if req.SelfPlayMessage == nil {
		return errors.New("tree search sends drafted messages only")
	}
	if e.sendErr != nil {
		return e.sendErr
	}
	msg := *req.SelfPlayMessage
	e.sent = append(e.sent, msg.Content)
	state, err := req.StateStoreConfig.Store.(statestore.Store).Load(ctx, req.ConversationID)
	if err != nil {
		return err
	}
	var before []string
	for _, m := range state.Messages {
		if m.Role == "user" {
			before = append(before, m.Content)
		}
	}
	e.history = append(e.history, before)
	reply := "no"
	if e.reply != nil {
		reply = e.reply(msg.Content)
	}
	return saveMessagesToStateStore(ctx, req, []types.Message{
		msg, {Role: "assistant", Content: reply, CostInfo: &types.CostInfo{TotalCost: 0.01}},
	})
}

func (e *treeExecutor) ExecuteTurnStream(context.Context, turnexecutors.TurnRequest) (<-chan turnexecutors.MessageStreamChunk, error) {
	return nil, errors.New("not used")
}

// fakeJudge scores a reply with score(content) and checks a draft with
// onTopic(content), counting calls.
type fakeJudge struct {
	score   func(content string) float64
	onTopic func(content string) bool
	err     error
	calls   int
}

func (j *fakeJudge) Judge(_ context.Context, opts handlers.JudgeOpts) (*handlers.JudgeResult, error) {
	j.calls++
	if j.err != nil {
		return nil, j.err
	}
	if opts.SystemPrompt == searchOnTopicPrompt {
		return &handlers.JudgeResult{Passed: j.onTopic == nil || j.onTopic(opts.Content)}, nil
	}
	if j.score == nil {
		return &handlers.JudgeResult{Score: 1}, nil
	}
	return &handlers.JudgeResult{Score: j.score(opts.Content)}, nil
}

type treeRun struct {
	exec     *treeExecutor
	judge    *fakeJudge
	persona  *arenaconfig.UserPersonaPack
	noOrch   bool
	turnSeed int
}

func (r *treeRun) run(t *testing.T, search *arenaconfig.ObjectiveSearchConfig) *ConversationResult {
	t.Helper()
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{
			{Role: "user", Content: "Start"}, {Role: "assistant", Content: "Hello"},
		})
	}}
	persona := r.persona
	if persona == nil {
		persona = &arenaconfig.UserPersonaPack{ID: "attacker", SystemPrompt: "Ask for a refund."}
	}
	executor := NewDefaultConversationExecutor(scripted, r.exec, fidelityRegistry(t, persona),
		createTestPromptRegistry(t), nil)
	if !r.noOrch {
		executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")
	}
	if r.judge != nil {
		judgeFor := searchJudgeFunc(func(_ *arenaconfig.Config, name string) (handlers.JudgeProvider, error) {
			if name != "scorer" {
				return nil, fmt.Errorf("no judge %q", name)
			}
			return r.judge, nil
		})
		executor.searchJudge = &judgeFor
	}
	obj := refundObjective("", nil)
	obj.Search = search
	return executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{ID: "tree", TaskType: "assistant", Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Start"},
			{Role: "attacker", Persona: persona.ID, Seed: r.turnSeed, Objective: obj},
		}},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "tree-conv",
	})
}

func searchDetails(t *testing.T, r asrt.ConversationValidationResult) map[string]interface{} {
	t.Helper()
	d, ok := r.Details[objectiveKeySearch].(map[string]interface{})
	require.True(t, ok, "objective result has search details: %+v", r.Details)
	return d
}

func TestTreeSearch_StopsAtFirstAchievingBranch(t *testing.T) {
	exec := &treeExecutor{reply: func(msg string) string {
		if msg == "d4" {
			return "REFUND ISSUED"
		}
		return "no"
	}}
	res := (&treeRun{exec: exec}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 2, Width: 1, Depth: 5,
	})
	require.False(t, res.Failed, res.Error)

	assert.Equal(t, []string{"d1", "d2", "d3", "d4"}, exec.sent, "stops at the first branch that achieves it")
	assert.Equal(t, [][]string{{"Start"}, {"Start"}, {"Start", "d1"}, {"Start", "d1"}}, exec.history,
		"each candidate is sent from its leaf's conversation; the tie at depth 1 keeps the first branch")
	assert.Equal(t, []string{"Start", "Hello", "d1", "no", "d4", "REFUND ISSUED"}, contents(res.Messages),
		"the run records the achieving branch")

	r := objectiveResult(t, res)
	assert.True(t, r.Passed)
	assert.Equal(t, 2, r.Details["achieved_at_exchange"])
	assert.Equal(t, 2, r.Details["exchanges"])
	d := searchDetails(t, r)
	assert.Equal(t, 4, d[searchKeyNodes])
	assert.Equal(t, 4, d[searchKeyTargetCalls])
	assert.Equal(t, 4, d[searchKeyAttackerCalls])
	assert.Equal(t, 0, d[searchKeyJudgeCalls])
	assert.Equal(t, 2, d[searchKeyDepthReached])
	assert.Equal(t, searchMaxScore, d[searchKeyBestScore])
	assert.Equal(t, "tree", d[searchKeyStrategy])
	assert.Equal(t, 2, d[searchKeyBranching])
	assert.Equal(t, 1, d[searchKeyWidth])
	assert.Equal(t, 5, d[searchKeyDepth])
	assert.Contains(t, r.Message, "achieved at exchange 2 of 2 (tree search: 4 branches, 0 pruned, best score 10.0)")

	// Every branch's cost counts: the two kept exchanges are in the messages,
	// the other two in the search cost.
	assert.InDelta(t, 4*0.011, res.Cost.TotalCost, 1e-9)

	assert.Equal(t, "1 of 2", exec.vars[0][selfplay.VarSearchBranch])
	assert.Equal(t, "2 of 2", exec.vars[1][selfplay.VarSearchBranch])
	assert.Equal(t, "1", exec.vars[0][selfplay.VarExchange])
	assert.Empty(t, exec.vars[0][selfplay.VarSearchScore], "no score before the first exchange")
	assert.Equal(t, "2", exec.vars[2][selfplay.VarExchange])
	assert.Equal(t, "1", exec.vars[2][selfplay.VarSearchScore])
	assert.Contains(t, exec.vars[2][selfplay.VarObjectiveFeedback], "failed")
	assert.Equal(t, "get a refund", exec.vars[2][selfplay.VarObjective])
}

func TestTreeSearch_KeepsBestScoringBranch(t *testing.T) {
	exec := &treeExecutor{reply: func(msg string) string {
		if msg == "d2" {
			return "maybe"
		}
		return "no"
	}}
	judge := &fakeJudge{score: func(content string) float64 {
		if strings.Contains(content, "maybe") {
			return 7
		}
		return 2
	}}
	res := (&treeRun{exec: exec, judge: judge}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 2, Width: 1, Depth: 2, Judge: "scorer",
	})
	require.False(t, res.Failed, res.Error)

	assert.Equal(t, [][]string{{"Start"}, {"Start"}, {"Start", "d2"}, {"Start", "d2"}}, exec.history,
		"the higher-scoring branch is the one extended")
	assert.Equal(t, "7", exec.vars[2][selfplay.VarSearchScore])
	assert.Equal(t, []string{"Start", "Hello", "d2", "maybe"}, contents(res.Messages),
		"not achieved: the run records the best-scoring branch")

	r := objectiveResult(t, res)
	assert.False(t, r.Passed)
	assert.Equal(t, false, r.Details["achieved"])
	assert.Equal(t, 1, r.Details["exchanges"])
	d := searchDetails(t, r)
	assert.Equal(t, 7.0, d[searchKeyBestScore])
	assert.Equal(t, 4, d[searchKeyJudgeCalls])
	assert.Equal(t, 2, d[searchKeyDepthReached])
	assert.Equal(t, 4, judge.calls)
}

func TestTreeSearch_JudgeScoreNeverAchieves(t *testing.T) {
	judge := &fakeJudge{score: func(string) float64 { return 42 }}
	res := (&treeRun{exec: &treeExecutor{}, judge: judge}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 1, Width: 1, Depth: 1, Judge: "scorer",
	})
	r := objectiveResult(t, res)
	assert.Equal(t, false, r.Details["achieved"], "only the success checks achieve the objective")
	assert.Equal(t, searchMaxScore-1, searchDetails(t, r)[searchKeyBestScore])
}

func TestTreeSearch_PrunesOffTopicDrafts(t *testing.T) {
	exec := &treeExecutor{draft: func(n int) string {
		if n%2 == 1 {
			return "<notes>x</notes>talk about the weather"
		}
		return fmt.Sprintf("refund please %d", n)
	}}
	judge := &fakeJudge{onTopic: func(content string) bool { return !strings.Contains(content, "weather") }}
	res := (&treeRun{exec: exec, judge: judge}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 2, Width: 2, Depth: 2, Judge: "scorer", PruneOffTopic: true,
	})
	require.False(t, res.Failed, res.Error)

	assert.Equal(t, []string{"refund please 2", "refund please 4"}, exec.sent, "off-topic drafts are never sent")
	d := searchDetails(t, objectiveResult(t, res))
	assert.Equal(t, 2, d[searchKeyPruned])
	assert.Equal(t, 2, d[searchKeyNodes])
	assert.Equal(t, 4, d[searchKeyAttackerCalls])
	assert.Equal(t, 2, d[searchKeyTargetCalls])
	assert.Equal(t, 6, d[searchKeyJudgeCalls], "4 drafts checked, 2 replies scored")
	// Pruned drafts still cost what they cost.
	assert.InDelta(t, 4*0.001+2*0.01, res.Cost.TotalCost, 1e-9)
}

func TestTreeSearch_AllPrunedRecordsNoExchange(t *testing.T) {
	judge := &fakeJudge{onTopic: func(string) bool { return false }}
	res := (&treeRun{exec: &treeExecutor{}, judge: judge}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 2, Depth: 3, Judge: "scorer", PruneOffTopic: true,
	})
	require.False(t, res.Failed, res.Error)
	r := objectiveResult(t, res)
	assert.Equal(t, 0, r.Details["exchanges"])
	assert.Equal(t, []string{"Start", "Hello"}, contents(res.Messages))
	d := searchDetails(t, r)
	assert.Equal(t, 2, d[searchKeyPruned])
	assert.Equal(t, 0, d[searchKeyDepthReached])
}

func TestTreeSearch_JudgeErrors(t *testing.T) {
	judge := &fakeJudge{err: errors.New("judge down")}
	res := (&treeRun{exec: &treeExecutor{}, judge: judge}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 1, Depth: 1, Judge: "scorer", PruneOffTopic: true,
	})
	require.False(t, res.Failed, res.Error)
	d := searchDetails(t, objectiveResult(t, res))
	assert.Equal(t, 1, d[searchKeyNodes], "a failed on-topic check keeps the draft")
	assert.Equal(t, 2, d[searchKeyJudgeErrors])
	assert.Equal(t, searchMinScore, d[searchKeyBestScore], "a failed score falls back to the success checks")
}

func TestTreeSearch_Errors(t *testing.T) {
	search := &arenaconfig.ObjectiveSearchConfig{Strategy: "tree", Branching: 1, Depth: 2}

	t.Run("drafting fails", func(t *testing.T) {
		res := (&treeRun{exec: &treeExecutor{genErr: errors.New("persona down")}}).run(t, search)
		assert.True(t, res.Failed)
		assert.Contains(t, res.Error, "persona down")
		r := objectiveResult(t, res)
		assert.Equal(t, true, r.Details[objectiveKeyEndedEarly])
		assert.Equal(t, 1, searchDetails(t, r)[searchKeyAttackerCalls])
	})
	t.Run("sending fails", func(t *testing.T) {
		res := (&treeRun{exec: &treeExecutor{sendErr: errors.New("agent down")}}).run(t, search)
		assert.True(t, res.Failed)
		assert.Contains(t, res.Error, "agent down")
	})
	t.Run("unknown judge", func(t *testing.T) {
		withJudge := &arenaconfig.ObjectiveSearchConfig{Strategy: "tree", Judge: "missing"}
		res := (&treeRun{exec: &treeExecutor{}, judge: &fakeJudge{}}).run(t, withJudge)
		assert.True(t, res.Failed)
		assert.Contains(t, res.Error, `objective.search.judge: no judge "missing"`)
	})
	t.Run("judge not in config", func(t *testing.T) {
		withJudge := &arenaconfig.ObjectiveSearchConfig{Strategy: "tree", Judge: "scorer"}
		res := (&treeRun{exec: &treeExecutor{}}).run(t, withJudge)
		assert.True(t, res.Failed)
		assert.Contains(t, res.Error, `no judge "scorer" in spec.judges`)
	})
	t.Run("executor can't draft", func(t *testing.T) {
		executor := NewDefaultConversationExecutor(&MockTurnExecutor{}, &MockTurnExecutor{},
			createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
		obj := refundObjective("", nil)
		obj.Search = search
		res := executor.ExecuteConversation(context.Background(), ConversationRequest{
			Scenario: &arenaconfig.Scenario{ID: "s", Turns: []arenaconfig.TurnDefinition{
				{Role: "attacker", Persona: "attacker", Objective: obj},
			}},
			Config:           &arenaconfig.Config{},
			StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
			ConversationID:   "c",
		})
		assert.True(t, res.Failed)
		assert.Contains(t, res.Error, "can't draft messages without sending them")
	})
}

func TestTreeSearch_NoEvalRunner(t *testing.T) {
	res := (&treeRun{exec: &treeExecutor{}, noOrch: true}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 1, Depth: 1,
	})
	r := objectiveResult(t, res)
	assert.False(t, r.Passed)
	assert.Equal(t, objectiveErrNoRunner, r.Details["error"])
}

func TestTreeSearch_SeedsEachDraft(t *testing.T) {
	exec := &treeExecutor{}
	(&treeRun{exec: exec, turnSeed: 40}).run(t, &arenaconfig.ObjectiveSearchConfig{Strategy: "tree", Branching: 3, Depth: 1})
	require.Len(t, exec.seeds, 3)
	for i, s := range exec.seeds {
		require.NotNil(t, s)
		assert.Equal(t, 40+i, *s)
	}

	seed := 7
	exec = &treeExecutor{}
	persona := &arenaconfig.UserPersonaPack{ID: "attacker", SystemPrompt: "x",
		Defaults: arenaconfig.PersonaDefaults{Seed: &seed}}
	(&treeRun{exec: exec, persona: persona}).run(t, &arenaconfig.ObjectiveSearchConfig{Strategy: "tree", Branching: 2, Depth: 1})
	require.Len(t, exec.seeds, 2)
	assert.Equal(t, 7, *exec.seeds[0])
	assert.Equal(t, 8, *exec.seeds[1])

	exec = &treeExecutor{}
	(&treeRun{exec: exec}).run(t, &arenaconfig.ObjectiveSearchConfig{Strategy: "tree", Branching: 2, Depth: 1})
	assert.Nil(t, exec.seeds[0], "no seed configured, none sent")
}

func TestTreeSearch_PersonaStateForksPerBranch(t *testing.T) {
	zero := 0
	persona := statefulPersona(arenaconfig.PersonaStopCondition{State: "patience", AtMost: &zero, Outcome: "gave_up"})
	// The agent asks to verify on the first branch only: that branch's
	// patience drops, the other's does not.
	exec := &treeExecutor{reply: func(msg string) string {
		if msg == "d1" {
			return "Please verify."
		}
		return "no"
	}}
	res := (&treeRun{exec: exec, persona: persona}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 2, Width: 2, Depth: 2,
	})
	require.False(t, res.Failed, res.Error)

	require.Len(t, exec.vars, 6)
	assert.Equal(t, "2", exec.vars[0]["state.patience"])
	assert.Equal(t, "1", exec.vars[2]["state.patience"], "drafts from d1 see its patience")
	assert.Equal(t, "2", exec.vars[4]["state.patience"], "drafts from d2 see its own")

	results := stateResults(res)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Equal(t, 2, results[0].Details[keyExchanges])
}

func TestTreeSearch_PersonaStopEndsBranch(t *testing.T) {
	one := 1
	persona := statefulPersona(arenaconfig.PersonaStopCondition{State: "patience", AtMost: &one, Outcome: "gave_up"})
	exec := &treeExecutor{reply: func(string) string { return "Please verify." }}
	res := (&treeRun{exec: exec, persona: persona}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 2, Width: 2, Depth: 3,
	})
	require.False(t, res.Failed, res.Error)
	assert.Len(t, exec.sent, 2, "every branch gave up at depth 1, so none was extended")
	r := objectiveResult(t, res)
	assert.Equal(t, "gave_up", r.Details["persona_outcome"])
	results := stateResults(res)
	require.Len(t, results, 1)
	assert.Equal(t, "gave_up", results[0].Details[stateKeyOutcome])
}

func TestTreeSearch_FidelityChecksTheKeptBranch(t *testing.T) {
	persona := &arenaconfig.UserPersonaPack{ID: "attacker", SystemPrompt: "x", Facts: []arenaconfig.PersonaFact{{ID: "order", Value: "A-100"}}}
	exec := &treeExecutor{draft: func(n int) string {
		if n == 1 {
			return "My order is A-999." // an invented specific, on a branch that is not kept
		}
		return "My order is A-100."
	}, reply: func(msg string) string {
		if strings.Contains(msg, "A-100") {
			return "maybe"
		}
		return "no"
	}}
	judge := &fakeJudge{score: func(c string) float64 {
		if strings.Contains(c, "maybe") {
			return 5
		}
		return 1
	}}
	res := (&treeRun{exec: exec, persona: persona, judge: judge}).run(t, &arenaconfig.ObjectiveSearchConfig{
		Strategy: "tree", Branching: 2, Width: 1, Depth: 2, Judge: "scorer",
	})
	require.False(t, res.Failed, res.Error)
	var fidelity *asrt.ConversationValidationResult
	for i := range res.ConversationAssertionResults {
		if res.ConversationAssertionResults[i].Type == personaFidelityResultType {
			fidelity = &res.ConversationAssertionResults[i]
		}
	}
	require.NotNil(t, fidelity)
	assert.Equal(t, true, fidelity.Details[fidelityKeyReliable], "only the kept branch is checked")
	assert.Equal(t, 2, fidelity.Details["checked_messages"])
}

func TestTreeSearch_StreamingScenarioSearchesToo(t *testing.T) {
	exec := &treeExecutor{reply: func(string) string { return "REFUND ISSUED" }}
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Start"}, {Role: "assistant", Content: "Hello"}})
	}}
	executor := NewDefaultConversationExecutor(scripted, exec, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")
	off := false
	obj := refundObjective("", nil)
	obj.Search = &arenaconfig.ObjectiveSearchConfig{Strategy: "tree", Branching: 2}
	res := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{ID: "s", TaskType: "assistant", Streaming: true, Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Start", Streaming: &off},
			{Role: "attacker", Persona: "attacker", Objective: obj},
		}},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "c",
	})
	require.False(t, res.Failed, res.Error)
	assert.True(t, objectiveResult(t, res).Passed)
	assert.Equal(t, []string{"d1"}, exec.sent)
}

func TestObjectiveOutcomes_ReadSearch(t *testing.T) {
	stats := &SearchStats{Searches: 1, DepthReached: 2, Nodes: 5, PrunedOffTopic: 1, AttackerCalls: 6,
		TargetCalls: 5, JudgeCalls: 11, JudgeErrors: 1, BestScore: 7}
	o := objectiveOutcome{cfg: &arenaconfig.ObjectiveConfig{Search: &arenaconfig.ObjectiveSearchConfig{Strategy: "tree"}},
		exchanges: 2, search: stats}
	res := o.result()

	got := ObjectiveOutcomes([]asrt.ConversationValidationResult{res})
	require.Len(t, got, 1)
	assert.Equal(t, stats, got[0].Search)

	// The same after a JSON round trip, where numbers become float64.
	d := res.Details[objectiveKeySearch].(map[string]interface{})
	jsonLike := map[string]interface{}{}
	for k, v := range d {
		if n, ok := v.(int); ok {
			jsonLike[k] = float64(n)
		} else {
			jsonLike[k] = v
		}
	}
	res.Details[objectiveKeySearch] = jsonLike
	assert.Equal(t, stats, ObjectiveOutcomes([]asrt.ConversationValidationResult{res})[0].Search)

	assert.Nil(t, searchStatsFrom(nil))
}

func TestObjectiveTrialStats_SumsSearch(t *testing.T) {
	trial := func(depth int, best float64) trialRunResult {
		return trialRunResult{objectives: []ObjectiveOutcome{{TurnIndex: 1, Evaluated: true,
			Search: &SearchStats{Searches: 1, DepthReached: depth, Nodes: 3, BestScore: best}}}}
	}
	stats := objectiveTrialStats([]trialRunResult{trial(2, 4), trial(3, 9)})
	require.Len(t, stats, 1)
	require.NotNil(t, stats[0].Search)
	assert.Equal(t, SearchStats{Searches: 2, DepthReached: 5, Nodes: 6, BestScore: 9}, *stats[0].Search)
	assert.Nil(t, AddSearchStats(nil, nil))
}

func TestChecksScore(t *testing.T) {
	pass := asrt.ConversationValidationResult{Passed: true}
	fail := asrt.ConversationValidationResult{}
	skipped := asrt.ConversationValidationResult{Passed: true, Details: map[string]interface{}{"skipped": true}}
	assert.Equal(t, searchMinScore, checksScore(nil, 0))
	assert.Equal(t, searchMinScore, checksScore([]asrt.ConversationValidationResult{fail, skipped}, 2))
	assert.Equal(t, 5.0, checksScore([]asrt.ConversationValidationResult{pass, fail}, 2))
}

func TestSpecSearchJudge(t *testing.T) {
	_, err := specSearchJudge(nil, "x")
	assert.ErrorContains(t, err, `no judge "x"`)
	_, err = specSearchJudge(&arenaconfig.Config{LoadedJudges: map[string]*arenaconfig.JudgeTarget{"x": {}}}, "x")
	assert.Error(t, err)
}
