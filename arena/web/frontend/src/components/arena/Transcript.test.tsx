import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { Transcript } from "./Transcript";
import type { TranscriptMessage } from "@/types";

describe("Transcript", () => {
  it("renders a user bubble with the starlight accent border and an assistant bubble with the ion-cyan accent", () => {
    const messages: TranscriptMessage[] = [
      {
        role: "user",
        idx: 0,
        accent: "var(--starlight-300)",
        bg: "color-mix(in srgb, var(--starlight-300) 11%, transparent)",
        content: "Hi there",
      },
      {
        role: "assistant",
        idx: 1,
        accent: "var(--ion-cyan)",
        bg: "color-mix(in srgb, var(--ion-cyan) 11%, transparent)",
        content: "Hello!",
      },
    ];
    const { container } = render(<Transcript messages={messages} />);
    const cards = container.firstElementChild!.children;
    expect(cards).toHaveLength(2);
    expect((cards[0] as HTMLElement).style.borderLeft).toContain("var(--starlight-300)");
    expect((cards[1] as HTMLElement).style.borderLeft).toContain("var(--ion-cyan)");
    expect(screen.getByText("user")).toBeInTheDocument();
    expect(screen.getByText("assistant")).toBeInTheDocument();
    expect(screen.getByText("Hi there")).toBeInTheDocument();
  });

  it("renders a tool card with its name and body", () => {
    const messages: TranscriptMessage[] = [
      {
        role: "assistant",
        idx: 0,
        accent: "var(--ion-cyan)",
        bg: "transparent",
        tool: { name: "memory__recall", body: '{"query":"prefs"}' },
      },
    ];
    render(<Transcript messages={messages} />);
    expect(screen.getByText("⚙ memory__recall")).toBeInTheDocument();
    expect(screen.getByText('{"query":"prefs"}')).toBeInTheDocument();
  });

  it("renders a failing assert chip with the signal color", () => {
    const messages: TranscriptMessage[] = [
      {
        role: "assistant",
        idx: 0,
        accent: "var(--ion-cyan)",
        bg: "transparent",
        asserts: [{ name: "no-pii", ok: false }],
      },
    ];
    render(<Transcript messages={messages} />);
    const chip = screen.getByText("✗ no-pii");
    expect(chip.style.color).toBe("var(--signal-red-300)");
  });

  it("renders a passing assert chip with the pulsar color", () => {
    const messages: TranscriptMessage[] = [
      {
        role: "assistant",
        idx: 0,
        accent: "var(--ion-cyan)",
        bg: "transparent",
        asserts: [{ name: "on-topic", ok: true }],
      },
    ];
    render(<Transcript messages={messages} />);
    const chip = screen.getByText("✓ on-topic");
    expect(chip.style.color).toBe("var(--pulsar-300)");
  });

  it("shows meta when present", () => {
    const messages: TranscriptMessage[] = [
      { role: "assistant", idx: 0, accent: "var(--ion-cyan)", bg: "transparent", meta: "$0.0069 · 820ms" },
    ];
    render(<Transcript messages={messages} />);
    expect(screen.getByText("$0.0069 · 820ms")).toBeInTheDocument();
  });

  it("calls onSelectMessage with the clicked message's idx when provided", () => {
    const onSelectMessage = vi.fn();
    const messages: TranscriptMessage[] = [
      { role: "user", idx: 0, accent: "var(--starlight-300)", bg: "transparent", content: "Hi there" },
      { role: "assistant", idx: 1, accent: "var(--ion-cyan)", bg: "transparent", content: "Hello!" },
    ];
    render(<Transcript messages={messages} onSelectMessage={onSelectMessage} />);
    fireEvent.click(screen.getByText("Hello!"));
    expect(onSelectMessage).toHaveBeenCalledWith(1);
  });

  it("does not make message cards clickable when onSelectMessage is omitted", () => {
    const messages: TranscriptMessage[] = [
      { role: "user", idx: 0, accent: "var(--starlight-300)", bg: "transparent", content: "Hi there" },
    ];
    const { container } = render(<Transcript messages={messages} />);
    const card = container.firstElementChild!.children[0] as HTMLElement;
    expect(card.style.cursor).not.toBe("pointer");
  });
});
