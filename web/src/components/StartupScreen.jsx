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
      <div className="startup-bg-shine" />
      <div className="startup-card">
        <div className="startup-logo-wrap">
          <div className="startup-logo-ring" />
          <div className="startup-logo">ZP</div>
        </div>
        <div className="startup-title">ZPUI</div>
        <div className="startup-subtitle">{t('startup.tagline')}</div>

        <div className="startup-steps">
          {STAGES.map((s, i) => {
            const done = i < currentIdx;
            const active = i === currentIdx;
            const visible = i <= currentIdx + 1;
            return (
              <div
                key={s.key}
                className={'startup-step' + (active ? ' active' : '') + (done ? ' done' : '') + (visible ? ' visible' : '')}
                style={{ transitionDelay: visible ? `${(i - currentIdx + 1) * 80}ms` : '0ms' }}
              >
                <div className="startup-step-icon">
                  {done
                    ? <svg className="startup-check" viewBox="0 0 16 16" width="12" height="12"><path d="M3 8l3 3 7-7" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>
                    : active ? <span className="startup-spin">{s.icon}</span> : s.icon}
                  {active && <div className="startup-step-glow" />}
                </div>
                <div className="startup-step-label">{label(s.key)}</div>
                {active && detail() && <div className="startup-step-detail">{detail()}</div>}
              </div>
            );
          })}
        </div>

        <div className="startup-bar-track">
          <div className="startup-bar-fill" style={{ width: `${Math.round(progress * 100)}%` }}>
            <div className="startup-bar-glow" />
          </div>
        </div>

        <div className="startup-percent">{Math.round(progress * 100)}%</div>

        {state?.error && (
          <div className="startup-error">
            <svg viewBox="0 0 16 16" width="12" height="12"><circle cx="8" cy="8" r="7" fill="none" stroke="currentColor" strokeWidth="1.5"/><path d="M8 5v3M8 10.5v.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>
            {state.error}
          </div>
        )}
      </div>
    </div>
  );
}
