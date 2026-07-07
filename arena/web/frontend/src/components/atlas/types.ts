// Shared prop types for the Atlas viz/primitive components. Kept separate
// from the app-level src/types.ts on purpose — these are Atlas-specific
// shapes, not domain models.

export interface MetricSpec {
  label: string;
  value: string;
  unit?: string;
  sub?: string;
  tone?: "default" | "healthy" | "pending" | "error" | "gold";
  dot?: "healthy" | "pending" | "error";
}
