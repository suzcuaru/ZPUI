import { useT } from '../../i18n';
import { Minus, X, Sun, Moon, Square, ArrowDownCircle } from 'lucide-react';

// Глиф «восстановить»: две рамки (задняя — частичная), как у Windows.
// Lucide-аналога (Layers/Copy не честен) нет — рисуем inline.
function RestoreGlyph() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="4" y="9" width="11" height="11" rx="1.5" />
      <path d="M9.5 5h8A1.5 1.5 0 0 1 19 6.5v8" />
    </svg>
  );
}

export default function TitleBar({ version, arch, theme, onToggleTheme, maximized, onToggleMaximize, onClose, updateInfo, onOpenUpdate }) {
  const { t } = useT();
  return (
    <div className="tbar">
      <span className="tbar-logo">ZPUI</span>
      {updateInfo && (
        <button className="tbar-update" onClick={onOpenUpdate} aria-label={t('stand.updateAvailable', { v: updateInfo.latest })}>
          <ArrowDownCircle size={14} strokeWidth={2} />
          <span className="tbar-update-ver mono">{updateInfo.latest}</span>
          <span className="tbar-update-label">{t('stand.updateAvailableShort')}</span>
        </button>
      )}
      <div className="tbar-right">
        <span className="tbar-ver mono">{version ? `v${version}${arch ? ` (${arch})` : ''}` : ''}</span>
        <button
          className="tbar-btn"
          onClick={onToggleTheme}
          aria-label={t('stand.themeToggle')}
        >
          {theme === 'dark' ? <Sun size={14} strokeWidth={2} /> : <Moon size={14} strokeWidth={2} />}
        </button>
        <span className="tbar-sep" />
        <div className="tbar-caps">
          <button className="tbar-cap" aria-label={t('stand.minimize')}>
            <Minus size={15} strokeWidth={1.2} />
          </button>
          <button className="tbar-cap" onClick={onToggleMaximize} aria-label={t(maximized ? 'stand.restore' : 'stand.maximize')}>
            {maximized ? <RestoreGlyph /> : <Square size={12} strokeWidth={1.2} />}
          </button>
          <button className="tbar-cap tbar-cap-close" aria-label={t('stand.close')} onClick={onClose}>
            <X size={16} strokeWidth={1.2} />
          </button>
        </div>
      </div>
    </div>
  );
}
