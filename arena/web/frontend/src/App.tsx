import { useCallback, useEffect, useMemo, useRef, useState, Component } from "react";
import type { ReactNode, ErrorInfo } from "react";
import { ArrowLeft } from "lucide-react";
import { TopBar } from "@/components/arena/TopBar";
import { Hero } from "@/components/arena/Hero";
import { CommandStrip } from "@/components/arena/CommandStrip";
import { DevToolsPanel } from "@/components/DevToolsPanel";
import { HistoricalResults } from "@/components/HistoricalResults";
import { InteractiveChat } from "@/components/InteractiveChat";
import { TrialMatrix } from "@/components/arena/TrialMatrix";
import { InstrumentBand } from "@/components/arena/InstrumentBand";
import { TrialInspector } from "@/components/arena/TrialInspector";
import { SessionReview, ConstellationGraph } from "@altairalabs/atlas";
import { useArenaEvents } from "@/hooks/useArenaEvents";
import { useArenaAPI } from "@/hooks/useArenaAPI";
import { useTheme } from "@/hooks/useTheme";
import { AudioPlayer } from "@/audio/player";
import { buildMatrix, overlayWorkflowRun } from "@/lib/arenaView";
import { adaptRun, adaptWorkflow } from "@/lib/atlasAdapter";
import type { Message, RunResult, ActiveRun, ProviderInfo, ScenarioInfo, TrialCell, WorkflowGraph } from "@/types";

// activeRunToResult maps a still-running ActiveRun into a synthetic
// RunResult-shaped entry so buildMatrix can overlay it onto the trial
// matrix without any special-casing — it lands in the same scenario×
// provider cell as the run it's replacing, and stays selectable via its
// runId while it's in flight.
function activeRunToResult(run: ActiveRun): RunResult {
  return {
    RunID: run.runId,
    PromptPack: "",
    Region: run.region,
    ScenarioID: run.scenario,
    ProviderID: run.provider,
    Params: {},
    Messages: [],
    Commit: {},
    Cost: {
      input_tokens: run.costs.inputTokens,
      output_tokens: run.costs.outputTokens,
      input_cost_usd: 0,
      output_cost_usd: 0,
      total_cost_usd: run.costs.totalCost,
    },
    Violations: [],
    StartTime: run.startTime,
    EndTime: run.startTime,
    Duration: run.duration ?? 0,
    Error: run.error ?? "",
    SelfPlay: false,
    PersonaID: "",
    MediaOutputs: [],
    A2AAgents: [],
  };
}

