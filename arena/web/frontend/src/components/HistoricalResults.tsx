import { useMemo, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Card, Button } from "@altairalabs/atlas";
import { orderLedgerRows, runOutcome } from "@/lib/arenaView";
import type { RunResult } from "@/types";

// PAGE_SIZE bounds how many trial rows the ledger renders at once — with
// hundreds of trials, rendering them all is unusable, so the rest paginate.
const PAGE_SIZE = 25;

type OutcomeFilter = "all" | "pass" | "fail" | "error";

interface HistoricalResultsProps {
  results: RunResult[];
  // Open one run's transcript.
  onSelectRun: (runId: string) => void;
  onClear: () => void | Promise<void>;
  // When a matrix cell is clicked, the ledger scopes to that scenario×provider.
  filter?: { scenarioId: string; providerId: string } | null;
  onClearFilter?: () => void;
  // The run currently open in the transcript view, if any — its row is
  // highlighted so returning from a run makes clear where you were.
  selectedRunId?: string | null;
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return `${Math.floor(days / 30)}mo ago`;
}

const eyebrowStyle: React.CSSProperties = {
  fontFamily: "var(--font-mono)",
  fontSize: "var(--text-size-mono-label)",
  fontWeight: "var(--fw-medium)",
  textTransform: "uppercase",
  letterSpacing: "var(--tracking-eyebrow)",
  color: "var(--star-900)",
};

const selectStyle: React.CSSProperties = {
  cursor: "pointer",
  borderRadius: "var(--radius-sm)",
  border: "1px solid var(--hairline)",
  background: "var(--surface-2)",
  color: "var(--star-300)",
  font: "12px var(--font-mono)",
  padding: "4px 8px",
};

const OUTCOME: Record<"pass" | "fail" | "error", { label: string; color: string; dot: string }> = {
  pass: { label: "Pass", color: "var(--status-healthy-text)", dot: "var(--status-healthy)" },
  fail: { label: "Fail", color: "var(--status-error-text)", dot: "var(--status-error)" },
  error: { label: "Error", color: "var(--status-error-text)", dot: "var(--signal-red)" },
};

// A single run (trial) row. The whole row opens that run's transcript. The
// currently-open run's row is highlighted (a left rule + tint) so returning
// from the transcript makes clear which row you came from.
function RunRow({ run, onClick, selected }: { run: RunResult; onClick: () => void; selected: boolean }) {
  const o = OUTCOME[runOutcome(run)];
  const when = run.EndTime || run.StartTime;
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={selected ? "true" : undefined}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 14,
        width: "100%",
        textAlign: "left",
        cursor: "pointer",
        border: "none",
        borderTop: "1px solid var(--hairline-faint)",
        boxShadow: selected ? "inset 3px 0 0 var(--starlight-400)" : "none",
        padding: "10px 16px",
        background: selected ? "var(--starlight-tint)" : "transparent",
      }}
    >
      <span style={{ display: "inline-flex", alignItems: "center", gap: 6, width: 64, flex: "none", font: "600 12px var(--font-mono)", color: o.color }}>
        <span style={{ width: 7, height: 7, borderRadius: "50%", background: o.dot, flex: "none" }} />
        {o.label}
      </span>
      <span style={{ flex: 1, font: "500 13px var(--font-sans)", color: "var(--star-300)" }}>{run.ScenarioID}</span>
      <span style={{ flex: 1, font: "13px var(--font-sans)", color: "var(--star-600)" }}>{run.ProviderID}</span>
      <span style={{ font: "12px var(--font-mono)", color: "var(--star-700)", width: 72, textAlign: "right", flex: "none" }}>
        {run.Cost?.total_cost_usd ? `$${run.Cost.total_cost_usd.toFixed(3)}` : "—"}
      </span>
      <span style={{ font: "12px var(--font-mono)", color: "var(--star-800)", width: 64, textAlign: "right", flex: "none" }}>
        {when ? timeAgo(when) : "—"}
      </span>
    </button>
  );
}

