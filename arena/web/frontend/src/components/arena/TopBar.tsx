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

// TopBar — the sticky Atlas top bar: logo + wordmark + studio tag + promptpack
// context on the left, the connection/live StatusPill + theme toggle on the
// right. The gold "Run trial" action lives in CommandStrip instead — TopBar
// used to duplicate it, but one action surface is enough. The bar follows the
// theme: a light frosted surface in light mode, dark ink in dark mode, with
// child text/pills reading the flipping --star-*/--c-* tokens.
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
        display: "flex",
        alignItems: "center",
        gap: 16,
        height: 68,
        position: "sticky",
        top: 0,
        zIndex: 20,
        backdropFilter: "blur(8px)",
        // Exactly the Atlas design's top bar (Arena.dc.html): a frosted bar
        // that follows the theme via --ink-canvas (light paper in light mode,
        // night-sky ink in dark). Child text/pills use the flipping
        // --star-*/--c-* tokens, so they stay legible on either surface.
        background: "color-mix(in srgb, var(--ink-canvas) 78%, transparent)",
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
