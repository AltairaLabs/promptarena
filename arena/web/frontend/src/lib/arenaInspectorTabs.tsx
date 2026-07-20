// Arena-specific Inspector tabs. The old DevToolsPanel exposed ~14 per-message
// tabs; the Atlas Inspector already covers most of them with its built-ins:
//   • Overview  → info / metrics
//   • Tool      → tool calls
//   • Checks    → assertions / evals / validators (mapped to AtlasCheck)
//   • Raw       → raw message JSON
// The tabs below are the Arena-only diagnostics with NO built-in equivalent —
// each reads a key off `message.meta` (carried through by atlasAdapter) and
// renders it faithfully: structured payloads as a collapsible JSON tree, raw
// prompt/YAML text as literal preformatted text so exact bytes survive.
//
// Each tab is contextual: its `visible` predicate hides it for messages that
// carry no data for it (Chrome-DevTools style), so a message only shows the
// panels that apply. The Empty fallback stays as a defensive belt-and-braces.
import type * as React from "react";
import { JsonView, Markdown, type InspectorSubject, type InspectorTab } from "@altairalabs/atlas";

// metaValue pulls a meta key off a message subject; non-message subjects
// (toolCall / event) never carry Arena meta, so they always read undefined.
function metaValue(subject: InspectorSubject, key: string): unknown {
  return subject.kind === "message" ? subject.message.meta?.[key] : undefined;
}

const emptyStyle: React.CSSProperties = {
  fontFamily: "var(--font-mono)",
  fontSize: "var(--text-size-mono-xs)",
  color: "var(--text-faint)",
};

const preStyle: React.CSSProperties = {
  margin: 0,
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  fontFamily: "var(--font-mono)",
  fontSize: "var(--text-size-mono-micro)",
  lineHeight: 1.55,
  color: "var(--star-400)",
};

function Empty({ label }: { label: string }) {
  return <div style={emptyStyle}>No {label} for this message.</div>;
}

// JsonTab: structured payloads render as a collapsible JSON tree. Some backends
// hand these keys through as an already-serialised string (e.g. a raw HTTP
// body) — render those literally so they stay readable instead of a one-line
// quoted blob.
function JsonPayload({ value }: { value: unknown }) {
  if (typeof value === "string") return <pre style={preStyle}>{value}</pre>;
  return <JsonView value={value} />;
}

function jsonTab(id: string, label: string, key: string, emptyLabel: string): InspectorTab {
  return {
    id,
    label,
    appliesTo: ["message"],
    visible: (subject) => metaValue(subject, key) != null,
    render: (subject) => {
      const value = metaValue(subject, key);
      if (value == null) return <Empty label={emptyLabel} />;
      return <JsonPayload value={value} />;
    },
  };
}

// MarkdownTab: prompt-style text (system prompt, self-play prompt) renders via
// the Markdown leaf since these are usually markdown-authored; a non-string
// value falls back to the JSON tree so nothing is silently dropped.
function markdownTab(id: string, label: string, key: string, emptyLabel: string): InspectorTab {
  return {
    id,
    label,
    appliesTo: ["message"],
    visible: (subject) => metaValue(subject, key) != null,
    render: (subject) => {
      const value = metaValue(subject, key);
      if (value == null) return <Empty label={emptyLabel} />;
      if (typeof value === "string") return <Markdown>{value}</Markdown>;
      return <JsonView value={value} />;
    },
  };
}

// TextTab: literal preformatted text. Used for the persona YAML, where markdown
// rendering would mangle the indentation-significant structure — keep it byte-
// exact so it stays debuggable.
function textTab(id: string, label: string, key: string, emptyLabel: string): InspectorTab {
  return {
    id,
    label,
    appliesTo: ["message"],
    visible: (subject) => metaValue(subject, key) != null,
    render: (subject) => {
      const value = metaValue(subject, key);
      if (value == null) return <Empty label={emptyLabel} />;
      if (typeof value === "string") return <pre style={preStyle}>{value}</pre>;
      return <JsonView value={value} />;
    },
  };
}

// ToolsTab: what the model could call on this turn. `_available_tools` is the
// name list and `_tool_descriptors` the full schemas — show the names as a
// summary with the schemas underneath, since the descriptors are verbose and
// the usual question is just "did it have the tool at all?".
function toolsTab(): InspectorTab {
  const has = (subject: InspectorSubject) =>
    metaValue(subject, "_available_tools") != null || metaValue(subject, "_tool_descriptors") != null;
  return {
    id: "tools",
    label: "Tools",
    appliesTo: ["message"],
    visible: has,
    render: (subject) => {
      const names = metaValue(subject, "_available_tools");
      const descriptors = metaValue(subject, "_tool_descriptors");
      if (names == null && descriptors == null) return <Empty label="tool catalog" />;
      return (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          {Array.isArray(names) && names.length > 0 && (
            <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
              {names.map((n, i) => (
                <span
                  key={`${String(n)}-${i}`}
                  style={{
                    fontFamily: "var(--font-mono)",
                    fontSize: "var(--text-size-mono-xs)",
                    color: "var(--star-500)",
                    background: "var(--surface-2)",
                    border: "1px solid var(--border-default)",
                    borderRadius: "var(--radius-sm)",
                    padding: "2px 7px",
                  }}
                >
                  {String(n)}
                </span>
              ))}
            </div>
          )}
          {descriptors != null && <JsonPayload value={descriptors} />}
        </div>
      );
    },
  };
}

// Order mirrors a request→response→trace debugging flow, then the
// tools/workflow/persona/self-play context that shaped the turn.
//
// The HTML report has long shown a Tools and a Composition tab that the web UI
// did not — `_available_tools`/`_tool_descriptors` appear in roughly half the
// saved runs, so this was a real blind spot rather than a theoretical one. The
// report's `pack_evals` tab is deliberately not mirrored: no saved run carries
// that key, so it would be a tab that never populates.
export const arenaInspectorTabs: InspectorTab[] = [
  markdownTab("prompt", "Prompt", "system_prompt", "system prompt"),
  jsonTab("request", "Request", "_llm_raw_request", "raw request"),
  jsonTab("response", "Response", "_llm_raw_response", "raw response"),
  jsonTab("trace", "Trace", "_llm_trace", "trace"),
  toolsTab(),
  jsonTab("workflow", "Workflow", "_workflow_state", "workflow state"),
  jsonTab("workflow-current", "Workflow State", "current_workflow_state", "current workflow state"),
  jsonTab("composition", "Composition", "_composition_snapshot", "composition snapshot"),
  textTab("persona", "Persona", "_persona_yaml", "persona"),
  markdownTab("selfplay", "Self-Play", "_selfplay_prompt", "self-play prompt"),
];
