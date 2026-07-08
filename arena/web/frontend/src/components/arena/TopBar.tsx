import { StatusPill } from "@/components/atlas/StatusPill";
import logoUrl from "@/assets/logo-promptarena.svg";

export interface TopBarProps {
  connected: boolean;
  promptpack?: string;
  runningLive: boolean;
  theme: "light" | "dark";
  onToggleTheme: () => void;
}

function MoonIcon() {
  return (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  );
}

function SunIcon() {
  return (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
    </svg>
  );
}

// The TopBar is the app's "instrument bezel" — a night-sky ink bar that never
// flips to the light/paper surface, unlike the rest of the page chrome. The
// semantic tokens it (and its children, StatusPill) read via var(...) are
// theme-dependent (they re-point under [data-theme="light"]), so the bar
// pins its own local values for the ones actually used here, overriding the
// cascade for this subtree only. Keeping these as CSS custom-property
// overrides (rather than hardcoding hex through the JSX below) means every
// var(--…) reference in TopBar and its children keeps working — it just
// always resolves to the dark ramp inside this <header>.
const DARK_RAMP_OVERRIDE = {
  "--ink-canvas": "#0A1120",
  "--star-100": "#F4F8FF",
  "--star-600": "#9FB1CC",
  "--star-700": "#8195B0",
  "--star-900": "#5E708C",
  "--hairline": "rgba(147, 197, 253, 0.10)",
  "--hairline-strong": "rgba(147, 197, 253, 0.18)",
  "--pulsar-300": "#7DD3A8",
  "--pulsar-500": "#34D399",
  "--signal-red-300": "#FCA5A5",
  "--gold-300": "#F0D79A",
  "--glow-gold": "0 8px 22px -8px rgba(227,179,65,0.5)",
} as React.CSSProperties;

// TopBar — the sticky Atlas top bar: logo + wordmark + studio tag + promptpack
// context on the left, the connection/live StatusPill + theme toggle on the
// right. The gold "Run trial" action lives in CommandStrip instead — TopBar
// used to duplicate it, but one action surface is enough. Rendered as an ink
// night-sky bar in BOTH themes (intentional — it's the app's "instrument
// bezel", not page chrome that follows the light/dark surface) via
// DARK_RAMP_OVERRIDE above.
export function TopBar({
  connected,
  promptpack,
  runningLive,
  theme,
  onToggleTheme,
}: TopBarProps) {
  const status = runningLive ? "running" : connected ? "healthy" : "error";
  const statusLabel = runningLive ? "Live" : connected ? "Online" : "Offline";
  const isDark = theme === "dark";

  return (
    <header
      style={{
        ...DARK_RAMP_OVERRIDE,
        display: "flex",
        alignItems: "center",
        gap: 16,
        height: 68,
        position: "sticky",
        top: 0,
        zIndex: 20,
        backdropFilter: "blur(8px)",
        background: "linear-gradient(90deg, #070C16 0%, #0C1526 60%, #101D30 100%)",
        borderBottom: "1px solid var(--hairline)",
        margin: "0 -32px",
        padding: "0 32px",
      }}
    >
      <img src={logoUrl} alt="PromptArena" width={30} height={30} style={{ flex: "none" }} />
      <span style={{ font: "600 17px var(--font-sans)", letterSpacing: "-0.01em", color: "var(--star-100)" }}>
        PromptArena
      </span>
      <span
        style={{
          font: "500 10px var(--font-mono)",
          letterSpacing: "0.22em",
          textTransform: "uppercase",
          color: "var(--star-900)",
        }}
      >
        studio
      </span>
      <div style={{ width: 1, height: 22, background: "var(--hairline)", flex: "none" }} />
      {promptpack && (
        <span style={{ font: "400 12px var(--font-mono)", color: "var(--star-700)" }}>{promptpack}</span>
      )}

      <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 12 }}>
        <StatusPill status={status}>{statusLabel}</StatusPill>
        <button
          onClick={onToggleTheme}
          aria-label={isDark ? "Switch to light mode" : "Switch to dark mode"}
          title={isDark ? "Switch to light mode" : "Switch to dark mode"}
          style={{
            width: 38,
            height: 38,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            borderRadius: "var(--radius-md)",
            border: "1px solid var(--hairline-strong)",
            background: "transparent",
            color: "var(--star-600)",
            cursor: "pointer",
          }}
        >
          {isDark ? <MoonIcon /> : <SunIcon />}
        </button>
      </div>
    </header>
  );
}
