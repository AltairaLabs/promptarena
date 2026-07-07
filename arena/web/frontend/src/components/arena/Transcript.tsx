import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { TranscriptMessage } from "@/types";

export interface TranscriptProps {
  messages: TranscriptMessage[];
}

// Transcript — the Atlas Trial Inspector's left pane: role-accented turn
// cards (3px left border + faint tint per role), tool calls rendered as
// ink-void JSON cards, and pass/fail assertion chips. Purely presentational;
// the viewmodel is built upstream by `buildTranscript` in `lib/arenaView.ts`.
export function Transcript({ messages }: TranscriptProps) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
      {messages.map((m) => (
        <TranscriptCard key={m.idx} message={m} />
      ))}
    </div>
  );
}

function TranscriptCard({ message: m }: { message: TranscriptMessage }) {
  return (
    <div
      style={{
        borderLeft: `3px solid ${m.accent}`,
        background: m.bg,
        borderRadius: 10,
        padding: "11px 14px",
      }}
    >
      <div style={{ display: "flex", gap: 9, alignItems: "baseline", marginBottom: 5 }}>
        <span
          style={{
            textTransform: "uppercase",
            font: "700 10px var(--font-mono)",
            letterSpacing: "0.14em",
            color: m.accent,
          }}
        >
          {m.role}
        </span>
        <span style={{ font: "400 10px var(--font-mono)", color: "var(--star-950)" }}>#{m.idx}</span>
        {m.meta && (
          <span style={{ marginLeft: "auto", font: "500 11px var(--font-mono)", color: "var(--amber-500)" }}>
            {m.meta}
          </span>
        )}
      </div>

      {m.content && (
        <div
          className="markdown-message"
          style={{ font: "400 13.5px/1.55 var(--font-sans)", color: "var(--star-300)" }}
        >
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{m.content}</ReactMarkdown>
        </div>
      )}

      {m.tool && (
        <div
          style={{
            marginTop: 9,
            border: "1px solid var(--hairline)",
            borderRadius: 8,
            background: "var(--ink-void)",
            padding: "9px 11px",
          }}
        >
          <div style={{ font: "600 11px var(--font-mono)", color: "var(--ion-cyan)" }}>⚙ {m.tool.name}</div>
          <pre
            style={{
              font: "400 11px/1.5 var(--font-mono)",
              color: "var(--star-500)",
              whiteSpace: "pre-wrap",
              margin: 0,
            }}
          >
            {m.tool.body}
          </pre>
        </div>
      )}

      {m.asserts && m.asserts.length > 0 && (
        <div style={{ marginTop: 9, display: "flex", flexWrap: "wrap", gap: 6 }}>
          {m.asserts.map((a) => (
            <span
              key={a.name}
              style={{
                font: "600 10px var(--font-mono)",
                padding: "4px 8px",
                borderRadius: 6,
                color: a.ok ? "var(--pulsar-300)" : "var(--signal-red-300)",
                background: a.ok
                  ? "color-mix(in srgb, var(--pulsar-500) 16%, transparent)"
                  : "color-mix(in srgb, var(--signal-red) 16%, transparent)",
              }}
            >
              {a.ok ? "✓" : "✗"} {a.name}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
