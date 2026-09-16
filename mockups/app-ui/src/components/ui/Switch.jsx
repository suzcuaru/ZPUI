import { useT } from '../../i18n/index.jsx';

export default function Switch({ checked, onChange, label, disabled, loading, hideState, flash, size }) {
  const { t } = useT();
  const sizeClass = size === 'lg' ? ' switch-lg' : '';
  return (
    <label className={'switch-wrap' + sizeClass + (disabled ? ' disabled' : '') + (loading ? ' loading' : '')}>
      {!hideState && (
        <span className="switch-state" data-on={checked ? '1' : '0'}>{t(checked ? 'common.on' : 'common.off')}</span>
      )}
      <button
        className={'switch-track' + sizeClass + (flash ? ' act-flash-' + flash : '')}
        role="switch"
        aria-checked={!!checked}
        onClick={() => onChange()}
        disabled={disabled || loading}
        tabIndex={0}
      >
        {loading ? (
          <span className="switch-spinner" />
        ) : (
          <span className="switch-thumb" />
        )}
      </button>
      {label && <span className="switch-label">{label}</span>}
    </label>
  );
}
