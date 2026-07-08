export interface StarTrailProps {
  points: number[];
  width?: number;
  height?: number;
  keyIndex?: number;
  style?: React.CSSProperties;
}

/**
 * StarTrail — a sparkline drawn as a star trail. A faint starlight
 * polyline across a series, with a gold key star glowing at the latest
 * (or peak) point. For trends where one point is "the one that matters".
 */
export function StarTrail({
  points = [],
  width = 320,
  height = 80,
  keyIndex = -1,
  style = {},
  ...rest
}: StarTrailProps) {
  if (!points.length) return null;
  const max = Math.max(...points);
  const min = Math.min(...points);
  const span = max - min || 1;
  const pad = 8;
  const innerH = height - pad * 2;
  const coords = points.map((v, i) => {
    const x = points.length === 1 ? width / 2 : (i / (points.length - 1)) * width;
    const y = pad + innerH - ((v - min) / span) * innerH;
    return [x, y];
  });
  const ki = keyIndex < 0 ? points.length - 1 : keyIndex;
  const poly = coords.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(' ');

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      width="100%"
      preserveAspectRatio="none"
      style={{ display: 'block', ...style }}
      {...rest}
    >
      <polyline points={poly} fill="none" stroke="var(--starlight-500)" strokeWidth={1.5} opacity={0.7} />
      {coords.map(([x, y], i) =>
        i === ki ? null : <circle key={i} cx={x} cy={y} r={2} fill="var(--starlight-300)" opacity={0.65} />
      )}
      {coords[ki] && (
        <circle
          cx={coords[ki][0]} cy={coords[ki][1]} r={4}
          fill="var(--gold-500)"
          style={{ filter: 'drop-shadow(0 0 6px rgba(227,179,65,0.9))', animation: 'atlas-twinkle 2.4s ease-in-out infinite' }}
        />
      )}
    </svg>
  );
}
