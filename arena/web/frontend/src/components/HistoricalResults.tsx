import { useMemo } from "react";
import { DataTable, Button, type DataTableColumn } from "@altairalabs/atlas";
import type { RunResult } from "@/types";

interface HistoricalResultsProps {
  results: RunResult[];
  onSelectRun: (id: string) => void;
  onClear: () => void | Promise<void>;
}

const PAGE_SIZE = 25;

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

// failureBlurb summarises why a run failed in a one-liner suitable for a
// table cell: prefer assertion counts when available, fall back to the
// first chunk of the error message.
function failureBlurb(r: RunResult): string {
  if (r.ConversationAssertions && r.ConversationAssertions.failed > 0) {
    const f = r.ConversationAssertions.failed;
    const total = r.ConversationAssertions.total;
    return `${f}/${total} assertion${total === 1 ? "" : "s"} failed`;
  }
  if (r.Error) {
    return r.Error.length > 80 ? r.Error.slice(0, 80) + "…" : r.Error;
  }
  return "Failed";
}

// runTime returns the canonical timestamp for sorting/display.
function runTime(r: RunResult): number {
  return new Date(r.EndTime || r.StartTime).getTime();
}

// runPassed treats Error + failed assertions as Fail; everything else
// (including missing assertion summary) is Pass.
function runPassed(r: RunResult): boolean {
  return !r.Error && (r.ConversationAssertions?.passed ?? true);
}

// getSearchText backs DataTable's built-in filter — same fields the old
// hand-rolled filter matched on: scenario, provider, region and error.
function getSearchText(r: RunResult): string {
  return [r.ScenarioID, r.ProviderID, r.Region ?? "", r.Error ?? ""].join(" ");
}

// Column config for the package DataTable. Each column carries its own
// header, sort value and cell renderer so the domain shape (RunResult)
// never leaks into the shared table. Sort values mirror the old `compare`
// helper; strings are lower-cased for case-insensitive ordering and the
// Result column maps pass→0 / fail→1 so passes sort first on ascending.
const columns: DataTableColumn<RunResult>[] = [
  {
    key: "scenario",
    header: "Scenario",
    sortable: true,
    sortValue: (r) => r.ScenarioID.toLowerCase(),
    render: (r) => <span className="font-medium text-fg">{r.ScenarioID}</span>,
  },
  {
    key: "provider",
    header: "Provider",
    sortable: true,
    sortValue: (r) => r.ProviderID.toLowerCase(),
    render: (r) => <span className="text-fg-muted">{r.ProviderID}</span>,
  },
  {
    key: "region",
    header: "Region",
    sortable: true,
    sortValue: (r) => (r.Region ?? "").toLowerCase(),
    render: (r) => <span className="text-fg-muted">{r.Region}</span>,
  },
  {
    key: "result",
    header: "Result",
    sortable: true,
    sortValue: (r) => (runPassed(r) ? 0 : 1),
    render: (r) => {
      const passed = runPassed(r);
      return (
        <div className="flex flex-col gap-0.5">
          {/* Atlas splits status fill from status text on purpose — the dot
              uses the saturated fill, the label the contrast-tuned text
              variant. Mapping both to one token would lose that tuning. */}
          <span
            className="inline-flex items-center gap-1.5"
            style={{
              font: "var(--fw-semibold) var(--text-size-mono-xs) var(--font-mono)",
              color: passed ? "var(--status-healthy-text)" : "var(--status-error-text)",
            }}
          >
            <span
              className="h-1.5 w-1.5 rounded-full"
              style={{ background: passed ? "var(--status-healthy)" : "var(--status-error)" }}
            />
            {passed ? "Pass" : "Fail"}
          </span>
          {!passed && (
            <span className="text-[10px] text-fg-muted truncate max-w-[260px]" title={r.Error}>
              {failureBlurb(r)}
            </span>
          )}
        </div>
      );
    },
  },
  {
    key: "cost",
    header: "Cost",
    align: "right",
    sortable: true,
    sortValue: (r) => r.Cost?.total_cost_usd ?? 0,
    render: (r) => (
      <span className="font-mono text-fg-muted">${r.Cost?.total_cost_usd?.toFixed(4) ?? "0.0000"}</span>
    ),
  },
  {
    key: "msgs",
    header: "Msgs",
    align: "right",
    sortable: true,
    sortValue: (r) => r.Messages?.length ?? 0,
    render: (r) => <span className="text-fg-muted">{r.Messages?.length ?? 0}</span>,
  },
  {
    key: "when",
    header: "When",
    align: "right",
    sortable: true,
    sortValue: (r) => runTime(r),
    render: (r) => (
      <span className="text-fg-muted">
        {r.EndTime ? timeAgo(r.EndTime) : r.StartTime ? timeAgo(r.StartTime) : "—"}
      </span>
    ),
  },
];

export function HistoricalResults({ results, onSelectRun, onClear }: HistoricalResultsProps) {
  // Default to newest-first, matching the old table's initial "when desc"
  // ordering. DataTable has no initial-sort prop, so we pre-sort the rows
  // it receives; clicking a header still re-sorts via each column's
  // sortValue.
  const rows = useMemo(() => [...results].sort((a, b) => runTime(b) - runTime(a)), [results]);

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-xs font-semibold text-fg-muted uppercase tracking-wider whitespace-nowrap">
          Previous Runs
          <span className="ml-2 rounded-full bg-surface-2 text-fg-muted px-2 py-0.5 text-[10px] font-mono normal-case tracking-normal">
            {results.length}
          </span>
        </h3>
        <Button variant="danger" size="sm" onClick={onClear}>
          Clear all
        </Button>
      </div>

      {results.length === 0 ? (
        <div className="rounded-xl border border-mist bg-surface px-6 py-8 text-center text-xs text-fg-muted">
          No runs yet.
        </div>
      ) : (
        <DataTable<RunResult>
          columns={columns}
          rows={rows}
          rowKey={(r) => r.RunID}
          pageSize={PAGE_SIZE}
          searchable
          getSearchText={getSearchText}
          onRowClick={(r) => onSelectRun(r.RunID)}
          empty="No runs match your search."
        />
      )}
    </div>
  );
}
