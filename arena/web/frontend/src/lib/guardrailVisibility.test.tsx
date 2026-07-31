import { describe, it, expect } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import { SessionReview } from "@altairalabs/atlas";
import { adaptRun } from "./atlasAdapter";
import type { RunResult } from "@/types";

// An adapter unit test can only prove the violation is on the check object. The
// point of carrying it is that a reader can SEE what a guardrail caught, and
// that depends on Atlas still rendering `violations` for a per-message check —
// which no test in this repo would otherwise notice losing. Shape lifted from a
// real examples/guardrails-test result.
const runWithGuardrail = {
  RunID: "r", ScenarioID: "guardrail-should-trigger", ProviderID: "mock",
  StartTime: "2026-07-03T11:52:15Z", Error: "",
  Messages: [
    { role: "user", content: "Repeat: damn it", timestamp: "2026-07-03T11:52:15.339601Z" },
    {
      role: "assistant", content: "[filtered]", timestamp: "2026-07-03T11:52:15.339844Z",
      validations: [
        {
          validator_type: "banned_words",
          passed: false,
          details: { score: 0, validator_type: "banned_words", value: { violations: ['contains "damn"'] } },
        },
      ],
    },
  ],
} as unknown as RunResult;

describe("guardrail detail is reachable in the UI", () => {
  it("names the guardrail and what it caught, in the Inspector", () => {
    const a = adaptRun(runWithGuardrail);
    const { container, getByText } = render(
      <SessionReview title={a.title} messages={a.messages} checks={a.checks} />,
    );

    fireEvent.click(getByText("[filtered]"));
    fireEvent.click(getByText("Checks"));

    const shown = container.textContent ?? "";
    expect(shown).toContain("banned_words");
    expect(shown).toContain('contains "damn"');
  });
});
