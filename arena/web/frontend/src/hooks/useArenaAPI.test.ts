import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useArenaAPI } from "./useArenaAPI";
import type { WorkflowGraph } from "@/types";

describe("useArenaAPI", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it("getWorkflow fetches /api/workflow and returns the parsed graph", async () => {
    const graph: WorkflowGraph = {
      nodes: [
        { id: "intake", label: "intake", kind: "entry", entry: true, terminal: false },
        { id: "resolve", label: "resolve", kind: "output", entry: false, terminal: true },
      ],
      edges: [{ from: "intake", to: "resolve", label: "classified" }],
    };
    const fetchMock = vi.fn().mockResolvedValue({ json: () => Promise.resolve(graph) });
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useArenaAPI());
    const out = await result.current.getWorkflow();

    expect(fetchMock).toHaveBeenCalledWith("/api/workflow");
    expect(out).toEqual(graph);
  });
});
