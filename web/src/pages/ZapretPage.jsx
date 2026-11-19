import { useState, useEffect } from 'react';
import { api } from '../api';
import { useT } from '../i18n';
import { subscribeEvent } from '../events';
import { Check, Stethoscope } from 'lucide-react';
import Switch from '../components/ui/Switch';
import { useToast } from '../context/ToastContext';
import AutoSwitchToggle from '../components/zapret/AutoSwitchToggle';
import StrategiesTab from '../components/zapret/StrategiesTab';
import ListsTab from '../components/zapret/ListsTab';
import FiltersPanel from '../components/zapret/FiltersPanel';
import { LuaTab } from '../components/zapret/LuaTab';

export default function ZapretPage({ status, onOpenDiagnostics, zapretLoading, onZapretToggle }) {
  const showToast = useToast();
  const { t } = useT();
  const [subtab, setSubtab] = useState('strategies');
  const [skipped, setSkipped] = useState(false);
  const [serviceBusy, setServiceBusy] = useState(false);
  const [operatorName, setOperatorName] = useState('');

  useEffect(() => {
    api('GET', '/api/config').then(c => {
      if (c) setSkipped(c.zapret_skipped === true);
    });
    api('GET', '/api/isp/current').then(d => {
      if (d?.name) setOperatorName(d.name);
    });
  }, []);

  useEffect(() => {
    const handler = (data) => {
      if (data?.name) {
        setOperatorName(data.name);
        showToast(t('zapret.operatorChanged', { name: data.name }), 'info');
      }
    };
    const strategyHandler = (data) => {
      // Во время проверки/подбора стратегий бэкенд переключает стратегии —
      // это не ручное действие, тост не нужен.
      if (data?.source === 'autotest' || data?.source === 'single_test') return;
      if (data?.strategy) {
        showToast(t('zapret.strategySwitched', { strategy: data.strategy }), 'success');
      }
    };
    const offOp = subscribeEvent('operator:changed', handler);
    const offStrat = subscribeEvent('strategy:changed', strategyHandler);
    return () => { offOp(); offStrat(); };
  }, [showToast, t]);

  const handleReenable = async () => {
    await api('POST', '/api/config', { zapret_skipped: false });
    setSkipped(false);
    showToast(t('zapret.reenabled'), 'success');
  };

  if (skipped) {
    return (
      <div className="zapret-skipped-banner">
        <svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="var(--warning)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>
        </svg>
        <div className="zapret-skipped-text">
          <strong>{t('zapret.skippedTitle')}</strong>
          <p>{t('zapret.skippedDesc')}</p>
        </div>
        <button className="btn btn-accent btn-sm" onClick={handleReenable}>{t('zapret.reenable')}</button>
      </div>
    );
  }

  const zRun = status?.zapret?.status === 'running';
  const zVersion = status?.zapret?.version || '—';
  const busy = serviceBusy || zapretLoading;

  return (
    <>
      <div className='zp-page stagger'>
      <div className={'zp-service-bar' + (zRun ? ' on' : '')}>
        <div className="zp-service-status">
          <span className={'zp-status-dot ' + (zRun ? 'on' : 'off')} />
          {/* До первого /api/status состояние движка неизвестно — нейтральное «Нет данных» */}
          <span className="zp-version">v{zVersion}</span>
          {operatorName && (
            <span className="zp-operator">
              {t('zapret.operator')}: {operatorName}
            </span>
          )}
        </div>
        <div className="zp-service-actions">
          <button
            className="btn btn-sm zp-btn-wide"
            onClick={onOpenDiagnostics}
            data-tooltip={t('zapret.diagnosticsTip')}
            data-tooltip-pos="bottom"
          >
            <Stethoscope size={13} />
            {t('zapret.diagnostics')}
          </button>
          <AutoSwitchToggle />
          {/* Тумблер как у остальных вкладок: состояние движка — одним взглядом */}
          <Switch
            size="lg"
            checked={zRun}
            onChange={onZapretToggle}
            loading={busy}
            hideState
          />
        </div>
      </div>

      <div className="subtabs">
        <button className={'subtab' + (subtab === 'strategies' ? ' active' : '')} onClick={() => setSubtab('strategies')}>{t('zapret.strategies')}</button>
        <button className={'subtab' + (subtab === 'lists' ? ' active' : '')} onClick={() => setSubtab('lists')}>{t('zapret.lists')}</button>
        <button className={'subtab' + (subtab === 'lua' ? ' active' : '')} onClick={() => setSubtab('lua')}>{t('zapret.luaTab')}</button>
        <span id="strat-hide-slot" className="subtabs-right" />
      </div>

      <div className="zp-tab-body">
        {subtab === 'strategies' && <StrategiesTab status={status} zRun={zRun} />}
        {subtab === 'lists' && <ListsTab />}
        {subtab === 'lua' && <LuaTab showToast={showToast} />}
      </div>

      {/* Полоса фильтров прижата книзу вкладки и живёт на обеих подвкладках
          (фидбек 04.11: «панель должна быть всегда внизу») */}
      <FiltersPanel />
    </div>
    </>
  );
}
