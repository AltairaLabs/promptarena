import { Transcript } from "./Transcript";
import { AgentFlowCard } from "./AgentFlowCard";
import { TerminalCard } from "./TerminalCard";
import { StatusPill } from "@/components/atlas/StatusPill";
import { buildTranscript, buildAgentFlow, buildTerminalLines } from "@/lib/arenaView";
import type { RunResult, ActiveRun, TrialCell, Message } from "@/types";

export interface TrialInspectorProps {
  run: RunResult | ActiveRun | undefined;
  cell: TrialCell | undefined;
  scenarioId: string;
  providerId: string;
  providerLabel: string;
  // onSelectMessage, when provided, is invoked when a transcript message is
  // clicked — mirrors the old RunDetail/ConversationThread contract so the
  // DevTools drawer stays reachable from the Runs tab.
  onSelectMessage?: (index: number, message?: Message, allMessages?: Message[]) => void;
  // listeningRunId/onToggleListen surface the audio "Listen" control for a
  // live running trial — mirrors the original RunDetail wiring. Omitted (or
  // undefined onToggleListen) hides the toggle entirely, and it's only ever
  // shown for a still-running ActiveRun (a completed/historical run has no
  // live audio stream to attach to).
  listeningRunId?: string | null;
  onToggleListen?: (runId: string) => void;
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
export function TrialInspector({
  run,
  cell,
  scenarioId,
  providerId,
  providerLabel,
  onSelectMessage,
  listeningRunId,
  onToggleListen,
}: TrialInspectorProps) {
  const { status, label } = statusFor(run, cell);

  // The Listen toggle only makes sense for a still-running ActiveRun — a
  // completed RunResult has no live audio stream to attach to.
  const isLiveRunning = run !== undefined && "status" in run && run.status === "running";
  const liveRunId = isLiveRunning ? (run as ActiveRun).runId : undefined;
  const isListening = liveRunId !== undefined && listeningRunId === liveRunId;

  // handleSelectMessage adapts the transcript's index-only click into the
  // (index, message, allMessages) triple DevToolsPanel expects. A completed
  // RunResult carries the full Message[] the DevTools tabs need; a still-
  // running ActiveRun has no Message[] to offer, so only the index is passed
  // — DevToolsPanel tolerates an undefined message/allMessages.
  const handleSelectMessage = (idx: number) => {
    if (run && "Messages" in run && Array.isArray(run.Messages)) {
      onSelectMessage?.(idx, run.Messages[idx], run.Messages);
    } else {
      onSelectMessage?.(idx);
    }
  };

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
          <span style={{ font: "400 12px var(--font-mono)", color: "var(--star-800)" }}>·</span>
          <span style={{ font: "500 12px var(--font-mono)", color: "var(--star-500)" }}>{scenarioId}</span>
          <span style={{ font: "400 12px var(--font-mono)", color: "var(--star-800)" }}>· {providerLabel}</span>
          <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 }}>
            {liveRunId !== undefined && onToggleListen && (
              <button
                onClick={() => onToggleListen(liveRunId)}
                style={{
                  font: "500 11px var(--font-mono)",
                  color: isListening ? "var(--gold-300)" : "var(--star-600)",
                  background: "transparent",
                  border: "1px solid var(--hairline-strong)",
                  borderRadius: "var(--radius-sm)",
                  padding: "6px 10px",
                  cursor: "pointer",
                }}
              >
                {isListening ? "🔇 Listening" : "🔊 Listen"}
              </button>
            )}
            <StatusPill status={status}>{label}</StatusPill>
          </div>
        </div>
        <div style={{ padding: "16px 18px", maxHeight: 560, overflowY: "auto" }}>
          <Transcript messages={buildTranscript(run)} onSelectMessage={onSelectMessage ? handleSelectMessage : undefined} />
        </div>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <AgentFlowCard flow={buildAgentFlow(run)} />
        <TerminalCard lines={buildTerminalLines(cell, scenarioId, providerId)} />
      </div>
    </div>
  );
}
