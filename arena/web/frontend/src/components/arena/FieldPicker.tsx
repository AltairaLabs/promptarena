import { useMemo, useState } from "react";

// At or below this many items an axis is just a row of toggle chips — the
// whole field fits on one line and search would be ceremony. Above it, chips
// stop being a control: 300 badges is a wall, whether or not a search box
// filters them, which is why the large-field mode shows the SELECTION and
// treats search as the way to add to it.
export const CHIP_THRESHOLD = 10;

// How many selected chips to show before summarising the rest behind "+N more".
// A 50-item selection is meaningful state but not something to read in full.
// Comfortably above CHIP_THRESHOLD on purpose: a field that only just tipped
// into search mode should still show its whole selection rather than hiding
// three chips behind a "+3 more" that costs more attention than it saves.
export const VISIBLE_SELECTED = 16;

// Search results are capped so a one-letter query can't paint the wall back in.
const MAX_RESULTS = 40;

export interface FieldPickerItem {
  id: string;
  label?: string;
}

export interface FieldPickerProps {
  // Axis name, shown as the row's eyebrow ("Scenarios", "Contenders").
  label: string;
  items: FieldPickerItem[];
  selected: string[];
  onChange: (selected: string[]) => void;
  // Most this axis will select at once — bounds both the rendered grid and,
  // since selection is double-duty, the blast radius of a field run.
  cap: number;
}

const nameOf = (item: FieldPickerItem) => item.label ?? item.id;

// FieldPicker — one axis of the "chart a run" selection. A small field is the
// familiar row of toggle chips. A large one shows only what's selected, with a
// search box to reach the rest; either way the selection it emits is what the
// trial matrix draws AND what "Run the field" runs.
export function FieldPicker({ label, items, selected, onChange, cap }: FieldPickerProps) {
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState(false);
  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const searchable = items.length > CHIP_THRESHOLD;

  // Selection is emitted in config order, never click order, so the matrix
  // columns don't reshuffle as you toggle.
  const toggle = (id: string) => {
    const next = new Set(selectedSet);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    onChange(items.filter((i) => next.has(i.id)).map((i) => i.id));
  };

  const results = useMemo(() => {
    if (!query) return [];
    const q = query.toLowerCase();
    return items.filter((i) => nameOf(i).toLowerCase().includes(q)).slice(0, MAX_RESULTS);
  }, [items, query]);

  const selectedItems = useMemo(
    () => items.filter((i) => selectedSet.has(i.id)),
    [items, selectedSet],
  );

  // The cap is a rendering/cost bound, not a preference — say so when it bit,
  // but not when the user simply picked a handful themselves, and not while a
  // search is on screen (the results are the subject then, not the selection).
  const capped = !query && items.length > cap && selected.length === cap;

  const visible = expanded ? selectedItems : selectedItems.slice(0, VISIBLE_SELECTED);
  const hidden = selectedItems.length - visible.length;

  return (
    <div style={{ display: "flex", alignItems: "flex-start", gap: 12, width: "100%" }}>
      <span style={{ ...eyebrowStyle, flex: "none", paddingTop: 6, width: 92 }}>{label}</span>

      <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 8 }}>
        {searchable && (
          <input
            type="search"
            role="searchbox"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={`Search ${items.length} ${label.toLowerCase()}…`}
            style={searchStyle}
          />
        )}

        {query ? (
          results.length > 0 ? (
            <div role="listbox" aria-multiselectable style={chipRowStyle(true)}>
              {results.map((item) => {
                const active = selectedSet.has(item.id);
                return (
                  <button
                    key={item.id}
                    type="button"
                    role="option"
                    aria-selected={active}
                    onClick={() => toggle(item.id)}
                    style={chipStyle(active)}
                  >
                    {nameOf(item)}
                    {active ? " ✓" : ""}
                  </button>
                );
              })}
            </div>
          ) : (
            <span style={mutedStyle}>
              no {label.toLowerCase()} match “{query}”
            </span>
          )
        ) : (
          // A small field shows the whole thing — unselected items included,
          // because toggling them back on IS the control. A large one shows
          // only the selection; the rest is reachable through search.
          <div data-testid="field-picker-selection" style={chipRowStyle(expanded)}>
            {(searchable ? visible : items).map((item) => {
              const active = selectedSet.has(item.id);
              return (
                <button
                  key={item.id}
                  type="button"
                  aria-pressed={active}
                  onClick={() => toggle(item.id)}
                  title={active ? `Remove ${nameOf(item)}` : `Add ${nameOf(item)}`}
                  style={chipStyle(active)}
                >
                  {nameOf(item)}
                  {searchable ? " ×" : ""}
                </button>
              );
            })}
            {selectedItems.length === 0 && searchable && (
              <span style={mutedStyle}>nothing selected — search to add</span>
            )}
          </div>
        )}

        {!query && hidden > 0 && (
          <button type="button" onClick={() => setExpanded(true)} style={moreStyle}>
            +{hidden} more
          </button>
        )}
        {!query && expanded && selectedItems.length > VISIBLE_SELECTED && (
          <button type="button" onClick={() => setExpanded(false)} style={moreStyle}>
            show fewer
          </button>
        )}

        {capped && (
          <span style={mutedStyle}>
            showing the first {cap} of {items.length} — search to pick others
          </span>
        )}
      </div>

      <span style={{ display: "inline-flex", alignItems: "center", gap: 8, flex: "none", paddingTop: 4 }}>
        {items.length > selected.length || selected.length === 0 ? (
          <span style={{ font: "12px var(--font-mono)", color: "var(--star-700)" }}>
            {selected.length} of {items.length}
          </span>
        ) : null}
        <button
          type="button"
          onClick={() => onChange(items.slice(0, cap).map((i) => i.id))}
          style={quickToggleStyle}
        >
          All
        </button>
        <button type="button" onClick={() => onChange([])} style={quickToggleStyle}>
          None
        </button>
      </span>
    </div>
  );
}

