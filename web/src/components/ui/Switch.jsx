export default function Switch({ checked, onChange, label, disabled, loading }) {
  return (
    <label className={'switch-wrap' + (disabled ? ' disabled' : '') + (loading ? ' loading' : '')}>
      <button
        className="switch-track"
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
