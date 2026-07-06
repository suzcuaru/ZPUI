import { useT } from '../i18n';

const STAGES = [
  { key: 'welcome', icon: '◆' },
  { key: 'self_check', icon: '◈' },
  { key: 'self_download', icon: '⬇' },
  { key: 'mod_check', icon: '◇' },
  { key: 'mod_download', icon: '⬇' },
  { key: 'install', icon: '⚙' },
  { key: 'restart', icon: '↻' },
];

export default function StartupScreen({ state }) {
  const { t } = useT();
  const stage = state?.stage || 'welcome';
  const sub = state?.sub || '';
  const progress = state?.progress || 0;
  const selfUpdate = state?.self_update;
  const modUpdates = state?.mod_updates || [];

  const currentIdx = STAGES.findIndex(s => s.key === stage);

  const label = (key) => {
    const m = {
      welcome: t('startup.welcome'),
      self_check: t('startup.selfCheck'),
      self_download: t('startup.selfDownload'),
      mod_check: t('startup.modCheck'),
      mod_download: t('startup.modDownload'),
      install: t('startup.install'),
      restart: t('startup.restart'),
      done: t('startup.done'),
    };
    return m[key] || '';
  };

  const detail = () => {
    if (selfUpdate && stage === 'self_download') {
      return selfUpdate.version;
    }
    if (sub) return sub;
    return '';
  };

  return (
    <div className="startup-overlay">
      <div className="startup-card">
        <div className="startup-logo">ZP</div>
        <div className="startup-title">ZPUI</div>
        <div className="startup-subtitle">{t('startup.tagline')}</div>

        <div className="startup-steps">
          {STAGES.map((s, i) => {
            const done = i < currentIdx;
            const active = i === currentIdx;
            return (
              <div key={s.key} className={'startup-step' + (active ? ' active' : '') + (done ? ' done' : '')}>
                <div className="startup-step-icon">{done ? '✓' : active ? <span className="startup-spin">{s.icon}</span> : s.icon}</div>
                <div className="startup-step-label">{label(s.key)}</div>
              </div>
            );
          })}
        </div>

        {detail() && <div className="startup-detail">{detail()}</div>}

        <div className="startup-bar-track">
          <div className="startup-bar-fill" style={{ width: `${Math.round(progress * 100)}%` }} />
        </div>

        <div className="startup-percent">{Math.round(progress * 100)}%</div>

        {state?.error && <div className="startup-error">{state.error}</div>}
      </div>
    </div>
  );
}
