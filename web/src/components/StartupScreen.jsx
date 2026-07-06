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
      <div className="startup-center">
        <div className="startup-rings">
          <div className="startup-ring startup-ring-1" />
          <div className="startup-ring startup-ring-2" />
          <div className="startup-ring startup-ring-3" />
          <div className="startup-logo">ZP</div>
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
    </div>
  );
}
