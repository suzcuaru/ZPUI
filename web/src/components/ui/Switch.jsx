import { useT } from '../../i18n/index.jsx';
import { sound } from '../../sound';
import Spinner from './Spinner';

export default function Switch({ checked, onChange, label, disabled, loading, pending, hideState, flash, size }) {
  const { t } = useT();
  const sizeClass = size === 'lg' ? ' switch-lg' : '';
  return (
    <label className={'switch-wrap' + sizeClass + (disabled ? ' disabled' : '') + (loading ? ' loading' : '') + (pending ? ' pending' : '')}>
      {!hideState && (
        <span className="switch-state" data-on={checked ? '1' : '0'}>{t(checked ? 'common.on' : 'common.off')}</span>
      )}
      <button
        className={'switch-track' + sizeClass + (pending ? ' pending' : '') + (flash ? ' act-flash-' + flash : '')}
        role="switch"
        aria-checked={!!checked}
        aria-busy={pending || undefined}
        onClick={() => {
          if (disabled || loading || pending) return;
          sound.play('toggle');
          // Новое состояние наружу: обработчик может положиться на аргумент
          // (например, onChange={setter} вместо замыкания).
          onChange(!checked);
        }}
        disabled={disabled || loading || pending}
        tabIndex={0}
      >
        {loading ? (
          <Spinner size={13} tone="white" className="switch-spinner" />
        ) : (
          <span className="switch-thumb" />
        )}
      </button>
      {label && <span className="switch-label">{label}</span>}
    </label>
  );
}
