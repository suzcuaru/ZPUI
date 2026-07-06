import { useT } from '../i18n';

const STAGE_LABELS = {
  welcome: 'startup.welcome',
  self_check: 'startup.selfCheck',
  self_download: 'startup.selfDownload',
  mod_check: 'startup.modCheck',
  mod_download: 'startup.modDownload',
  install: 'startup.install',
  restart: 'startup.restart',
};

export default function StartupScreen({ state }) {
  const { t } = useT();
  const stage = state?.stage || 'welcome';
  const sub = state?.sub || '';
  const progress = state?.progress || 0;
  const selfUpdate = state?.self_update;
  const modUpdates = state?.mod_updates || [];

  const detail = () => {
    if (selfUpdate && stage === 'self_download') return selfUpdate.version;
    if (sub) return sub;
    return '';
  };

  const label = STAGE_LABELS[stage] ? t(STAGE_LABELS[stage]) : '';

  return (
    <div className="startup-overlay">
      <div className="startup-rings">
        <div className="startup-ring startup-ring-1" />
        <div className="startup-ring startup-ring-2" />
        <div className="startup-ring startup-ring-3" />
        <div className="startup-icon">
          <svg viewBox="0 0 256 256" width="56" height="56">
            <defs>
              <linearGradient id="sg1" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stopColor="#7c4dff"/>
                <stop offset="100%" stopColor="#ff4081"/>
              </linearGradient>
            </defs>
            <path d="M128 48 L204 80 L204 140 C204 182 172 214 128 234 C84 214 52 182 52 140 L52 80 Z"
                  fill="none" stroke="url(#sg1)" strokeWidth="3"/>
            <path d="M108 116 L148 116 L108 164 L148 164"
                  fill="none" stroke="#fff" strokeWidth="7" strokeLinecap="round" strokeLinejoin="round" opacity="0.9"/>
          </svg>
        </div>
      </div>

      <div className="startup-title">ZPUI</div>
      <div className="startup-subtitle">{t('startup.tagline')}</div>

      <div className="startup-stage">{label}</div>
      {detail() && <div className="startup-detail">{detail()}</div>}

      <div className="startup-bar-track">
        <div className="startup-bar-fill" style={{ width: `${Math.round(progress * 100)}%` }} />
      </div>
      <div className="startup-percent">{Math.round(progress * 100)}%</div>

      {state?.error && <div className="startup-error">{state.error}</div>}
    </div>
  );
}
