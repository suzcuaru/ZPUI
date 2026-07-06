export default function Switch({ checked, onChange, disabled, label }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      className="switch-track"
      disabled={disabled}
      onClick={() => !disabled && onChange(!checked)}
    >
      <span className="switch-thumb" />
    </button>
  );
}
