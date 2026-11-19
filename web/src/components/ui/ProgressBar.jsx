// ProgressBar — единый прогресс-бар дизайн-системы: токены, контекстный цвет
// (variant: accent/success/warning/danger), анимированные полосы, % и подпись.
// value=null — неопределённый режим (полосатая анимация), value 0..100 —
// детерминированный. Анимации уважают prefers-reduced-motion (глушатся в CSS).
export default function ProgressBar({
  value,
  variant = 'accent',
  label,
  showPercent = false,
  size = 'md',
  striped = true,
  className = '',
}) {
  const indeterminate = value === null || value === undefined || !isFinite(Number(value));
  const pct = indeterminate ? 0 : Math.min(100, Math.max(0, Math.round(Number(value))));
  const cls = [
    'pb',
    'pb-' + (['accent', 'success', 'warning', 'danger'].includes(variant) ? variant : 'accent'),
    size === 'sm' || size === 'lg' ? 'pb-' + size : '',
    indeterminate ? 'pb-indet' : '',
    striped && !indeterminate ? 'pb-striped' : '',
    className,
  ].filter(Boolean).join(' ');

  return (
    <div className={cls} role="progressbar"
      aria-valuemin={0} aria-valuemax={100}
      aria-valuenow={indeterminate ? undefined : pct}
      aria-label={label || undefined}>
      <div className="pb-track">
        <div className="pb-fill" style={indeterminate ? undefined : { width: pct + '%' }} />
      </div>
      {(showPercent || label) && (
        <div className="pb-meta">
          {label && <span className="pb-label">{label}</span>}
          {showPercent && !indeterminate && <span className="pb-pct mono">{pct}%</span>}
        </div>
      )}
    </div>
  );
}
