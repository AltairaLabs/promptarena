import { useId } from "react";

export interface MetricChartProps {
  data: (number | { y: number })[];
  type?: "line" | "area" | "bar";
  width?: number;
  height?: number;
  xLabels?: string[];
  yTicks?: number;
  keyIndex?: number;
  color?: string;
  unit?: string;
  showGrid?: boolean;
  showAxis?: boolean;
  style?: React.CSSProperties;
}

/**
 * MetricChart — the workhorse time-series chart for the control plane.
 * line / area / bar in one component. Instrument-grade: hairline grid,
 * mono axis labels, and a single gold key star on the point that matters
 * (latest by default). Pure SVG, token-driven (themes automatically).
 */
export function MetricChart({
  data = [],
  type = 'line',
  width = 460,
  height = 200,
  xLabels = [],
  yTicks = 4,
  keyIndex = -1,
  color = 'var(--starlight-500)',
  unit = '',
  showGrid = true,
  showAxis = true,
  style = {},
  ...rest
}: MetricChartProps) {
  const uid = useId().replace(/[:]/g, '');
  const vals = data.map((d) => (typeof d === 'number' ? d : d.y));
  if (!vals.length) return null;

  const padL = showAxis ? 38 : 6;
  const padR = 8;
  const padT = 12;
  const padB = showAxis ? 26 : 6;
  const iw = width - padL - padR;
  const ih = height - padT - padB;

  const rawMax = Math.max(...vals, 0);
  const rawMin = Math.min(...vals, 0);
  const max = rawMax === rawMin ? rawMax + 1 : rawMax;
  const min = type === 'bar' ? 0 : rawMin;
  const span = max - min || 1;

  const x = (i: number) => padL + (vals.length === 1 ? iw / 2 : (i / (vals.length - 1)) * iw);
  const y = (v: number) => padT + ih - ((v - min) / span) * ih;
  const ki = keyIndex < 0 ? vals.length - 1 : keyIndex;

  const ticks = Array.from({ length: yTicks + 1 }, (_, i) => min + (span * i) / yTicks);
  const fmt = (n: number) => {
    const a = Math.abs(n);
    if (a >= 1000) return (n / 1000).toFixed(a >= 10000 ? 0 : 1) + 'k';
    return Number.isInteger(n) ? String(n) : n.toFixed(1);
  };

  const linePts = vals.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ');
  const areaPts = `${padL},${padT + ih} ${linePts} ${padL + iw},${padT + ih}`;
  const barW = Math.max(3, (iw / vals.length) * 0.56);

  return (
    <svg viewBox={`0 0 ${width} ${height}`} width="100%" preserveAspectRatio="xMidYMid meet"
      style={{ display: 'block', ...style }} {...rest}>
      <defs>
        <linearGradient id={`area-${uid}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.22" />
          <stop offset="100%" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>

      {/* grid + y labels */}
      {showGrid && ticks.map((t, i) => (
        <g key={i}>
          <line x1={padL} y1={y(t)} x2={padL + iw} y2={y(t)}
            stroke="var(--hairline)" strokeWidth="1" />
          {showAxis && (
            <text x={padL - 8} y={y(t) + 3} textAnchor="end"
              style={{ font: '500 9px var(--font-mono)', fill: 'var(--star-900)' }}>{fmt(t)}</text>
          )}
        </g>
      ))}

      {/* series */}
      {type === 'area' && <polygon points={areaPts} fill={`url(#area-${uid})`} />}
      {(type === 'line' || type === 'area') && (
        <polyline points={linePts} fill="none" stroke={color} strokeWidth="1.75"
          strokeLinejoin="round" strokeLinecap="round" />
      )}
      {type === 'bar' && vals.map((v, i) => {
        const isKey = i === ki;
        const bx = x(i) - barW / 2;
        const by = y(v);
        return (
          <rect key={i} x={bx} y={by} width={barW} height={Math.max(0, padT + ih - by)} rx="2"
            fill={isKey ? 'var(--gold-500)' : color}
            opacity={isKey ? 1 : 0.55} />
        );
      })}

      {/* key star (line/area only — bar marks its key bar in gold) */}
      {(type === 'line' || type === 'area') && vals[ki] != null && (
        <>
          {vals.map((v, i) => i === ki ? null : (
            <circle key={i} cx={x(i)} cy={y(v)} r="2" fill={color} opacity="0.6" />
          ))}
          <circle cx={x(ki)} cy={y(vals[ki])} r="4" fill="var(--gold-500)"
            style={{ filter: 'drop-shadow(0 0 6px rgba(227,179,65,0.85))', animation: 'atlas-twinkle 2.6s ease-in-out infinite' }} />
        </>
      )}

      {/* x labels */}
      {showAxis && xLabels.length > 0 && xLabels.map((lbl, i) => {
        const idx = xLabels.length === vals.length ? i : Math.round((i / (xLabels.length - 1)) * (vals.length - 1));
        return (
          <text key={i} x={x(idx)} y={height - 8} textAnchor="middle"
            style={{ font: '500 9px var(--font-mono)', fill: 'var(--star-900)' }}>{lbl}</text>
        );
      })}

      {unit && showAxis && (
        <text x={padL} y={9} style={{ font: '500 9px var(--font-mono)', fill: 'var(--star-950)' }}>{unit}</text>
      )}
    </svg>
  );
}
