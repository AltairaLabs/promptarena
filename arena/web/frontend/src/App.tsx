import { useCallback, useEffect, useMemo, useRef, useState, Component } from "react";
import type { ReactNode, ErrorInfo } from "react";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { TopBar } from "@/components/arena/TopBar";
import { Hero } from "@/components/arena/Hero";
import { CommandStrip } from "@/components/arena/CommandStrip";

import { HistoricalResults } from "@/components/HistoricalResults";
import { InteractiveChat } from "@/components/InteractiveChat";
import { TrialMatrix } from "@/components/arena/TrialMatrix";
import { InstrumentBand } from "@/components/arena/InstrumentBand";

import { SessionReview, ConstellationGraph, Tabs, Card, Button } from "@altairalabs/atlas";
import { useArenaEvents } from "@/hooks/useArenaEvents";
import { useArenaAPI } from "@/hooks/useArenaAPI";
import { useTheme } from "@/hooks/useTheme";
import { AudioPlayer } from "@/audio/player";
import { buildMatrix, currentWorkflowStateAt, estimateFieldRun, orderLedgerRows, overlayWorkflowCurrentState, overlayWorkflowRun, runScored } from "@/lib/arenaView";
import { adaptAnyRun, adaptWorkflow, isRunResult } from "@/lib/atlasAdapter";
import { arenaInspectorTabs } from "@/lib/arenaInspectorTabs";
import type { RunResult, ActiveRun, ProviderInfo, ScenarioInfo, WorkflowGraph } from "@/types";

// RELOAD_DEBOUNCE_MS coalesces the burst of run-completion events from a big
// field run into a single historical-results refetch, instead of one refetch
// per completed run (which storms the store and collapses the ledger).
const RELOAD_DEBOUNCE_MS = 250;

// FETCH_CONCURRENCY bounds how many run detail fetches are in flight at once.
// A store with hundreds of runs would otherwise fire hundreds of parallel
// requests on load, which (competing with in-flight runs) overwhelm the server,
// time out, and leave the dashboard empty. Completed runs are immutable, so we
// only ever fetch a run once (see the cache in App) — this cap just paces the
// initial backfill.
const FETCH_CONCURRENCY = 8;