// The ledger — a collapsible, filterable, paginated list of individual runs
// (trials), newest first. Always present; the chevron just folds the body.
// Clicking a run opens its transcript. A matrix-cell click scopes it to that
// scenario×provider; the outcome/scenario/provider dropdowns narrow it further.
export function HistoricalResults({
  results,
  onSelectRun,
  onClear,
  filter,
  onClearFilter,
  selectedRunId,
}: HistoricalResultsProps) {
  const [collapsed, setCollapsed] = useState(false);
  const [outcome, setOutcome] = useState<OutcomeFilter>("all");
  const [scenario, setScenario] = useState("");
  const [provider, setProvider] = useState("");

  // Dropdown options come from the whole result set (not the current filtered
  // view) so narrowing on one axis never hides choices on another.
  const scenarioOptions = useMemo(
    () => [...new Set(results.map((r) => r.ScenarioID))].sort((a, b) => a.localeCompare(b)),
    [results],
  );
  const providerOptions = useMemo(
    () => [...new Set(results.map((r) => r.ProviderID))].sort((a, b) => a.localeCompare(b)),
    [results],
  );

  // rows = cell-scoped + newest-first (orderLedgerRows), then narrowed by the
  // outcome/scenario/provider dropdowns.
  const rows = useMemo(() => {
    return orderLedgerRows(results, filter).filter(
      (r) =>
        (scenario === "" || r.ScenarioID === scenario) &&
        (provider === "" || r.ProviderID === provider) &&
        (outcome === "all" || runOutcome(r) === outcome),
    );
  }, [results, filter, scenario, provider, outcome]);

  // Start on the page holding the currently-open run so returning from a
  // transcript lands on its row (the ledger remounts on return). Recomputed
  // only at mount — user paging afterwards is preserved.
  const [page, setPage] = useState(() => {
    if (!selectedRunId) return 0;
    const idx = orderLedgerRows(results, filter).findIndex((r) => r.RunID === selectedRunId);
    return idx >= 0 ? Math.floor(idx / PAGE_SIZE) : 0;
  });

  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount - 1);
  const start = safePage * PAGE_SIZE;
  const pageRows = rows.slice(start, start + PAGE_SIZE);

  // Any filter change collapses back to the first page.
  const resetPage = () => setPage(0);
  const Chevron = collapsed ? ChevronRight : ChevronDown;

  return (
    <Card padding={0} style={{ overflow: "hidden" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 12,
          padding: "12px 16px",
          borderBottom: collapsed ? "none" : "1px solid var(--hairline)",
        }}
      >
        <button
          type="button"
          onClick={() => setCollapsed((c) => !c)}
          aria-expanded={!collapsed}
          style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", background: "transparent", border: "none", padding: 0 }}
        >
          <Chevron className="h-4 w-4" style={{ color: "var(--star-700)" }} />
          <span style={eyebrowStyle}>Ledger</span>
          <span style={{ borderRadius: "999px", background: "var(--surface-2)", color: "var(--star-700)", padding: "1px 8px", font: "10px var(--font-mono)" }}>
            {rows.length}
          </span>
        </button>
        {filter && (
          <button
            type="button"
            onClick={onClearFilter}
            style={{ cursor: "pointer", background: "transparent", border: "none", font: "12px var(--font-mono)", color: "var(--text-link)" }}
          >
            {filter.scenarioId} × {filter.providerId} · clear
          </button>
        )}
        <span style={{ flex: 1 }} />
        {results.length > 0 && (
          <Button variant="danger" size="sm" onClick={onClear}>
            Clear all
          </Button>
        )}
      </div>

      {!collapsed && (
        <>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              flexWrap: "wrap",
              padding: "10px 16px",
              borderBottom: "1px solid var(--hairline-faint)",
            }}
          >
            <select
              aria-label="filter by outcome"
              value={outcome}
              onChange={(e) => { setOutcome(e.target.value as OutcomeFilter); resetPage(); }}
              style={selectStyle}
            >
              <option value="all">All outcomes</option>
              <option value="pass">Passed</option>
              <option value="fail">Failed</option>
              <option value="error">Errored</option>
            </select>
            <select
              aria-label="filter by scenario"
              value={scenario}
              onChange={(e) => { setScenario(e.target.value); resetPage(); }}
              style={selectStyle}
            >
              <option value="">All scenarios</option>
              {scenarioOptions.map((s) => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
            <select
              aria-label="filter by contender"
              value={provider}
              onChange={(e) => { setProvider(e.target.value); resetPage(); }}
              style={selectStyle}
            >
              <option value="">All contenders</option>
              {providerOptions.map((p) => (
                <option key={p} value={p}>{p}</option>
              ))}
            </select>
          </div>

          {rows.length === 0 ? (
            <div style={{ padding: "24px 16px", textAlign: "center", font: "12px var(--font-mono)", color: "var(--star-800)" }}>
              No runs yet.
            </div>
          ) : (
            <>
              <div>
                {pageRows.map((r) => (
                  <RunRow
                    key={r.RunID}
                    run={r}
                    onClick={() => onSelectRun(r.RunID)}
                    selected={r.RunID === selectedRunId}
                  />
                ))}
              </div>

              {rows.length > PAGE_SIZE && (
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: 12,
                    padding: "10px 16px",
                    borderTop: "1px solid var(--hairline)",
                  }}
                >
                  <span style={{ font: "12px var(--font-mono)", color: "var(--star-700)" }}>
                    {start + 1}–{Math.min(start + PAGE_SIZE, rows.length)} of {rows.length}
                  </span>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => setPage(safePage - 1)}
                      disabled={safePage <= 0}
                    >
                      Prev
                    </Button>
                    <span style={{ font: "12px var(--font-mono)", color: "var(--star-800)" }}>
                      {safePage + 1} / {pageCount}
                    </span>
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => setPage(safePage + 1)}
                      disabled={safePage >= pageCount - 1}
                    >
                      Next
                    </Button>
                  </span>
                </div>
              )}
            </>
          )}
        </>
      )}
    </Card>
  );
}