class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  constructor(props: { children: ReactNode }) {
    super(props);
    this.state = { error: null };
  }
  static getDerivedStateFromError(error: Error) { return { error }; }
  componentDidCatch(error: Error, info: ErrorInfo) { console.error("Arena UI error:", error, info); }
  render() {
    if (this.state.error) {
      return (
        <div className="min-h-screen bg-canvas flex items-center justify-center p-8">
          <div className="rounded-xl border border-red-200 bg-surface p-8 max-w-lg w-full text-center shadow-sm">
            <h2 className="text-lg font-semibold text-[#EF4444] mb-2">Something went wrong</h2>
            <p className="text-sm text-fg-muted mb-6">{this.state.error.message}</p>
            <button
              className="rounded-lg bg-blue-50 border border-blue-200 px-4 py-2 text-sm font-medium text-[#2563EB] hover:bg-blue-100 transition-colors"
              onClick={() => this.setState({ error: null })}
            >
              Try again
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

export default function App() {
  const { registerInteractiveRun, ...state } = useArenaEvents();
  const { theme, toggle: toggleTheme } = useTheme();
  const [activeTab, setActiveTab] = useState<"runs" | "chat">("runs");
  const { startRun, getResults, getResult, getConfig, getRunOptions, clearResults, getWorkflow, loading } = useArenaAPI();
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [showLedger, setShowLedger] = useState(false);
  const [selectedScenario, setSelectedScenario] = useState<string | null>(null);
  const [devToolsMessage, setDevToolsMessage] = useState<Message | undefined>();
  const [devToolsAllMessages, setDevToolsAllMessages] = useState<Message[] | undefined>();
  const [devToolsIndex, setDevToolsIndex] = useState<number | undefined>();
  const [devToolsOpen, setDevToolsOpen] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);
  const [historicalResults, setHistoricalResults] = useState<RunResult[]>([]);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);
  const [promptpack, setPromptpack] = useState<string | undefined>(undefined);
  const [workflowGraph, setWorkflowGraph] = useState<WorkflowGraph | null>(null);

  // The workflow topology is static for the life of the config — fetched
  // once on mount, same pattern as run-options/config below. TrialInspector
  // renders a placeholder until this resolves.
  useEffect(() => {
    getWorkflow()
      .then((graph) => setWorkflowGraph(graph))
      .catch(() => {});
  }, [getWorkflow]);

  // Run-options (the providers/scenarios universe) drive the matrix's
  // columns/rows — same fetch pattern as RunControls/EmptyStateLauncher.
  useEffect(() => {
    getRunOptions()
      .then((opts) => {
        setProviders(opts.providers ?? []);
        setScenarios(opts.scenarios ?? []);
      })
      .catch(() => {});
  }, [getRunOptions]);

  // CommandStrip's chip selection defaults to the first scenario once
  // run-options land — mirrors the old pickers' "first scenario by sort
  // order" default.
  useEffect(() => {
    if (!selectedScenario && scenarios.length > 0) {
      setSelectedScenario(scenarios[0].id);
    }
  }, [scenarios, selectedScenario]);

  // TopBar's promptpack context — "<name> · <version>" when the arena
  // config has a loaded pack, else omitted entirely (TopBar renders nothing
  // for an undefined promptpack).
  useEffect(() => {
    getConfig()
      .then((cfg) => {
        const pack = cfg?.loaded_pack;
        if (pack?.name) {
          setPromptpack(pack.version ? `${pack.name} · ${pack.version}` : pack.name);
        }
      })
      .catch(() => {});
  }, [getConfig]);

  // Single global AudioPlayer for the TrialInspector's "Listen" toggle;
  // rebuilt when the user switches Listen target. Restored from the
  // original RunDetail-era wiring (see git history pre-TrialInspector).
  const playerRef = useRef<AudioPlayer | null>(null);
  const [listeningRunId, setListeningRunId] = useState<string | null>(null);

  useEffect(() => {
    return () => {
      playerRef.current?.close();
      playerRef.current = null;
    };
  }, []);

  // Toggle audio playback for a run. Closes any prior EventSource before
  // opening a new one so we never have two audio streams in flight.
  const handleListen = useCallback((runId: string) => {
    if (listeningRunId === runId) {
      playerRef.current?.pause();
      setListeningRunId(null);
      return;
    }
    if (playerRef.current) {
      playerRef.current.close();
      playerRef.current = null;
    }
    playerRef.current = new AudioPlayer({
      runId,
      onError: (msg) => console.warn("audio:", msg),
    });
    playerRef.current.connect("/api/events");
    playerRef.current.play();
    setListeningRunId(runId);
  }, [listeningRunId]);

  // Track runIds we've already seen so a freshly-spawned run can be auto-
  // selected even when StartRun returns no runId.
  const knownRunIdsRef = useRef<Set<string>>(new Set(Object.keys(state.runs)));
  const [pendingAutoSelect, setPendingAutoSelect] = useState(false);

  // Load historical results on mount and after each run completes.
  const completedCount = state.completedRunIds.length;
  useEffect(() => {
    getResults().then(async (ids) => {
      if (!ids || ids.length === 0) { setHistoricalResults([]); return; }
      const results = await Promise.all(ids.map((id) => getResult(id).catch(() => null)));
      setHistoricalResults(results.filter((r): r is RunResult => r !== null));
    }).catch(() => {});
  }, [getResults, getResult, completedCount]);

  // When the user clicks Start Run we set pendingAutoSelect; the next new
  // runId that lands in state.runs gets auto-selected so the demo flow is
  // "click Run → page navigates to live conversation."
  useEffect(() => {
    const ids = Object.keys(state.runs);
    if (pendingAutoSelect) {
      // Skip synthetic interactive-chat runs — they have no RunDetail to show.
      const newId = ids.find(
        (id) => !knownRunIdsRef.current.has(id) && state.runs[id]?.scenario !== "interactive"
      );
      if (newId) {
        const newRun = state.runs[newId];
        if (newRun) setSelectedKey(`${newRun.scenario}:${newRun.provider}`);
        setSelectedRunId(newId);
        setPendingAutoSelect(false);
      }
    }
    knownRunIdsRef.current = new Set(ids);
  }, [state.runs, pendingAutoSelect]);

  // Exclude synthetic interactive-chat entries from the runs-tab aggregates.
  const liveRuns = Object.values(state.runs).filter((r) => r.scenario !== "interactive");
  const selectedRun = selectedRunId ? state.runs[selectedRunId] : undefined;

  // The trial matrix overlays in-flight runs onto historical results so a
  // cell with a live run stays selectable while its trial is running. Only
  // runs still "running" are overlaid: a "completed" ActiveRun has no
  // ConversationAssertions, so cellPassRate would read it as a bare 100%
  // pass regardless of the real outcome. Excluding it means a just-finished
  // cell briefly shows its prior/empty state instead of a false pass in the
  // window before historicalResults' async refetch lands.
  const matrixResults = useMemo(
    () => [...historicalResults, ...liveRuns.filter((r) => r.status === "running").map(activeRunToResult)],
    [historicalResults, liveRuns],
  );
  const matrix = useMemo(
    () => buildMatrix(matrixResults, providers, scenarios),
    [matrixResults, providers, scenarios],
  );

  // CommandStrip's chip-click contract: clicking a scenario selects that
  // scenario's best provider (matrix row's `best` cell), falling back to
  // the row's first provider when nothing's run yet.
  const bestProviderForScenario = useCallback(
    (scenarioId: string | null): { id: string; label: string } | undefined => {
      if (!scenarioId) return undefined;
      const row = matrix.rows.find((r) => r.scenarioId === scenarioId);
      const cell = row?.cells.find((c) => c.best) ?? row?.cells[0];
      if (!cell) return undefined;
      return matrix.providers.find((p) => p.id === cell.providerId) ?? { id: cell.providerId, label: cell.providerId };
    },
    [matrix],
  );
  const chartProvider = useMemo(
    () => bestProviderForScenario(selectedScenario),
    [bestProviderForScenario, selectedScenario],
  );

  // The Trial Inspector is driven entirely off selectedKey: it looks up the
  // backing cell in the matrix, then the run behind that cell — the saved
  // RunResult if one's landed, else the still-running ActiveRun.
  const selectedCell = useMemo(
    () => (selectedKey ? matrix.rows.flatMap((row) => row.cells).find((c) => c.key === selectedKey) : undefined),
    [matrix, selectedKey],
  );
  // The inspector's run prefers the SPECIFIC run identified by selectedRunId
  // (set when a ledger/historical row is clicked) over the cell's latest
  // run — buildMatrix always pins a cell's runId to the most recent run for
  // that scenario:provider, so an older ledger row would otherwise be
  // shadowed by a newer run in the same cell. Falls back to the run behind
  // selectedCell.runId, then to a matching live ActiveRun.
  const selectedCellRun: RunResult | ActiveRun | undefined = useMemo(() => {
    const bySelectedRunId = selectedRunId
      ? historicalResults.find((r) => r.RunID === selectedRunId)
      : undefined;
    if (bySelectedRunId) return bySelectedRunId;

    const byCell = selectedCell?.runId
      ? (historicalResults.find((r) => r.RunID === selectedCell.runId) ??
         liveRuns.find((r) => r.runId === selectedCell.runId))
      : undefined;
    if (byCell) return byCell;

    return selectedRunId ? liveRuns.find((r) => r.runId === selectedRunId) : undefined;
  }, [selectedRunId, selectedCell, historicalResults, liveRuns]);

  // When the run being shown differs from the matrix cell (an older ledger
  // run selected while the cell points at a newer one), the StatusPill and
  // terminal readout must reflect the SHOWN run, not the (possibly newer)
  // cell. isRunResult narrows: only completed RunResults carry
  // ConversationAssertions/Cost/Duration to derive a cell-shaped reading
  // from; a live ActiveRun's own cell already matches (buildMatrix overlays
  // it 1:1), so it's returned as-is.
  const inspectorCell: TrialCell | undefined = useMemo(() => {
    if (!selectedCell) return undefined;
    if (!selectedCellRun) return selectedCell;
    const isRunResult = "Messages" in selectedCellRun && Array.isArray(selectedCellRun.Messages);
    if (!isRunResult) return selectedCell;
    const run = selectedCellRun as RunResult;
    if (run.RunID === selectedCell.runId) return selectedCell;
    return {
      ...selectedCell,
      scenarioId: run.ScenarioID,
      providerId: run.ProviderID,
      passed: run.ConversationAssertions?.passed ?? !run.Error,
      costUsd: run.Cost?.total_cost_usd ?? 0,
      // run.Duration is nanoseconds (Go time.Duration on the wire); convert
      // to milliseconds to match TrialCell.latencyMs's contract, same as
      // buildMatrix does for the cells sourced directly from arenaView.
      latencyMs: (run.Duration ?? 0) / 1e6,
      runId: run.RunID,
      hasData: true,
    };
  }, [selectedCell, selectedCellRun]);

  const selectedProviderLabel = useMemo(
    () => matrix.providers.find((p) => p.id === inspectorCell?.providerId)?.label ?? inspectorCell?.providerId ?? "",
    [matrix, inspectorCell],
  );

  const handleSelectMessage = (index: number, message?: Message, allMsgs?: Message[]) => {
    setDevToolsIndex(index);
    setDevToolsMessage(message);
    setDevToolsAllMessages(allMsgs);
    setDevToolsOpen(true);
  };

  // Selecting a matrix cell drives both the matrix's own selection ring and
  // the TrialInspector navigation (which reads off selectedKey); selectedRunId
  // is kept in step purely so DevToolsPanel's live-run lookup keeps working.
  const handleSelectCell = useCallback((key: string) => {
    setSelectedKey(key);
    const cell = matrix.rows.flatMap((row) => row.cells).find((c) => c.key === key);
    if (cell?.hasData && cell.runId) {
      setSelectedRunId(cell.runId);
      setDevToolsOpen(false);
    }
  }, [matrix]);

  // Clears the Trial Inspector back to the matrix/empty-hero view.
  const handleBackFromInspector = useCallback(() => {
    setSelectedKey(null);
    setSelectedRunId(null);
    setDevToolsOpen(false);
  }, []);

  // Rows in the ledger (HistoricalResults) carry a RunResult, not a matrix
  // key — derive the key so selecting a ledger row opens the same Trial
  // Inspector a matrix click would.
  const handleSelectHistoricalRun = useCallback((id: string) => {
    const r = historicalResults.find((x) => x.RunID === id);
    if (r) setSelectedKey(`${r.ScenarioID}:${r.ProviderID}`);
    setSelectedRunId(id);
    setDevToolsOpen(false);
  }, [historicalResults]);

  // handleStartRun kicks off a run for an arbitrary set of provider/scenario
  // ids — shared by "Run trial" (all providers) and a single matrix-cell run
  // (one provider × one scenario).
  const handleStartRun = useCallback(async (providerIds: string[], scenarioIds: string[]) => {
    setStartError(null);
    setPendingAutoSelect(true);
    // If the user is currently viewing a previous run's detail, navigate
    // them back to the dashboard immediately. Without this they'd stare
    // at the old run until SSE delivered the first turn of the new one,
    // which feels like nothing happened. The dashboard shows the live
    // run appearing, then pendingAutoSelect kicks in and switches to
    // the new TrialInspector when the runId lands.
    setSelectedRunId(null);
    setSelectedKey(null);
    setDevToolsOpen(false);
    try {
      await startRun({ providers: providerIds, scenarios: scenarioIds });
    } catch (err) {
      setPendingAutoSelect(false);
      setStartError(err instanceof Error ? err.message : "Failed to start run");
    }
  }, [startRun]);

  // Backs the CommandStrip's "Run trial" — runs the selected scenario across
  // EVERY configured provider so it fills the whole matrix row in one go
  // (real providers are billed; that's intended).
  const handleRunTrial = useCallback(() => {
    if (!selectedScenario || providers.length === 0) return;
    void handleStartRun(providers.map((p) => p.id), [selectedScenario]);
  }, [selectedScenario, providers, handleStartRun]);

  // Clicking an empty matrix cell runs just that scenario×provider pair.
  const handleRunCell = useCallback((scenarioId: string, providerId: string) => {
    void handleStartRun([providerId], [scenarioId]);
  }, [handleStartRun]);

  return (
    <ErrorBoundary>
      <div className="min-h-screen bg-canvas" style={{ paddingLeft: 32, paddingRight: 32 }}>
        <TopBar
          connected={state.connected}
          promptpack={promptpack}
          runningLive={liveRuns.some((r) => r.status === "running")}
          theme={theme}
          onToggleTheme={toggleTheme}
        />
        <main className="py-8">
        {/* Tab bar */}
        <div className="flex gap-1 mb-6 border-b border-mist pb-0">
          <button
            className={`px-4 py-2 text-sm font-medium rounded-t-lg border border-b-0 transition-colors ${
              activeTab === "runs"
                ? "bg-surface border-mist text-fg"
                : "bg-canvas border-transparent text-fg-muted hover:text-fg hover:bg-surface"
            }`}
            onClick={() => { setActiveTab("runs"); setDevToolsOpen(false); }}
          >
            Runs
          </button>
          <button
            className={`px-4 py-2 text-sm font-medium rounded-t-lg border border-b-0 transition-colors ${
              activeTab === "chat"
                ? "bg-surface border-mist text-fg"
                : "bg-canvas border-transparent text-fg-muted hover:text-fg hover:bg-surface"
            }`}
            onClick={() => { setActiveTab("chat"); setDevToolsOpen(false); }}
          >
            Interactive Chat
          </button>
        </div>

        {activeTab === "chat" ? (
          <InteractiveChat
            state={state}
            registerInteractiveRun={registerInteractiveRun}
            onBack={() => setActiveTab("runs")}
          />
        ) : (
          <>
            <div className={devToolsOpen ? "lg:mr-[420px] transition-[margin] duration-200" : "transition-[margin] duration-200"}>
              {selectedKey && selectedCell ? (
                <div className="space-y-4">
                  <button
                    onClick={handleBackFromInspector}
                    className="flex items-center gap-2 text-sm text-[#2563EB] hover:underline"
                  >
                    <ArrowLeft className="h-4 w-4" /> Back
                  </button>
                  {selectedCellRun && "Messages" in selectedCellRun && Array.isArray(selectedCellRun.Messages) ? (
                    (() => {
                      const run = selectedCellRun as RunResult;
                      const a = adaptRun(run);
                      const wf =
                        workflowGraph && workflowGraph.nodes.length
                          ? adaptWorkflow(overlayWorkflowRun(workflowGraph, run))
                          : null;
                      return (
                        <div style={{ height: "calc(100vh - 210px)", minHeight: 460 }}>
                          <SessionReview
                            title={a.title}
                            messages={a.messages}
                            checks={a.checks}
                            recording={a.recording}
                            tabs={
                              wf
                                ? [{ id: "workflow", label: "Workflow", render: () => <ConstellationGraph nodes={wf.nodes} edges={wf.edges} theme={theme} direction="LR" height="100%" /> }]
                                : undefined
                            }
                          />
                        </div>
                      );
                    })()
                  ) : (
                    <TrialInspector
                      run={selectedCellRun}
                      cell={inspectorCell}
                      scenarioId={inspectorCell?.scenarioId ?? selectedCell.scenarioId}
                      providerId={inspectorCell?.providerId ?? selectedCell.providerId}
                      providerLabel={selectedProviderLabel}
                      workflowGraph={workflowGraph}
                      onSelectMessage={handleSelectMessage}
                      listeningRunId={listeningRunId}
                      onToggleListen={handleListen}
                      theme={theme}
                    />
                  )}
                </div>
              ) : (
                <div className="space-y-8">
                  <Hero scenarioCount={scenarios.length} providerCount={providers.length} />
                  <CommandStrip
                    scenarios={scenarios}
                    selectedScenario={selectedScenario}
                    selectedProviderLabel={chartProvider?.label ?? null}
                    onSelectScenario={setSelectedScenario}
                    onRunTrial={handleRunTrial}
                    runDisabled={!state.connected || loading || !selectedScenario || providers.length === 0}
                  />
                  {startError && (
                    <div className="rounded-xl bg-red-50 border border-red-200 px-4 py-3 text-sm text-[#EF4444]">{startError}</div>
                  )}
                  <div className="flex justify-end">
                    <button
                      onClick={() => setShowLedger((v) => !v)}
                      className="rounded-lg border border-mist bg-surface px-3 py-1.5 text-xs font-medium text-fg-muted hover:text-fg hover:bg-[var(--c-surface-2)] transition-colors"
                    >
                      {showLedger ? "Hide ledger" : "Show ledger"}
                    </button>
                  </div>
                  <InstrumentBand matrix={matrix} results={matrixResults} />
                  <TrialMatrix matrix={matrix} selectedKey={selectedKey} onSelect={handleSelectCell} onRunCell={handleRunCell} />
                  {showLedger && (
                    <HistoricalResults
                      results={historicalResults}
                      onSelectRun={handleSelectHistoricalRun}
                      onClear={async () => {
                        await clearResults();
                        setHistoricalResults([]);
                      }}
                    />
                  )}
                </div>
              )}
            </div>
            <DevToolsPanel
              message={devToolsMessage}
              messageIndex={devToolsIndex}
              allMessages={devToolsAllMessages}
              run={selectedRun}
              open={devToolsOpen}
              onClose={() => setDevToolsOpen(false)}
            />
          </>
        )}
        </main>
      </div>
    </ErrorBoundary>
  );
}
