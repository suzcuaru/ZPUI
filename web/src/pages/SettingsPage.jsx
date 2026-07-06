import { useT } from '../i18n';
import { api } from '../api';
import Switch from '../components/ui/Switch';

export default function SettingsPage({ config, status, onConfigChange, showToast }) {
  const { t, lang, changeLang } = useT();
  const version = status?.mod?.version || '0.0.0';
  const modulesDir = status?.app?.modules_dir || 'modules/';

  const save = (patch) => {
    api('POST', '/api/config', patch);
    onConfigChange(patch);
    showToast(t('toast.saved'), 'success');
  };

  const theme = config?.theme || status?.mod?.theme || 'system';

  return (
    <>
      <div className="page-title">{t('settings.title')}</div>

      <div className="section">
        <div className="section-title">{t('settings.appearance')}</div>
        <div className="set-row">
          <div className="set-row-info">
            <div className="set-row-title">{t('settings.theme')}</div>
          </div>
          <div className="set-theme-row">
            {['system', 'dark', 'light'].map(mode => (
              <button
                key={mode}
                className={'set-theme-btn' + (theme === mode ? ' active' : '')}
                onClick={() => save({ theme: mode })}
              >
                {t('settings.theme' + mode.charAt(0).toUpperCase() + mode.slice(1))}
              </button>
            ))}
          </div>
        </div>
        <div className="set-row">
          <div className="set-row-info">
            <div className="set-row-title">{t('settings.language')}</div>
          </div>
          <div className="set-theme-row">
            {['ru', 'en'].map(l => (
              <button
                key={l}
                className={'set-theme-btn' + (lang === l ? ' active' : '')}
                onClick={() => { changeLang(l); api('POST', '/api/language', { lang: l }); }}
              >
                {l === 'ru' ? 'Русский' : 'English'}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="section">
        <div className="section-title">{t('settings.behavior')}</div>
        <div className="set-row">
          <div className="set-row-info">
            <div className="set-row-title">{t('settings.closeToTray')}</div>
          </div>
          <Switch checked={config?.close_to_tray ?? true} onChange={(v) => save({ close_to_tray: v })} />
        </div>
        <div className="set-row">
          <div className="set-row-info">
            <div className="set-row-title">{t('settings.startMinimized')}</div>
          </div>
          <Switch checked={config?.start_minimized ?? false} onChange={(v) => save({ start_minimized: v })} />
        </div>
        <div className="set-row">
          <div className="set-row-info">
            <div className="set-row-title">{t('settings.autoStartMods')}</div>
          </div>
          <Switch checked={config?.auto_start_mods ?? false} onChange={(v) => save({ auto_start_mods: v })} />
        </div>
        <div className="set-row">
          <div className="set-row-info">
            <div className="set-row-title">{t('settings.verboseLogging')}</div>
            <div className="set-row-desc">{t('settings.verboseLoggingDesc')}</div>
          </div>
          <Switch checked={config?.verbose ?? false} onChange={(v) => { save({ verbose: v }); api('POST', '/api/verbose/logging', { verbose: v }); }} />
        </div>
      </div>

      <div className="section">
        <div className="section-title">{t('settings.about')}</div>
        <div className="set-row">
          <div className="set-row-info"><div className="set-row-title">{t('settings.version')}</div></div>
          <span className="footer-mono" style={{ color: 'var(--text-secondary)' }}>v{version}</span>
        </div>
        <div className="set-row">
          <div className="set-row-info"><div className="set-row-title">{t('settings.modulesDir')}</div></div>
          <span className="footer-mono" style={{ color: 'var(--text-secondary)', fontSize: 10 }}>{modulesDir}</span>
        </div>
      </div>
    </>
  );
}
