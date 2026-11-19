import { useState } from 'react';
import { api } from '../api';
import { useT } from '../i18n';
import { RefreshCw } from 'lucide-react';

// HvciFixButton — авто-исправление блокировки WinDivert «Целостностью памяти»:
// отключает политику (HVCI) и перезагружает ПК. Показывается на экранах
// критической ошибки при errorCode R-000-HVCI.
export default function HvciFixButton() {
  const { t } = useT();
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [err, setErr] = useState('');

  const fix = async () => {
    setBusy(true);
    const r = await api('POST', '/api/system/fix-hvci');
    setBusy(false);
    if (r && !r.error && r.rebooting) {
      setDone(true);
    } else {
      setErr((r && r.error) || t('hvci.fixFail'));
    }
  };

  if (done) {
    return (
      <div className="hvci-fix hvci-fix-done">
        <RefreshCw size={14} strokeWidth={2.2} />
        <span>{t('hvci.fixDone')}</span>
      </div>
    );
  }
  return (
    <div className="hvci-fix">
      <button className="btn btn-accent btn-sm" disabled={busy} onClick={fix}>
        <RefreshCw size={13} strokeWidth={2.2} />
        {busy ? t('hvci.fixing') : t('hvci.fixBtn')}
      </button>
      <span className="hvci-fix-hint">{t('hvci.fixHint')}</span>
      {err && <span className="hvci-fix-err">{err}</span>}
    </div>
  );
}
