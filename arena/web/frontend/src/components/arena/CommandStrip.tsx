import { CommandStrip as AtlasCommandStrip } from "@altairalabs/atlas";

export interface CommandStripProps {
  scenarios: { id: string; label?: string }[];
  selectedScenario: string | null;
  selectedProviderLabel: string | null;
  onSelectScenario: (scenarioId: string) => void;
  onRunTrial: () => void;
  runDisabled?: boolean;
}

// CommandStrip — Arena's "chart a run" strip, now backed by the
// @altairalabs/atlas CommandStrip. Maps Arena's scenario/provider props onto
// the package's generic option/readout/action API: scenario chips (clicking
// one selects that scenario's best/first provider upstream in App) plus the
// gold Run trial action. Sits directly under the Hero.
export function CommandStrip({
  scenarios,
  selectedScenario,
  selectedProviderLabel,
  onSelectScenario,
  onRunTrial,
  runDisabled,
}: CommandStripProps) {
  return (
    <AtlasCommandStrip
      style={{ marginTop: 26 }}
      label="CHART A RUN"
      options={scenarios}
      value={selectedScenario}
      onChange={onSelectScenario}
      readout={`${selectedProviderLabel ?? "—"} · ${selectedScenario ?? "—"}`}
      actionLabel="▶ Run trial"
      onAction={onRunTrial}
      actionDisabled={runDisabled}
    />
  );
}
