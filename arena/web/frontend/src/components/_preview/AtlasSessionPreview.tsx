// THROWAWAY WU-0 preview: renders the real Atlas SessionReview over a live
// Arena run (load /#atlas). Delete before finalizing; WU-3 wires this for real.
import { useEffect, useState } from "react";
import { SessionReview } from "@altairalabs/atlas";
import { useArenaAPI } from "@/hooks/useArenaAPI";
import { adaptRun } from "@/lib/atlasAdapter";
import type { RunResult } from "@/types";

export function AtlasSessionPreview() {
  const { getResults, getResult } = useArenaAPI();
  const [ids, setIds] = useState<string[]>([]);
  const [idx, setIdx] = useState(0);
  const [run, setRun] = useState<RunResult | null>(null);

  // Mirror the app's theme (default light) so the preview isn't jarringly dark.
  useEffect(() => {
    const stored = window.localStorage.getItem("arena.theme");
    const theme = stored === "dark" || stored === "light" ? stored : "light";
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
  }, []);

  useEffect(() => {
    getResults().then((rs) => setIds(rs ?? [])).catch(() => {});
  }, [getResults]);
  useEffect(() => {
    const id = ids[idx];
    if (id) getResult(id).then(setRun).catch(() => {});
  }, [ids, idx, getResult]);

  const adapted = run ? adaptRun(run) : null;

  return (
    <div style={{ height: "100vh", display: "flex", flexDirection: "column", background: "var(--bg-app)", color: "var(--text-body)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "10px 16px", borderBottom: "1px solid var(--border-default)", fontFamily: "var(--font-mono)", fontSize: "var(--text-size-mono-xs)" }}>
        <strong style={{ color: "var(--text-heading)" }}>Atlas SessionReview — preview</strong>
        <select value={idx} onChange={(e) => setIdx(Number(e.target.value))} style={{ background: "var(--surface-2)", color: "var(--text-body)", border: "1px solid var(--border-default)", borderRadius: 6, padding: "3px 6px", fontFamily: "inherit" }}>
          {ids.map((id, i) => (
            <option key={id} value={i}>{id}</option>
          ))}
        </select>
        <span style={{ marginLeft: "auto", color: "var(--text-faint)" }}>#atlas · throwaway · {ids.length} runs</span>
      </div>
      <div style={{ flex: 1, minHeight: 0, padding: 12 }}>
        {adapted ? <SessionReview title={adapted.title} messages={adapted.messages} checks={adapted.checks} recording={adapted.recording} /> : <div style={{ padding: 24, color: "var(--text-faint)" }}>Loading run…</div>}
      </div>
    </div>
  );
}
