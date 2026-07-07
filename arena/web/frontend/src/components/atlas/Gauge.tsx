export interface GaugeProps {
  value?: number;
  max?: number;
  size?: number;
  label?: string;
  unit?: string;
  display?: string | null;
  color?: string;
  thickness?: number;
  style?: React.CSSProperties;
}

/**
 * Gauge — a radial convergence dial. The instrument reading for a ratio
 * (replicas, convergence %, budget used). A 270° arc on a hairline track;
 * the reading arc is gold (this is a place the gold star belongs), with a
 * big mono value at the centre. Token-driven; themes dark & light.
 */
export function Gauge({
  value = 0,
  max = 100,
  size = 132,
  label = '',
  unit = '',
  display = null,
  color = 'var(--gold-500)',
  thickness = 8,
  style = {},
  ...rest
}: GaugeProps) {
  const pct = Math.max(0, Math.min(1, max ? value / max : 0));
  const r = (size - thickness) / 2 - 5;
  const cx = size / 2;
  const cy = size / 2;
  const sweep = 270; // degrees of the open dial
  const startAngle = 135; // bottom-left
  const circ = 2 * Math.PI * r;
  const arcFrac = sweep / 360;
  const dash = circ * arcFrac;
  const gap = circ * (1 - arcFrac);
  const shown = dash * pct;

  const centerText = display != null ? display : `${Math.round(pct * 100)}`;

  return (
    <div style={{ display: 'inline-flex', flexDirection: 'column', alignItems: 'center', gap: 8, ...style }} {...rest}>
      <div style={{ position: 'relative', width: size, height: size, overflow: 'visible' }}>
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ transform: `rotate(${startAngle}deg)`, overflow: 'visible' }}>
          <circle cx={cx} cy={cy} r={r} fill="none" stroke="var(--hairline-strong)" strokeWidth={thickness}
            strokeLinecap="round" strokeDasharray={`${dash} ${gap}`} />
          <circle cx={cx} cy={cy} r={r} fill="none" stroke={color} strokeWidth={thickness}
            strokeLinecap="round" strokeDasharray={`${shown} ${circ - shown}`}
            style={{ transition: 'stroke-dasharray var(--dur-slow) var(--ease-out)' }} />
        </svg>
        <div style={{ position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
          <span style={{ font: 'var(--fw-semibold) 26px/1 var(--font-sans)', letterSpacing: '-0.02em', color: 'var(--star-100)' }}>{centerText}</span>
          {unit && <span style={{ font: '500 10px/1 var(--font-mono)', color: 'var(--star-900)', marginTop: 5 }}>{unit}</span>}
        </div>
      </div>
      {label && (
        <span style={{ font: '500 10px/1 var(--font-mono)', letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--star-900)', whiteSpace: 'nowrap' }}>{label}</span>
      )}
    </div>
  );
}
