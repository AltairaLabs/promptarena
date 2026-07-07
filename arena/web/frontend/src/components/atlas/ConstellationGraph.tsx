import { Fragment } from "react";
import type { GraphNode, GraphEdge } from "./types";

const KIND: Record<string, { fill: string; halo: string; r: number; diamond?: boolean; glow?: boolean }> = {
  prompt: { fill: 'var(--node-prompt)', halo: 'rgba(196,181,253,0.16)', r: 4.5 },
  agent:  { fill: 'var(--node-agent)',  halo: 'rgba(147,197,253,0.14)', r: 4.5 },
  tool:   { fill: 'var(--node-tool)',   halo: 'rgba(103,232,249,0.16)', r: 4.5 },
  branch: { fill: 'var(--node-branch)', halo: 'rgba(227,179,65,0.18)',  r: 4,  diamond: true },
  output: { fill: 'var(--gold-500)',    halo: 'rgba(227,179,65,0.16)', r: 5.5, glow: true },
  entry:  { fill: 'var(--gold-500)',    halo: 'rgba(227,179,65,0.16)', r: 5.5, glow: true },
};

export interface ConstellationGraphProps {
  nodes: GraphNode[];
  edges: GraphEdge[];
  width?: number;
  height?: number;
  showLabels?: boolean;
  style?: React.CSSProperties;
}

/**
 * ConstellationGraph — the signature pattern. Workflows / topologies as
 * star charts: nodes by kind, wayfinding lines (dashed for else-branches),
 * gold for entry & output. Coordinates are in the supplied viewBox space.
 */
export function ConstellationGraph({
  nodes = [],
  edges = [],
  width = 360,
  height = 160,
  showLabels = false,
  style = {},
  ...rest
}: ConstellationGraphProps) {
  const byId = Object.fromEntries(nodes.map((n) => [n.id, n]));
  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      width={width}
      height={height}
      preserveAspectRatio="xMidYMid meet"
      style={{ display: 'block', ...style }}
      {...rest}
    >
      {edges.map((e, i) => {
        const a = byId[e.from], b = byId[e.to];
        if (!a || !b) return null;
        const goldEdge = (byId[e.to] && (byId[e.to].kind === 'output')) || e.gold;
        const opacity = e.dim ? 0.3 : (goldEdge ? 0.55 : 0.45);
        const stroke = goldEdge ? 'var(--gold-500)' : 'var(--starlight-500)';
        // Label sits on the arc apex for a curved (skip-layer) edge, else just
        // above the straight line's midpoint.
        const lx = e.curve ? e.curve.cx : (a.x + b.x) / 2;
        const ly = e.curve ? e.curve.cy - 2 : (a.y + b.y) / 2 - 5;
        return (
          <Fragment key={`e${i}`}>
            {e.curve ? (
              <path
                d={`M ${a.x} ${a.y} Q ${e.curve.cx} ${e.curve.cy} ${b.x} ${b.y}`}
                fill="none"
                stroke={stroke}
                strokeWidth={1.2}
                opacity={opacity}
                strokeDasharray={e.dashed ? '3 5' : undefined}
              />
            ) : (
              <line
                x1={a.x} y1={a.y} x2={b.x} y2={b.y}
                stroke={stroke}
                strokeWidth={1.2}
                opacity={opacity}
                strokeDasharray={e.dashed ? '3 5' : undefined}
              />
            )}
            {e.label && (
              <text
                x={lx} y={ly}
                textAnchor="middle"
                opacity={opacity}
                style={{
                  font: '500 10px var(--font-mono)',
                  fill: 'var(--star-500)',
                  // halo so the label stays readable where it crosses the wayfinding line
                  stroke: 'var(--surface)',
                  strokeWidth: 3,
                  strokeLinejoin: 'round',
                  paintOrder: 'stroke',
                }}
              >
                {e.label}
              </text>
            )}
          </Fragment>
        );
      })}
      {nodes.map((n) => {
        const k = KIND[n.kind] || KIND.agent;
        const cx = n.x, cy = n.y;
        return (
          <g key={n.id} style={n.dim ? { opacity: 0.3 } : undefined}>
            {k.diamond ? (
              <rect
                x={cx - k.r * 2.2} y={cy - k.r * 2.2}
                width={k.r * 4.4} height={k.r * 4.4}
                rx="2"
                transform={`rotate(45 ${cx} ${cy})`}
                fill={k.halo}
              />
            ) : (
              <circle cx={cx} cy={cy} r={k.r * 2.4} fill={k.halo} />
            )}
            {k.diamond ? (
              <rect
                x={cx - k.r} y={cy - k.r}
                width={k.r * 2} height={k.r * 2}
                transform={`rotate(45 ${cx} ${cy})`}
                fill={k.fill}
              />
            ) : (
              <circle
                cx={cx} cy={cy} r={k.r}
                fill={k.fill}
                style={k.glow ? { filter: 'drop-shadow(0 0 6px rgba(227,179,65,0.9))', animation: 'atlas-twinkle 3s ease-in-out infinite' } : undefined}
              />
            )}
            {showLabels && n.label && (
              <text
                x={cx} y={cy + k.r * 2.4 + 15}
                textAnchor="middle"
                style={{
                  font: '600 12px var(--font-mono)',
                  fill: 'var(--star-300)',
                  stroke: 'var(--surface)',
                  strokeWidth: 3,
                  strokeLinejoin: 'round',
                  paintOrder: 'stroke',
                }}
              >
                {n.label}
              </text>
            )}
          </g>
        );
      })}
    </svg>
  );
}
