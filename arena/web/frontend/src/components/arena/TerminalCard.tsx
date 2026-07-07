import { Terminal } from "@/components/atlas/Terminal";
import type { TerminalLine } from "@/components/atlas/types";

export interface TerminalCardProps {
  lines: TerminalLine[];
}

// TerminalCard — the Trial Inspector's right-rail bottom card: a cosmetic
// CLI transcript for the selected trial, built upstream by
// `buildTerminalLines` in `lib/arenaView.ts`. Terminal supplies its own chrome.
export function TerminalCard({ lines }: TerminalCardProps) {
  return <Terminal title="promptarena ❯ trial" prompt="arena ❯" lines={lines} />;
}
