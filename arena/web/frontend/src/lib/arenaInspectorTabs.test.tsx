import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import type { AtlasMessage, InspectorSubject } from "@altairalabs/atlas";
import { arenaInspectorTabs } from "./arenaInspectorTabs";

function messageSubject(meta?: Record<string, unknown>): InspectorSubject {
  const message: AtlasMessage = {
    id: "m0",
    role: "assistant",
    sequenceNum: 0,
    timestamp: new Date().toISOString(),
    parts: [{ type: "text", text: "hi" }],
    meta,
  };
  return { kind: "message", message };
}

const byId = (id: string) => {
  const tab = arenaInspectorTabs.find((t) => t.id === id);
  if (!tab) throw new Error(`no tab ${id}`);
  return tab;
};

describe("arenaInspectorTabs", () => {
  it("exposes the Arena-only tabs, all scoped to message subjects", () => {
    expect(arenaInspectorTabs.map((t) => t.id)).toEqual([
      "prompt",
      "request",
      "response",
      "trace",
      "tools",
      "workflow",
      "workflow-current",
      "composition",
      "persona",
      "selfplay",
    ]);
    for (const tab of arenaInspectorTabs) {
      expect(tab.appliesTo).toEqual(["message"]);
    }
  });

  // The HTML report has shown these for a long time; the web UI had no
  // equivalent, and _available_tools/_tool_descriptors appear in roughly half
  // the saved runs — so this was a real blind spot.
  describe("tools tab", () => {
    it("lists the available tool names", () => {
      const subject = messageSubject({ _available_tools: ["calculate", "search"] });
      render(<>{byId("tools").render(subject)}</>);
      expect(screen.getByText("calculate")).toBeInTheDocument();
      expect(screen.getByText("search")).toBeInTheDocument();
    });

    it("shows the descriptors alongside the names", () => {
      const subject = messageSubject({
        _available_tools: ["calculate"],
        _tool_descriptors: [{ description: "Perform mathematical calculations" }],
      });
      render(<>{byId("tools").render(subject)}</>);
      expect(screen.getByText("calculate")).toBeInTheDocument();
      expect(screen.getByText(/Perform mathematical calculations/)).toBeInTheDocument();
    });

    // Either key alone must open the tab — a turn can carry descriptors
    // without the name list.
    it("is visible when only descriptors are present", () => {
      const tab = byId("tools");
      expect(tab.visible?.(messageSubject({ _tool_descriptors: [{ description: "x" }] }))).toBe(true);
      expect(tab.visible?.(messageSubject({ _available_tools: ["a"] }))).toBe(true);
      expect(tab.visible?.(messageSubject({}))).toBe(false);
    });
  });

  it("shows the composition snapshot", () => {
    const subject = messageSubject({ _composition_snapshot: { step: "resolve" } });
    render(<>{byId("composition").render(subject)}</>);
    expect(screen.getByText(/resolve/)).toBeInTheDocument();
  });

  it("shows the current workflow state separately from the workflow tab", () => {
    const subject = messageSubject({ current_workflow_state: { node: "intake" } });
    expect(byId("workflow-current").visible?.(subject)).toBe(true);
    // The pre-existing workflow tab reads a different key and must stay closed.
    expect(byId("workflow").visible?.(subject)).toBe(false);
  });

  it("renders a structured meta payload as JSON (trace)", () => {
    const subject = messageSubject({ _llm_trace: { phase: "planning", steps: 3 } });
    render(<>{byId("trace").render(subject)}</>);
    expect(screen.getByText(/phase/)).toBeInTheDocument();
    expect(screen.getByText(/planning/)).toBeInTheDocument();
  });

  it("renders a raw request string literally", () => {
    const subject = messageSubject({ _llm_raw_request: "POST /v1/chat\n\nbody" });
    render(<>{byId("request").render(subject)}</>);
    expect(screen.getByText(/POST \/v1\/chat/)).toBeInTheDocument();
  });

  it("renders the system prompt text", () => {
    const subject = messageSubject({ system_prompt: "You are a helpful assistant." });
    render(<>{byId("prompt").render(subject)}</>);
    expect(screen.getByText(/You are a helpful assistant\./)).toBeInTheDocument();
  });

  it("renders persona YAML literally", () => {
    const subject = messageSubject({ _persona_yaml: "name: Ada\ntone: terse" });
    render(<>{byId("persona").render(subject)}</>);
    expect(screen.getByText(/name: Ada/)).toBeInTheDocument();
  });

  it("shows a faint empty-state note when the meta key is absent", () => {
    const subject = messageSubject({});
    render(<>{byId("workflow").render(subject)}</>);
    expect(screen.getByText(/No workflow state for this message\./)).toBeInTheDocument();
  });

  it("shows the empty state for non-message subjects too", () => {
    const subject: InspectorSubject = {
      kind: "event",
      event: { id: "e0", timestamp: new Date().toISOString(), type: "note", label: "note", ordinal: 0, category: "other" },
    };
    render(<>{byId("trace").render(subject)}</>);
    expect(screen.getByText(/No trace for this message\./)).toBeInTheDocument();
  });
});
