const SIZES = {
  sm: { padding: '8px 13px', font: '12px' },
  md: { padding: '10px 16px', font: '13px' },
  lg: { padding: '12px 20px', font: '14px' },
};

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "lg";
  disabled?: boolean;
  iconLeft?: React.ReactNode;
  iconRight?: React.ReactNode;
  children?: React.ReactNode;
  style?: React.CSSProperties;
}

/**
 * Atlas Button — "gold is the star": the primary variant is the one
 * thing that matters most in a view, so use it once.
 */
export function Button({
  variant = 'primary',
  size = 'md',
  disabled = false,
  iconLeft = null,
  iconRight = null,
  children,
  style = {},
  ...rest
}: ButtonProps) {
  const sz = SIZES[size] || SIZES.md;

  const base: React.CSSProperties = {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
    fontFamily: 'var(--font-sans)',
    fontWeight: 'var(--fw-semibold)',
    fontSize: sz.font,
    lineHeight: 1,
    padding: sz.padding,
    borderRadius: 'var(--radius-md)',
    border: '1px solid transparent',
    cursor: disabled ? 'not-allowed' : 'pointer',
    opacity: disabled ? 0.45 : 1,
    transition: 'background var(--dur-base) var(--ease-standard), border-color var(--dur-base) var(--ease-standard), color var(--dur-base) var(--ease-standard), box-shadow var(--dur-base) var(--ease-standard)',
    whiteSpace: 'nowrap',
    userSelect: 'none',
  };

  const variants: Record<string, React.CSSProperties> = {
    primary: {
      background: 'var(--gold-500)',
      color: 'var(--gold-ink)',
      boxShadow: 'var(--glow-gold)',
    },
    secondary: {
      background: 'transparent',
      color: 'var(--star-400)',
      borderColor: 'var(--hairline-strong)',
    },
    ghost: {
      background: 'transparent',
      color: 'var(--starlight-300)',
    },
    danger: {
      background: 'transparent',
      color: 'var(--signal-red-300)',
      borderColor: 'rgba(239,68,68,0.3)',
    },
  };

  return (
    <button
      type="button"
      disabled={disabled}
      style={{ ...base, ...(variants[variant] || variants.primary), ...style }}
      onMouseEnter={(e) => {
        if (disabled) return;
        if (variant === 'primary') e.currentTarget.style.background = 'var(--gold-300)';
        else if (variant === 'secondary') e.currentTarget.style.borderColor = 'var(--starlight-300)';
        else if (variant === 'ghost') e.currentTarget.style.background = 'var(--starlight-tint)';
        else if (variant === 'danger') e.currentTarget.style.background = 'rgba(239,68,68,0.08)';
      }}
      onMouseLeave={(e) => {
        const v = variants[variant] || variants.primary;
        e.currentTarget.style.background = v.background as string;
        e.currentTarget.style.borderColor = (v.borderColor as string) || 'transparent';
      }}
      {...rest}
    >
      {iconLeft}
      {children}
      {iconRight}
    </button>
  );
}
