import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// toDisplayString coerces an unknown value into a human-readable string for
// logs and UI messages. It handles the usual primitives directly, reaches for
// `.message` on Error-like objects, and JSON-stringifies anything else rather
// than letting objects fall through to "[object Object]".
// formatDuration renders a millisecond count as a compact, human-readable
// string: sub-millisecond durations round down to "<1ms" rather than "0ms",
// sub-second durations are whole milliseconds, sub-minute durations are
// seconds (one decimal place under 10s, whole seconds at/above), and
// anything longer rolls over into "Nm Ns".
export function formatDuration(ms: number): string {
  if (ms < 1) return "<1ms";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)}s`;
  const minutes = Math.floor(ms / 60000);
  const seconds = Math.round((ms % 60000) / 1000);
  return `${minutes}m ${seconds}s`;
}

export function toDisplayString(value: unknown, fallback: string): string {
  if (value === undefined || value === null) return fallback;
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") {
    return String(value);
  }
  if (typeof value === "object") {
    const maybeMessage = (value as { message?: unknown }).message;
    if (typeof maybeMessage === "string" && maybeMessage !== "") return maybeMessage;
    try {
      return JSON.stringify(value);
    } catch {
      return fallback;
    }
  }
  return fallback;
}