// A scrolling chip row, but only when it could actually get long — an
// unexpanded selection or a chip-mode field is short by construction.
const chipRowStyle = (scrollable: boolean): React.CSSProperties => ({
  display: "flex",
  alignItems: "center",
  gap: 8,
  flexWrap: "wrap",
  ...(scrollable ? { maxHeight: 96, overflowY: "auto" } : {}),
});

const chipStyle = (active: boolean): React.CSSProperties => ({
  cursor: "pointer",
  padding: "5px 12px",
  borderRadius: "999px",
  font: "500 13px var(--font-sans)",
  background: active ? "var(--starlight-tint)" : "transparent",
  border: `1px solid ${active ? "var(--starlight-300)" : "var(--hairline)"}`,
  color: active ? "var(--star-100)" : "var(--star-600)",
  transition: "background .12s ease, color .12s ease",
});

const eyebrowStyle: React.CSSProperties = {
  fontFamily: "var(--font-mono)",
  fontSize: "var(--text-size-mono-label)",
  fontWeight: "var(--fw-medium)",
  textTransform: "uppercase",
  letterSpacing: "var(--tracking-eyebrow)",
  color: "var(--star-900)",
};

const mutedStyle: React.CSSProperties = {
  font: "11px var(--font-mono)",
  color: "var(--star-800)",
};

const moreStyle: React.CSSProperties = {
  alignSelf: "flex-start",
  cursor: "pointer",
  background: "transparent",
  border: "none",
  padding: 0,
  font: "11px var(--font-mono)",
  color: "var(--text-link)",
};

const searchStyle: React.CSSProperties = {
  width: 260,
  maxWidth: "100%",
  boxSizing: "border-box",
  padding: "6px 12px",
  borderRadius: "var(--radius-sm)",
  border: "1px solid var(--hairline)",
  background: "var(--surface-2)",
  color: "var(--star-200)",
  font: "13px var(--font-sans)",
};

const quickToggleStyle: React.CSSProperties = {
  cursor: "pointer",
  background: "transparent",
  border: "none",
  padding: "2px 4px",
  font: "500 11px var(--font-mono)",
  textTransform: "uppercase",
  letterSpacing: "0.08em",
  color: "var(--text-link)",
};