// fetchMissing loads any ids not already in `cache` into it, at most
// FETCH_CONCURRENCY requests in flight. Runs are immutable once complete, so a
// cached run is never refetched — after the first load, a reload only pulls the
// handful of newly-completed runs instead of the whole store.
async function fetchMissing(
  ids: string[],
  cache: Map<string, RunResult>,
  getResult: (id: string) => Promise<RunResult | undefined>,
): Promise<void> {
  const missing = ids.filter((id) => !cache.has(id));
  let next = 0;
  const worker = async () => {
    while (next < missing.length) {
      const id = missing[next++];
      const r = await getResult(id).catch(() => null);
      if (r) cache.set(id, r);
    }
  };
  await Promise.all(
    Array.from({ length: Math.min(FETCH_CONCURRENCY, missing.length) }, worker),
  );
}

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
          <Card style={{ maxWidth: 512, width: "100%", textAlign: "center" }}>
            <h2
              style={{
                font: "var(--fw-semibold) var(--text-size-h4)/1.3 var(--font-sans)",
                color: "var(--status-error-text)",
                margin: "0 0 8px",
              }}
            >
              Something went wrong
            </h2>
            <p style={{ font: "var(--text-size-body-sm) var(--font-sans)", color: "var(--text-muted)", margin: "0 0 24px" }}>
              {this.state.error.message}
            </p>
            <Button variant="secondary" onClick={() => this.setState({ error: null })}>
              Try again
            </Button>
          </Card>
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
  // selectedRunId is the current / last-viewed run (from a ledger row). It
  // outlives the transcript view so the ledger keeps that row highlighted after
  // you return — "here's where you were". transcriptOpen gates the transcript
  // page itself; closing it (Back) leaves selectedRunId set for the highlight.
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [transcriptOpen, setTranscriptOpen] = useState(false);
  // Scenarios are multi-select (all selected by default once loaded); a field
  // run covers every selected scenario.
  const [selectedScenarios, setSelectedScenarios] = useState<string[]>([]);
  // The ledger's scenario×provider scope, set by clicking an (aggregate) matrix
  // cell to drill into the individual runs behind it.
  const [cellFilter, setCellFilter] = useState<{ scenarioId: string; providerId: string } | null>(null);
  // How many times "Run the field" sweeps the grid (each a distinct sweep).
  const [runCount, setRunCount] = useState(1);
  // Workflow tab highlight mode: "live" tracks the scrubber (highlights the
  // state at the current turn); "path" shows the whole route the run took.
  const [wfLive, setWfLive] = useState(true);
  const [startError, setStartError] = useState<string | null>(null);
  const [historicalResults, setHistoricalResults] = useState<RunResult[]>([]);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);
  const [promptpack, setPromptpack] = useState<string | undefined>(undefined);
  const [workflowGraph, setWorkflowGraph] = useState<WorkflowGraph | null>(null);

  // The workflow topology is static for the life of the config — fetched
  // once on mount, same pattern as run-options/config below. The Workflow
  // tab is omitted until this resolves.
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

  // Scenario pills default to ALL scenarios once run-options land. Guarded by a
  // ref so it only seeds once — otherwise deselecting every pill (None) would
  // immediately re-select them all.
  const didSeedScenarios = useRef(false);
  useEffect(() => {
    if (!didSeedScenarios.current && scenarios.length > 0) {
      setSelectedScenarios(scenarios.map((s) => s.id));
      didSeedScenarios.current = true;
    }
  }, [scenarios]);

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

  // Single global AudioPlayer for the run view's "Listen" toggle, rebuilt
  // when the user switches Listen target. Only offered for a still-running
  // run — a completed one has no live audio stream to attach to.
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

  // Cache of fetched runs keyed by RunID. Completed runs are immutable, so once
  // a run is fetched it never needs refetching — this is what keeps a store of
  // hundreds of runs from re-fetching everything on every reload.
  const resultCacheRef = useRef<Map<string, RunResult>>(new Map());

  // Load historical results on mount and after runs complete. A field run of N
  // sweeps completes ~N×cells runs in a burst, so completedCount ticks up
  // rapidly. The reload is DEBOUNCED so those coalesce into one refresh, and it
  // only fetches runs not already cached (bounded concurrency), so a reload is
  // cheap no matter how large the store — the first load backfills the cache,
  // later reloads pull just the new runs. The `stale` guard ensures an
  // out-of-order response can't clobber a fresher one. The live matrix still
  // animates in real time from SSE state, independent of this reload.
  const completedCount = state.completedRunIds.length;
  useEffect(() => {
    let stale = false;
    const timer = setTimeout(async () => {
      const ids = await getResults().catch(() => null);
      if (!ids) return;
      if (ids.length === 0) { if (!stale) setHistoricalResults([]); return; }
      const cache = resultCacheRef.current;
      await fetchMissing(ids, cache, getResult);
      if (stale) return;
      setHistoricalResults(ids.map((id) => cache.get(id)).filter((r): r is RunResult => !!r));
    }, RELOAD_DEBOUNCE_MS);
    return () => { stale = true; clearTimeout(timer); };
  }, [getResults, getResult, completedCount]);

  // Exclude synthetic interactive-chat entries from the runs-tab aggregates.
  const liveRuns = Object.values(state.runs).filter((r) => r.scenario !== "interactive");

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
  // The matrix aggregates every run per scenario×provider — a reliability view
  // across the model's non-determinism. It always shows all runs; individual
  // runs live in the ledger.
  const matrix = useMemo(
    () => buildMatrix(matrixResults, providers, scenarios),
    [matrixResults, providers, scenarios],
  );

  // fieldEstimate projects the spend + wall-clock of the pending "Run the
  // field × N" from each target cell's historical averages — null until there's
  // past data for at least one selected cell.
  const fieldEstimate = useMemo(
    () => estimateFieldRun(matrix, selectedScenarios, runCount),
    [matrix, selectedScenarios, runCount],
  );

  // gridResults scopes the instrument band (trial count, spend, suite trend) to
  // the SAME population as the matrix/gauge: scored trials for the current
  // config's scenarios × providers. This keeps every number consistent — no
  // stray off-config runs inflating the count.
  const gridResults = useMemo(() => {
    const scen = new Set(scenarios.map((s) => s.id));
    const prov = new Set(providers.map((p) => p.id));
    return matrixResults.filter((r) => scen.has(r.ScenarioID) && prov.has(r.ProviderID) && runScored(r));
  }, [matrixResults, scenarios, providers]);

  // The ledger lists individual trials for the current config's grid — same
  // scope as the matrix, so old off-config runs in the out dir don't show a
  // contradictory count.
  const gridHistorical = useMemo(() => {
    const scen = new Set(scenarios.map((s) => s.id));
    const prov = new Set(providers.map((p) => p.id));
    return historicalResults.filter((r) => scen.has(r.ScenarioID) && prov.has(r.ProviderID));
  }, [historicalResults, scenarios, providers]);

  // chartedAt drives the Hero's dateline: the most recent completed run, else
  // null (so the eyebrow omits the dateline rather than claiming "charted
  // today").
  const chartedAt = useMemo(() => {
    let latest = 0;
    for (const r of historicalResults) {
      const t = Date.parse(r.EndTime || r.StartTime || "");
      if (!Number.isNaN(t) && t > latest) latest = t;
    }
    return latest > 0 ? new Date(latest).toISOString() : null;
  }, [historicalResults]);

  // The open run (from a ledger row): the saved RunResult, else a live one.
  const selectedCellRun: RunResult | ActiveRun | undefined = useMemo(() => {
    if (!selectedRunId) return undefined;
    return (
      historicalResults.find((r) => r.RunID === selectedRunId) ??
      liveRuns.find((r) => r.runId === selectedRunId)
    );
  }, [selectedRunId, historicalResults, liveRuns]);

  // Clicking a matrix cell (an aggregate) scopes the ledger to that
  // scenario×provider's individual runs — a drill from the rate to the runs
  // behind it. It does NOT open a single run.
  const handleSelectCell = useCallback((key: string) => {
    const cell = matrix.rows.flatMap((row) => row.cells).find((c) => c.key === key);
    if (cell?.hasData) setCellFilter({ scenarioId: cell.scenarioId, providerId: cell.providerId });
  }, [matrix]);
  const clearCellFilter = useCallback(() => setCellFilter(null), []);

  // Ledger row → open that run's transcript.
  const handleSelectRun = useCallback((runId: string) => {
    setSelectedRunId(runId);
    setTranscriptOpen(true);
  }, []);

  // Close the transcript, back to the dashboard — but keep selectedRunId so the
  // ledger still shows which row you were on.
  const handleBackFromInspector = useCallback(() => setTranscriptOpen(false), []);

  // ledgerRunIds is the run order the ledger shows (scoped by the cell filter,
  // newest first) — the sequence the run-details back/next stepper walks, so the
  // stepper and the ledger stay in lockstep.
  const ledgerRunIds = useMemo(
    () => orderLedgerRows(gridHistorical, cellFilter).map((r) => r.RunID),
    [gridHistorical, cellFilter],
  );
  const ledgerIndex = selectedRunId ? ledgerRunIds.indexOf(selectedRunId) : -1;
  const stepRun = useCallback(
    (delta: number) => {
      if (ledgerIndex < 0) return;
      const next = ledgerRunIds[ledgerIndex + delta];
      if (next) setSelectedRunId(next);
    },
    [ledgerIndex, ledgerRunIds],
  );

  // Scenario pill toggles.
  const toggleScenario = useCallback((id: string) => {
    setSelectedScenarios((cur) => (cur.includes(id) ? cur.filter((s) => s !== id) : [...cur, id]));
  }, []);
  const selectAllScenarios = useCallback(() => setSelectedScenarios(scenarios.map((s) => s.id)), [scenarios]);
  const selectNoScenarios = useCallback(() => setSelectedScenarios([]), []);

  // handleStartRun kicks off a run for an arbitrary set of provider/scenario
  // ids — shared by "Run the field" (all providers) and a single matrix-cell
  // run (one provider × one scenario). Stays on the dashboard so the new runs
  // stream into the matrix in place.
  const handleStartRun = useCallback(async (providerIds: string[], scenarioIds: string[], runs = 1) => {
    setStartError(null);
    setTranscriptOpen(false);
    setSelectedRunId(null);
    try {
      await startRun({ providers: providerIds, scenarios: scenarioIds, runs });
    } catch (err) {
      setStartError(err instanceof Error ? err.message : "Failed to start run");
    }
  }, [startRun]);

  // Backs the CommandStrip's "Run the field" — runs EVERY selected scenario
  // across EVERY configured provider, runCount times (each a distinct sweep;
  // real providers are billed, that's intended).
  const handleRunTrial = useCallback(() => {
    if (selectedScenarios.length === 0 || providers.length === 0) return;
    void handleStartRun(providers.map((p) => p.id), selectedScenarios, runCount);
  }, [selectedScenarios, providers, handleStartRun, runCount]);

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
        {/* Atlas Tabs rather than a hand-rolled strip: it brings the
            role="tablist" / aria-selected / roving-tabindex keyboard nav the
            local version never had. */}
        <div style={{ marginBottom: 24 }}>
          <Tabs
            tabs={[
              { id: "runs", label: "Runs" },
              { id: "chat", label: "Interactive Chat" },
            ]}
            value={activeTab}
            onChange={(id) => setActiveTab(id as "runs" | "chat")}
          />
        </div>

        {activeTab === "chat" ? (
          <InteractiveChat
            state={state}
            registerInteractiveRun={registerInteractiveRun}
            onBack={() => setActiveTab("runs")}
          />
        ) : (
          <>
            <div className="transition-[margin] duration-200">
              {transcriptOpen ? (
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <button
                      onClick={handleBackFromInspector}
                      className="flex items-center gap-2 text-sm hover:underline"
                      style={{ color: "var(--text-link)" }}
                    >
                      <ArrowLeft className="h-4 w-4" /> Back to ledger
                    </button>
                    {ledgerIndex >= 0 && ledgerRunIds.length > 1 && (
                      <div className="flex items-center gap-3">
                        <button
                          type="button"
                          onClick={() => stepRun(-1)}
                          disabled={ledgerIndex <= 0}
                          aria-label="previous run"
                          className="flex items-center gap-1 text-sm disabled:opacity-40 disabled:cursor-not-allowed hover:underline"
                          style={{ color: "var(--text-link)" }}
                        >
                          <ArrowLeft className="h-4 w-4" /> Prev
                        </button>
                        <span style={{ font: "12px var(--font-mono)", color: "var(--star-700)" }}>
                          {ledgerIndex + 1} of {ledgerRunIds.length}
                        </span>
                        <button
                          type="button"
                          onClick={() => stepRun(1)}
                          disabled={ledgerIndex >= ledgerRunIds.length - 1}
                          aria-label="next run"
                          className="flex items-center gap-1 text-sm disabled:opacity-40 disabled:cursor-not-allowed hover:underline"
                          style={{ color: "var(--text-link)" }}
                        >
                          Next <ArrowRight className="h-4 w-4" />
                        </button>
                      </div>
                    )}
                  </div>
                  {selectedCellRun ? (
                    (() => {
                      // One path for both shapes. Live and completed runs
                      // render through the same SessionReview — the adapter
                      // owns the shape difference.
                      const a = adaptAnyRun(selectedCellRun);
                      const hasWorkflow = !!(workflowGraph && workflowGraph.nodes.length);
                      // Live audio only exists while a run is in flight; a
                      // completed run has no stream to attach to.
                      const liveRunId =
                        !isRunResult(selectedCellRun) && selectedCellRun.status === "running"
                          ? selectedCellRun.runId
                          : undefined;
                      return (
                        <div style={{ height: "calc(100vh - 210px)", minHeight: 460 }}>
                          {liveRunId && (
                            <div className="mb-2 flex justify-end">
                              <button
                                onClick={() => handleListen(liveRunId)}
                                className="rounded-lg border border-mist bg-surface px-3 py-1.5 text-xs font-medium text-fg-muted hover:text-fg hover:bg-surface-2 transition-colors"
                              >
                                {listeningRunId === liveRunId ? "Stop listening" : "Listen"}
                              </button>
                            </div>
                          )}
                          <SessionReview
                            title={a.title}
                            messages={a.messages}
                            checks={a.checks}
                            recording={a.recording}
                            inspectorTabs={arenaInspectorTabs}
                            tabs={
                              hasWorkflow
                                ? [{
                                    id: "workflow",
                                    label: "Workflow",
                                    render: (ctx) => {
                                      const { current, previous } = currentWorkflowStateAt(selectedCellRun, ctx.activeIndex);
                                      const overlaid = wfLive
                                        ? overlayWorkflowCurrentState(workflowGraph!, current, previous)
                                        : overlayWorkflowRun(workflowGraph!, selectedCellRun);
                                      const g = adaptWorkflow(overlaid);
                                      return (
                                        <div style={{ display: "flex", flexDirection: "column", height: "100%", gap: 8 }}>
                                          <div style={{ display: "flex", alignItems: "center", gap: 6, flex: "none" }}>
                                            <span style={{ font: "var(--text-size-mono-label) var(--font-mono)", textTransform: "uppercase", letterSpacing: "var(--tracking-eyebrow)", color: "var(--star-800)" }}>
                                              Highlight
                                            </span>
                                            {([["live", "Live"], ["path", "Path"]] as const).map(([mode, label]) => {
                                              const active = (mode === "live") === wfLive;
                                              return (
                                                <button
                                                  key={mode}
                                                  type="button"
                                                  onClick={() => setWfLive(mode === "live")}
                                                  title={mode === "live" ? "Highlight the state at the current point in the conversation (follows the scrubber)" : "Highlight the whole path the run took"}
                                                  style={{
                                                    cursor: "pointer",
                                                    padding: "2px 10px",
                                                    borderRadius: "999px",
                                                    font: "500 12px var(--font-sans)",
                                                    background: active ? "var(--starlight-tint)" : "transparent",
                                                    border: `1px solid ${active ? "var(--starlight-300)" : "var(--hairline)"}`,
                                                    color: active ? "var(--star-100)" : "var(--star-600)",
                                                  }}
                                                >
                                                  {label}
                                                </button>
                                              );
                                            })}
                                          </div>
                                          <div style={{ flex: 1, minHeight: 0 }}>
                                            <ConstellationGraph nodes={g.nodes} edges={g.edges} theme={theme} direction="LR" height="100%" />
                                          </div>
                                        </div>
                                      );
                                    },
                                  }]
                                : undefined
                            }
                          />
                        </div>
                      );
                    })()
                  ) : (
                    <div className="rounded-xl border border-mist bg-surface px-4 py-6 text-sm text-fg-muted">
                      Loading trial…
                    </div>
                  )}
                </div>
              ) : (
                <div className="space-y-8">
                  <Hero scenarioCount={scenarios.length} providerCount={providers.length} chartedAt={chartedAt} />
                  <CommandStrip
                    scenarios={scenarios}
                    selected={selectedScenarios}
                    onToggle={toggleScenario}
                    onSelectAll={selectAllScenarios}
                    onSelectNone={selectNoScenarios}
                    providerCount={providers.length}
                    runCount={runCount}
                    onRunCountChange={setRunCount}
                    onRunTrial={handleRunTrial}
                    estimate={fieldEstimate}
                    runDisabled={!state.connected || loading || selectedScenarios.length === 0 || providers.length === 0}
                  />
                  {startError && (
                    <div
                      style={{
                        borderRadius: "var(--radius-xl)",
                        background: "var(--status-error-tint)",
                        border: "1px solid color-mix(in srgb, var(--signal-red) 30%, transparent)",
                        padding: "12px 16px",
                        font: "var(--text-size-body-sm) var(--font-sans)",
                        color: "var(--status-error-text)",
                      }}
                    >
                      {startError}
                    </div>
                  )}
                  <InstrumentBand matrix={matrix} results={gridResults} />
                  <TrialMatrix
                    matrix={matrix}
                    selectedKey={cellFilter ? `${cellFilter.scenarioId}:${cellFilter.providerId}` : null}
                    onSelect={handleSelectCell}
                    onRunCell={handleRunCell}
                  />
                  <HistoricalResults
                    results={gridHistorical}
                    onSelectRun={handleSelectRun}
                    selectedRunId={selectedRunId}
                    filter={cellFilter}
                    onClearFilter={clearCellFilter}
                    onClear={async () => {
                      await clearResults();
                      resultCacheRef.current.clear();
                      setHistoricalResults([]);
                      setCellFilter(null);
                      setSelectedRunId(null);
                    }}
                  />
                </div>
              )}
            </div>
          </>
        )}
        </main>
      </div>
    </ErrorBoundary>
  );
}
