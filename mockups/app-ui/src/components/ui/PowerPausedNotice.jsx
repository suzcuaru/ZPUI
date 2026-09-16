import { BatteryCharging, Settings } from 'lucide-react';
import { useT } from '../../i18n';

// PowerPausedNotice — заглушка по центру вкладки, когда экономный режим
// с настройкой «Отключать мониторинг и синхронизацию» приостановил сервис.
// Контент вкладки серый (см. .pp-greyed), поверх — карточка с переходом в настройки.
export default function PowerPausedNotice({ titleKey, onNavigate }) {
  const { t } = useT();
  return (
    <div className="pp-notice">
      <div className="pp-card">
        <div className="pp-icon"><BatteryCharging size={26} strokeWidth={2} /></div>
        <div className="pp-title">{t(titleKey)}</div>
        <div className="pp-desc">{t('powerPaused.desc')}</div>
        <button className="btn btn-sm" onClick={() => onNavigate('settings')}>
          <Settings size={13} strokeWidth={2} />
          {t('powerPaused.goSettings')}
        </button>
      </div>
    </div>
  );
}
