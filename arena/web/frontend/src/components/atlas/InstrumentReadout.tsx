import type { MetricSpec } from "./types";

const TONE: Record<string, string> = {
  default: 'var(--star-200)',
  healthy: 'var(--pulsar-300)',
  pending: 'var(--gold-300)',
  error:   'var(--signal-red-300)',
  gold:    'var(--gold-300)',
};

export interface InstrumentReadoutProps {
  metrics: MetricSpec[];
  columns?: number;
  style?: React.CSSProperties;
}

/**
 * InstrumentReadout — calm gauges that read like a control panel:
 * desired vs observed, drift, convergence. A hairline-gridded set of
 * cells, each a mono label + a big value (+ optional unit / sub / dot).
 * Not marketing stat cards.
 */
export function InstrumentReadout({ metrics = [], columns = 2, style = {}, ...rest }: InstrumentReadoutProps) {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: `repeat(${columns}, 1fr)`,
        gap: 1,
        background: 'var(--hairline)',
        borderRadius: 'var(--radius-xl)',
        overflow: 'hidden',
        border: '1px solid var(--hairline)',
        ...style,
      }}
      {...rest}
    >
      {metrics.map((m, i) => (
        <div key={i} style={{ background: 'var(--ink-canvas)', padding: '16px 18px' }}>
          <div
            style={{
              fontFamily: 'var(--font-mono)',
              fontWeight: 'var(--fw-medium)',
              fontSize: 'var(--text-mono-micro)',
              letterSpacing: 'var(--tracking-label)',
              textTransform: 'uppercase',
              color: 'var(--star-900)',
              marginBottom: 10,
            }}
          >
            {m.label}
          </div>
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 6 }}>
            <span
              style={{
                fontFamily: 'var(--font-sans)',
                fontWeight: 'var(--fw-semibold)',
                fontSize: 22,
                lineHeight: 1,
                color: TONE[m.tone ?? 'default'] || TONE.default,
              }}
            >
              {m.value}
            </span>
            {m.unit && (
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-mono-micro)', color: 'var(--star-900)' }}>
                {m.unit}
              </span>
            )}
            {m.dot && (
              <span style={{ marginLeft: 'auto', color: m.dot === 'healthy' ? 'var(--pulsar-500)' : m.dot === 'pending' ? 'var(--amber-500)' : 'var(--signal-red)' }}>●</span>
            )}
            {m.sub && (
              <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: '9px', color: 'var(--star-900)' }}>
                {m.sub}
              </span>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
