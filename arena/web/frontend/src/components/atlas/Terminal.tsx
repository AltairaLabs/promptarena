import type { TerminalLine } from "./types";

/**
 * Terminal — the Atlas TUI surface: an on-brand terminal block on the
 * deepest ink, with the gold prompt glyph and a brand-mapped line palette.
 * Renders in both dark & light themes (deepest surface).
 *
 * Line types → colour role:
 *   command  gold ❯ prompt + bright text     comment  faint (# …)
 *   output   default star text               success  pulsar green ✓
 *   flag     starlight (args / flags)         error    signal red ✗
 *   muted    faint                            path     ion cyan
 */
const LINE: Record<string, { color: string; glyph?: string; gc?: string }> = {
  output:  { color: 'var(--star-400)' },
  comment: { color: 'var(--star-950)' },
  muted:   { color: 'var(--star-900)' },
  flag:    { color: 'var(--starlight-200)' },
  path:    { color: 'var(--ion-cyan)' },
  success: { color: 'var(--pulsar-300)', glyph: '✓ ', gc: 'var(--pulsar-500)' },
  error:   { color: 'var(--signal-red-300)', glyph: '✗ ', gc: 'var(--signal-red)' },
  warn:    { color: 'var(--gold-300)', glyph: '! ', gc: 'var(--amber-500)' },
};

export interface TerminalProps {
  lines: TerminalLine[];
  title?: string | null;
  prompt?: string;
  cursor?: boolean;
  height?: number | null;
  style?: React.CSSProperties;
}

export function Terminal({
  lines = [],
  title = null,
  prompt = 'arena ❯',
  cursor = true,
  height = null,
  style = {},
  ...rest
}: TerminalProps) {
  return (
    <div style={{
      borderRadius: 'var(--radius-lg)', overflow: 'hidden',
      border: '1px solid var(--hairline)', background: 'var(--ink-void)',
      boxShadow: 'var(--shadow-card)', ...style,
    }} {...rest}>
      {title != null && (
        <div style={{
          display: 'flex', alignItems: 'center', gap: 8, padding: '9px 14px',
          borderBottom: '1px solid var(--hairline)', background: 'var(--ink-void)',
        }}>
          <span style={{ display: 'inline-flex', gap: 5 }}>
            {['var(--star-dim)', 'var(--star-dim)', 'var(--gold-500)'].map((c, i) => (
              <span key={i} style={{ width: 7, height: 7, borderRadius: '50%', background: c }} />
            ))}
          </span>
          <span style={{ font: '500 11px/1 var(--font-mono)', color: 'var(--star-900)', marginLeft: 4 }}>{title}</span>
        </div>
      )}
      <div style={{
        padding: '14px 16px', font: '500 12.5px/1.75 var(--font-mono)',
        color: 'var(--star-400)', height: height || 'auto', overflow: 'auto',
      }}>
        {lines.map((ln, i) => {
          const isCmd = ln.type === 'command' || ln.type === 'cmd';
          const spec = LINE[ln.type] || {};
          const isLast = i === lines.length - 1;
          if (isCmd) {
            return (
              <div key={i} style={{ whiteSpace: 'pre-wrap', display: 'flex', gap: 8 }}>
                <span style={{ color: 'var(--gold-500)', userSelect: 'none' }}>{ln.prompt || prompt}</span>
                <span style={{ color: 'var(--star-100)', flex: 1 }}>
                  {ln.text}
                  {cursor && isLast && (
                    <span style={{ display: 'inline-block', width: 8, height: '1.05em', marginLeft: 3, verticalAlign: 'text-bottom', background: 'var(--gold-500)', animation: 'atlas-blink 1.1s steps(1) infinite' }} />
                  )}
                </span>
              </div>
            );
          }
          return (
            <div key={i} style={{ whiteSpace: 'pre-wrap', color: spec.color || 'var(--star-400)' }}>
              {spec.glyph && <span style={{ color: spec.gc }}>{spec.glyph}</span>}
              {ln.type === 'comment' ? <span style={{ color: 'var(--star-950)' }}>{ln.text}</span> : ln.text}
            </div>
          );
        })}
        {cursor && (lines.length === 0 || lines[lines.length - 1].type !== 'command') && (
          <div style={{ display: 'flex', gap: 8 }}>
            <span style={{ color: 'var(--gold-500)', userSelect: 'none' }}>{prompt}</span>
            <span style={{ display: 'inline-block', width: 8, height: '1.05em', background: 'var(--gold-500)', animation: 'atlas-blink 1.1s steps(1) infinite' }} />
          </div>
        )}
      </div>
    </div>
  );
}
