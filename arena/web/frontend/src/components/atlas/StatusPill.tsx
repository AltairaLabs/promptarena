const STATUS = {
  running:    { color: 'var(--pulsar-300)', dot: 'var(--pulsar-500)', bg: 'rgba(52,211,153,0.10)', bd: 'rgba(52,211,153,0.30)' },
  healthy:    { color: 'var(--pulsar-300)', dot: 'var(--pulsar-500)', bg: 'rgba(52,211,153,0.10)', bd: 'rgba(52,211,153,0.30)' },
  reconciled: { color: 'var(--pulsar-300)', dot: null,                bg: 'rgba(52,211,153,0.08)', bd: 'rgba(52,211,153,0.28)' },
  pending:    { color: 'var(--gold-300)',   dot: 'var(--amber-500)',  bg: 'rgba(245,158,11,0.10)', bd: 'rgba(245,158,11,0.30)' },
  error:      { color: 'var(--signal-red-300)', dot: 'var(--signal-red)', bg: 'rgba(239,68,68,0.10)', bd: 'rgba(239,68,68,0.30)' },
  idle:       { color: 'var(--star-600)',   dot: 'var(--star-900)',   bg: 'rgba(147,197,253,0.06)', bd: 'var(--hairline-strong)' },
};

export interface StatusPillProps {
  status?: "running" | "healthy" | "reconciled" | "pending" | "error" | "idle";
  children?: React.ReactNode;
  style?: React.CSSProperties;
}

/**
 * Atlas StatusPill — live operational state, read like an instrument.
 * Mono label + a small status dot; reconciled is dot-less (it's a
 * resolved, calm state).
 */
export function StatusPill({ status = 'idle', children, style = {}, ...rest }: StatusPillProps) {
  const s = STATUS[status] || STATUS.idle;
  const label = children || status;
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 7,
        fontFamily: 'var(--font-mono)',
        fontWeight: 'var(--fw-semibold)',
        fontSize: 'var(--text-mono-xs)',
        lineHeight: 1,
        color: s.color,
        background: s.bg,
        border: `1px solid ${s.bd}`,
        borderRadius: 'var(--radius-sm)',
        padding: '7px 12px',
        whiteSpace: 'nowrap',
        textTransform: 'capitalize',
        ...style,
      }}
      {...rest}
    >
      {s.dot && (
        <span style={{ width: 6, height: 6, borderRadius: '50%', background: s.dot, flex: 'none' }} />
      )}
      {label}
    </span>
  );
}
