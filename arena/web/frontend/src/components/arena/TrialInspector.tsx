import { Transcript } from "./Transcript";
import { AgentFlowCard } from "./AgentFlowCard";
import { TerminalCard } from "./TerminalCard";
import { StatusPill } from "@/components/atlas/StatusPill";
import { buildTranscript, buildAgentFlow, buildTerminalLines } from "@/lib/arenaView";
import type { RunResult, ActiveRun, TrialCell } from "@/types";

export interface TrialInspectorProps {
  run: RunResult | ActiveRun | undefined;
  cell: TrialCell | undefined;
  scenarioId: string;
  providerId: string;
  providerLabel: string;
}

// statusFor derives the header StatusPill's status/label. A still-running
// ActiveRun takes priority over the cell's (necessarily stale, since the
// cell only reflects completed runs) pass/fail reading.
function statusFor(
  run: RunResult | ActiveRun | undefined,
  cell: TrialCell | undefined,
): { status: "running" | "reconciled" | "error"; label: string } {
  const isRunning = run !== undefined && "status" in run && run.status === "running";
  if (isRunning) return { status: "running", label: "Running" };
  if (cell?.passed) return { status: "reconciled", label: "Passed" };
  return { status: "error", label: "Failed" };
}

// TrialInspector — the redesign's three-pane Trial Inspector: transcript
// (left) + agent-flow + terminal (right rail), replacing RunDetail for a
// selected trial cell.
export function TrialInspector({ run, cell, scenarioId, providerId, providerLabel }: TrialInspectorProps) {
  const { status, label } = statusFor(run, cell);

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1.55fr 1fr", gap: 16 }}>
      <div
        style={{
          border: "1px solid var(--hairline)",
          borderRadius: "var(--radius-2xl)",
          background: "var(--grad-surface)",
          overflow: "hidden",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            padding: "14px 18px",
            borderBottom: "1px solid var(--hairline)",
          }}
        >
          <span
            style={{
              font: "11px var(--font-mono)",
              textTransform: "uppercase",
              color: "var(--star-900)",
            }}
          >
            TRANSCRIPT
          </span>
          <span style={{ font: "500 12px var(--font-mono)", color: "var(--star-500)" }}>{scenarioId}</span>
          <span style={{ font: "400 12px var(--font-mono)", color: "var(--star-800)" }}>· {providerLabel}</span>
          <div style={{ marginLeft: "auto" }}>
            <StatusPill status={status}>{label}</StatusPill>
          </div>
        </div>
        <div style={{ padding: "16px 18px", maxHeight: 560, overflowY: "auto" }}>
          <Transcript messages={buildTranscript(run)} />
        </div>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <AgentFlowCard flow={buildAgentFlow(run)} />
        <TerminalCard lines={buildTerminalLines(cell, scenarioId, providerId)} />
      </div>
    </div>
  );
}
