// THROWAWAY WU-3 preview (load /#atlas): the real Atlas SessionReview as the
// run view, with the workflow pane rebuilt on Atlas ConstellationGraph. Delete
// once WU-3 wires this into App in place of TrialInspector.
import { useEffect, useMemo, useState } from "react";
import { SessionReview, ConstellationGraph } from "@altairalabs/atlas";
import { useArenaAPI } from "@/hooks/useArenaAPI";
import { overlayWorkflowRun } from "@/lib/arenaView";
import { adaptRun, adaptWorkflow } from "@/lib/atlasAdapter";
import type { RunResult, WorkflowGraph } from "@/types";

export function AtlasSessionPreview() {
  const { getResults, getResult, getWorkflow } = useArenaAPI();
  const [ids, setIds] = useState<string[]>([]);
  const [idx, setIdx] = useState(0);
  const [run, setRun] = useState<RunResult | null>(null);
  const [graph, setGraph] = useState<WorkflowGraph | null>(null);
  const [theme, setTheme] = useState<"light" | "dark">("light");

  useEffect(() => {
    const stored = window.localStorage.getItem("arena.theme");
    const t = stored === "dark" || stored === "light" ? stored : "light";
    setTheme(t);
    document.documentElement.dataset.theme = t;
    document.documentElement.style.colorScheme = t;
  }, []);

  useEffect(() => { getResults().then((rs) => setIds(rs ?? [])).catch(() => {}); }, [getResults]);
  useEffect(() => { getWorkflow().then(setGraph).catch(() => {}); }, [getWorkflow]);
  useEffect(() => {
    const id = ids[idx];
    if (id) getResult(id).then(setRun).catch(() => {});
  }, [ids, idx, getResult]);

  const adapted = run ? adaptRun(run) : null;
  const wf = useMemo(
    () => (graph && graph.nodes.length ? adaptWorkflow(overlayWorkflowRun(graph, run ?? undefined)) : null),
    [graph, run],
  );

  return (
    <div style={{ height: "100vh", display: "flex", flexDirection: "column", background: "var(--bg-app)", color: "var(--text-body)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "10px 16px", borderBottom: "1px solid var(--border-default)", fontFamily: "var(--font-mono)", fontSize: "var(--text-size-mono-xs)" }}>
        <strong style={{ color: "var(--text-heading)" }}>Atlas SessionReview — preview</strong>
        <select value={idx} onChange={(e) => setIdx(Number(e.target.value))} style={{ background: "var(--surface-2)", color: "var(--text-body)", border: "1px solid var(--border-default)", borderRadius: 6, padding: "3px 6px", fontFamily: "inherit" }}>
          {ids.map((id, i) => (<option key={id} value={i}>{id}</option>))}
        </select>
        <span style={{ marginLeft: "auto", color: "var(--text-faint)" }}>#atlas · throwaway · {ids.length} runs</span>
      </div>
      <div style={{ flex: 1, minHeight: 0, display: "flex" }}>
        <div style={{ flex: 1, minWidth: 0, padding: 12 }}>
          {adapted ? <SessionReview title={adapted.title} messages={adapted.messages} checks={adapted.checks} recording={adapted.recording} /> : <div style={{ padding: 24, color: "var(--text-faint)" }}>Loading run…</div>}
        </div>
        {wf && (
          <div style={{ width: 400, flex: "none", borderLeft: "1px solid var(--border-default)", display: "flex", flexDirection: "column", minHeight: 0 }}>
            <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border-default)", fontFamily: "var(--font-mono)", fontSize: "var(--text-size-mono-micro)", color: "var(--text-faint)", textTransform: "uppercase" }}>Workflow</div>
            <div style={{ flex: 1, minHeight: 0 }}>
              <ConstellationGraph nodes={wf.nodes} edges={wf.edges} theme={theme} direction="LR" height="100%" />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
