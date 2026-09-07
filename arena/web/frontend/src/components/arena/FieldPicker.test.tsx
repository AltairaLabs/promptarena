import { render, screen, fireEvent, within } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { FieldPicker, CHIP_THRESHOLD, VISIBLE_SELECTED } from "./FieldPicker";

const items = (n: number, prefix = "item") =>
  Array.from({ length: n }, (_, i) => ({ id: `${prefix}-${i + 1}` }));

function renderPicker(count: number, overrides: Partial<Parameters<typeof FieldPicker>[0]> = {}) {
  const onChange = vi.fn();
  const props = {
    label: "Scenarios",
    items: items(count),
    selected: items(count).map((i) => i.id),
    cap: 50,
    ...overrides,
    onChange,
  };
  return { ...render(<FieldPicker {...props} />), props: { ...props, onChange } };
}

describe("FieldPicker — small field", () => {
  it("renders every item as a toggle chip, with no search", () => {
    renderPicker(CHIP_THRESHOLD);
    expect(screen.getByRole("button", { name: "item-1" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: `item-${CHIP_THRESHOLD}` })).toBeInTheDocument();
    expect(screen.queryByRole("searchbox")).not.toBeInTheDocument();
  });

  it("shows unselected items too, so they can be toggled back on", () => {
    renderPicker(4, { selected: ["item-1"] });
    expect(screen.getByRole("button", { name: "item-3" })).toBeInTheDocument();
  });

  it("toggles an item off without disturbing the rest", () => {
    const { props } = renderPicker(4);
    fireEvent.click(screen.getByRole("button", { name: "item-2" }));
    expect(props.onChange).toHaveBeenCalledWith(["item-1", "item-3", "item-4"]);
  });

  it("toggles an item back on, in config order", () => {
    const { props } = renderPicker(4, { selected: ["item-1"] });
    fireEvent.click(screen.getByRole("button", { name: "item-3" }));
    expect(props.onChange).toHaveBeenCalledWith(["item-1", "item-3"]);
  });

  it("says nothing about counts when the whole field is showing", () => {
    renderPicker(4);
    expect(screen.queryByText(/ of /)).not.toBeInTheDocument();
  });
});

describe("FieldPicker — large field", () => {
  const big = CHIP_THRESHOLD + 1;

  it("switches to search once the field is too big to read as chips", () => {
    renderPicker(big);
    expect(screen.getByRole("searchbox")).toBeInTheDocument();
  });

  it("shows ONLY the selection, never the whole field, when idle", () => {
    renderPicker(300, { selected: ["item-1", "item-2"] });
    expect(screen.getByRole("button", { name: /item-1/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /item-2/ })).toBeInTheDocument();
    // item-3 is in the field but not selected — rendering it (and 297 more)
    // is the wall of badges this replaces.
    expect(screen.queryByRole("button", { name: /item-3/ })).not.toBeInTheDocument();
  });

  it("reports how much of the field is selected", () => {
    renderPicker(300, { selected: items(300).map((i) => i.id).slice(0, 25) });
    expect(screen.getByText("25 of 300")).toBeInTheDocument();
  });

  it("summarises a long selection rather than listing all of it", () => {
    renderPicker(300, { selected: items(300).map((i) => i.id).slice(0, 25) });
    const chips = screen.getByTestId("field-picker-selection");
    expect(within(chips).getAllByRole("button")).toHaveLength(VISIBLE_SELECTED);
    expect(screen.getByRole("button", { name: `+${25 - VISIBLE_SELECTED} more` })).toBeInTheDocument();
  });

  it("expands the summary to the full selection on demand", () => {
    renderPicker(300, { selected: items(300).map((i) => i.id).slice(0, 25) });
    fireEvent.click(screen.getByRole("button", { name: `+${25 - VISIBLE_SELECTED} more` }));
    const chips = screen.getByTestId("field-picker-selection");
    expect(within(chips).getAllByRole("button")).toHaveLength(25);
  });

  it("removes a selected item when its chip is clicked", () => {
    const { props } = renderPicker(300, { selected: ["item-1", "item-2"] });
    fireEvent.click(screen.getByRole("button", { name: /item-2/ }));
    expect(props.onChange).toHaveBeenCalledWith(["item-1"]);
  });

  it("searches the whole field, not just the selection", () => {
    renderPicker(300, { selected: ["item-1"] });
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "item-250" } });
    const results = screen.getByRole("listbox");
    expect(within(results).getByRole("option", { name: /item-250/ })).toBeInTheDocument();
  });

  it("adds an item picked from the search results", () => {
    const { props } = renderPicker(300, { selected: ["item-1"] });
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "item-250" } });
    fireEvent.click(screen.getByRole("option", { name: /item-250/ }));
    expect(props.onChange).toHaveBeenCalledWith(["item-1", "item-250"]);
  });

  it("marks results already in the selection, so search can remove too", () => {
    renderPicker(300, { selected: ["item-100"] });
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "item-100" } });
    expect(screen.getByRole("option", { name: /item-100/ })).toHaveAttribute("aria-selected", "true");
  });

  it("says so when a search matches nothing", () => {
    renderPicker(300, { selected: ["item-1"] });
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "zzz" } });
    expect(screen.getByText(/no scenarios match/i)).toBeInTheDocument();
  });

  it("prompts when nothing is selected at all", () => {
    renderPicker(300, { selected: [] });
    expect(screen.getByText(/search to add/i)).toBeInTheDocument();
  });

  it("All selects up to the cap, never past it", () => {
    const { props } = renderPicker(300, { selected: [], cap: 25 });
    fireEvent.click(screen.getByRole("button", { name: "All" }));
    expect(props.onChange.mock.calls[0][0]).toHaveLength(25);
  });

  it("None clears the selection", () => {
    const { props } = renderPicker(300);
    fireEvent.click(screen.getByRole("button", { name: "None" }));
    expect(props.onChange).toHaveBeenCalledWith([]);
  });

  it("warns when the selection is capped, so truncation is never silent", () => {
    renderPicker(300, { selected: items(300).map((i) => i.id).slice(0, 25), cap: 25 });
    expect(screen.getByText(/showing the first 25 of 300/i)).toBeInTheDocument();
  });

  it("does not warn when the user selected fewer than the cap themselves", () => {
    renderPicker(300, { selected: ["item-1", "item-2"], cap: 25 });
    expect(screen.queryByText(/showing the first/i)).not.toBeInTheDocument();
  });

  it("drops the cap warning while a search is showing results", () => {
    renderPicker(300, { selected: items(300).map((i) => i.id).slice(0, 25), cap: 25 });
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "item-9" } });
    expect(screen.queryByText(/showing the first/i)).not.toBeInTheDocument();
  });
});
