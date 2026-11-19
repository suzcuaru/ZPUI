import { useState, useEffect, useCallback, useMemo, useRef, Fragment } from 'react';
import { api, apiCacheGet, apiCachePut } from '../api';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';
import { useNow } from '../hooks/useNow';
import { subscribeEvent } from '../events';
import { OverlayPortal } from '../components/ui/Modal';
import { dnsUiState } from '../utils';
import Spinner from '../components/ui/Spinner';
import { useToast } from '../context/ToastContext';
import { AlertTriangle, Gauge, RotateCw } from 'lucide-react';

export default function DashboardPage({ status, onNavigate, onOpenCheckerWithUrl }) {
  const showToast = useToast();
  const { t } = useT();
  const [resources, setResources] = useState(null);
  const [loading, setLoading] = useState(true);
  const [lastCheckUnix, setLastCheckUnix] = useState(null);
  const [checkingNow, setCheckingNow] = useState(false);
  const [cached, setCached] = useState(false);
  const [availStd, setAvailStd] = useState([]);
  const [availUser, setAvailUser] = useState([]);
  const [availKey, setAvailKey] = useState([]);
  const [strategyChanges, setStrategyChanges] = useState([]);
  const [checkIntervalMin, setCheckIntervalMin] = useState(10);
  const [failAction, setFailAction] = useState(null); // { name, reason }
  // YouTube-проверка (DNS-уровень): баннер при подмене адресов резолвером.
  // Порт Telegram-прокси живёт в конфиге (в status поля port нет) — грузим один раз.
  const [tgPort, setTgPort] = useState(null);

  useEffect(() => {
    let alive = true;
    const cached = apiCacheGet('/api/tgproxy/config');
    if (alive && cached?.port) setTgPort(cached.port);
    api('GET', '/api/tgproxy/config').then(d => {
      if (alive && d?.port) {
        apiCachePut('/api/tgproxy/config', d);
        setTgPort(d.port);
      }
    });
    return () => { alive = false; };
  }, []);

  // Опрос статуса ресурсов: каждые 30 сек обычно, 2 сек во время проверки
  const fetchResources = useCallback(async () => {
    const cached = apiCacheGet('/api/resource-status');
    if (cached) {
      setResources(cached);
      setLoading(false);
    }
    const data = await api('GET', '/api/resource-status');
    if (data) {
      apiCachePut('/api/resource-status', data);
      setResources(data);
      setCached(!!data.cached);
      setCheckingNow(!!data.checking_now);
      if (data.checked_at_unix) {
        setLastCheckUnix(data.checked_at_unix * 1000);
      }
      if (data.resource_check_interval) {
        setCheckIntervalMin(data.resource_check_interval);
      }
    }
    setLoading(false);
    return data;
  }, []);

  // Замеры Интернетметра (последний час, из базы) — тултип графика
  // доступности показывает скорость/пинг в точке проверки.
  const [netHist, setNetHist] = useState([]);

  // Опрос истории из БД: каждые 60 сек (окно 1 час)
  const fetchAvailHistory = useCallback(async () => {
    const routes = {
      std: '/api/availability/history?hours=1&type=standard',
      usr: '/api/availability/history?hours=1&type=user',
      key: '/api/availability/history?hours=1&type=key',
      changes: '/api/zapret/strategy-changes',
      net: '/api/yandex/metrics',
    };
    const cachedStd = apiCacheGet(routes.std);
    const cachedUsr = apiCacheGet(routes.usr);
    const cachedKey = apiCacheGet(routes.key);
    const cachedChanges = apiCacheGet(routes.changes);
    const cachedNet = apiCacheGet(routes.net);
    if (cachedStd?.records) setAvailStd(cachedStd.records);
    if (cachedUsr?.records) setAvailUser(cachedUsr.records);
    if (cachedKey?.records) setAvailKey(cachedKey.records);
    if (cachedChanges?.changes) setStrategyChanges(cachedChanges.changes);
    if (cachedNet?.metrics) setNetHist(cachedNet.metrics);
    const [std, usr, key, changes, net] = await Promise.all([
      api('GET', routes.std),
      api('GET', routes.usr),
      api('GET', routes.key),
      api('GET', routes.changes),
      api('GET', routes.net),
    ]);
    apiCachePut(routes.std, std);
    apiCachePut(routes.usr, usr);
    apiCachePut(routes.key, key);
    apiCachePut(routes.changes, changes);
    apiCachePut(routes.net, net);
    if (std?.records) setAvailStd(std.records);
    if (usr?.records) setAvailUser(usr.records);
    if (key?.records) setAvailKey(key.records);
    if (changes?.changes) setStrategyChanges(changes.changes);
    if (net?.metrics) setNetHist(net.metrics);
  }, []);

  // Опрос: 30 сек обычно, 2 сек пока идёт проверка; история — раз в минуту,
  // но во время проверки каждые 5 сек: бэкенд пишет записи доступности в базу
  // по мере готовности каждого типа — график наполняется в реальном времени.
  // Скрытое окно — пауза (внутри usePolling), по возвращении — догоняющий вызов.
  usePolling(fetchResources, checkingNow ? 2000 : 30000);
  usePolling(fetchAvailHistory, checkingNow ? 5000 : 60000);

  // Проверка завершилась — история перечитывается немедленно, не дожидаясь
  // следующего планового опроса.
  const prevCheckingRef = useRef(null);
  useEffect(() => {
    if (prevCheckingRef.current === true && checkingNow === false) {
      fetchAvailHistory();
    }
    prevCheckingRef.current = checkingNow;
  }, [checkingNow, fetchAvailHistory]);

  // Серии графика — стабильная идентичность: иначе новый массив литералом
  // на каждом рендере пересчитывал бы пути SVG без реальных изменений.
  const chartSeries = useMemo(() => ([
    { records: availKey, color: 'var(--warning)', label: t('dashboard.keyResources'), noFill: true },
    { records: availStd, color: 'var(--accent)', label: t('dashboard.standard') },
    { records: availUser, color: 'var(--success)', label: t('dashboard.custom') },
  ]), [availKey, availStd, availUser, t]);

  // Когда проверка завершена (checked_at_unix изменился) — сразу обновляем график
  const prevCheckedAtRef = useRef(null);
  useEffect(() => {
    if (lastCheckUnix !== null) {
      if (prevCheckedAtRef.current !== null && prevCheckedAtRef.current !== lastCheckUnix) {
        fetchAvailHistory();
      }
      prevCheckedAtRef.current = lastCheckUnix;
    }
  }, [lastCheckUnix, fetchAvailHistory]);

  // Ручная проверка: запускает async-проверку на бэке, кнопка блокируется
  // пока checking_now=true (фронтенд опрашивает каждые 2 сек)
  const handleRecheck = async () => {
    if (checkingNow) return;
    setCheckingNow(true);  // сразу блокируем кнопку
    try {
      const data = await api('GET', '/api/resource-status/refresh');
      if (data?.error) {
        showToast?.(data.error, 'error');
      }
      // Не ждём завершения — useEffect с checkingNow будет опрашивать каждые 2с
      // и сам разблокирует кнопку когда checking_now станет false
    } catch {
      showToast?.(t('dashboard.recheckFailed'), 'error');
    }
  };

  const zRun = status?.zapret?.status === 'running';
  const tgRun = status?.tg_proxy?.running === true;
  const pRun = tgRun;
  const tgPortShown = status?.tg_proxy?.port || tgPort || 1443;
  const xRun = status?.xbox_dns?.enabled === true;
  // Единое DNS-состояние (то же поле status.xbox_dns, что и в сайдбаре/hero).
  const dnsUi = dnsUiState(status?.xbox_dns);

  // До первого ответа /api/status состояние неизвестно — не показываем
  // уверенное «Остановлен», только нейтральное «—». Первый успешный ответ
  // приходит ещё на стартовом экране (App опрашивает /api/status с монтирования),
  // поэтому пилюли заполняются сразу после загрузки дашборда.
  const svcState = (run) => (status ? (run ? t('status.running') : t('status.stopped')) : '—');

  // Клавиатурная навигация для кликабельных svc-card (role="button").
  const svcKey = (page) => (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onNavigate(page);
    }
  };

  const defRes = resources?.default || [];
  const userRes = resources?.user || [];
  const allRes = [...defRes, ...userRes];

  const defOk = defRes.filter(r => r.ok).length;
  const defPct = defRes.length > 0 ? Math.round(defOk / defRes.length * 100) : -1;
  const userOk = userRes.filter(r => r.ok).length;
  const userPct = userRes.length > 0 ? Math.round(userOk / userRes.length * 100) : -1;

  const blockedRes = allRes.filter(r => !r.ok);

  // Панель «Основные ресурсы» (как при подборе: discord/youtube/steam) —
  // свой опрос тех же деталей, из которых берётся ключевой срез.
  const [keyPanel, setKeyPanel] = useState({ pct: -1, ok: 0, total: 0 });
  const [panelModal, setPanelModal] = useState(null); // { type, title, items, checkedAt }
  const [panelLoading, setPanelLoading] = useState(false);

  const fetchKeyPanel = useCallback(async () => {
    const d = await api('GET', '/api/resources/details?type=key');
    if (d && !d.error && Array.isArray(d.items)) {
      const total = d.items.length;
      const ok = d.items.filter(i => i.ok).length;
      setKeyPanel({ pct: total > 0 ? Math.round(ok / total * 100) : -1, ok, total });
    }
  }, []);
  useEffect(() => { fetchKeyPanel(); }, [fetchKeyPanel]);
  usePolling(fetchKeyPanel, 30000);

  // Панель «Основные» после завершения проверки ресурсов: обновляется по
  // смене checked_at, а не по 30-секундному таймеру — данные не висят старыми.
  const prevCheckedForKeyRef = useRef(null);
  useEffect(() => {
    if (lastCheckUnix !== null) {
      if (prevCheckedForKeyRef.current !== null && prevCheckedForKeyRef.current !== lastCheckUnix) {
        fetchKeyPanel();
      }
      prevCheckedForKeyRef.current = lastCheckUnix;
    }
  }, [lastCheckUnix, fetchKeyPanel]);

  // Проверки, запущенные бэкендом (плановый такт, после смены стратегии):
  // мгновенный статус «проверяем», по завершении — немедленный рефеч графика
  // и панелей (не дожидаясь ближайшего опроса) и синхронное снятие
  // «проверяем»: отсчёт «следующая проверка через N» возвращается от факта
  // done, без ожидания ответа /api/resource-status.
  useEffect(() => {
    const offStarted = subscribeEvent('resource-check:started', () => setCheckingNow(true));
    const offDone = subscribeEvent('resource-check:done', () => {
      setCheckingNow(false);
      fetchResources();
      fetchKeyPanel();
      fetchAvailHistory();
    });
    return () => { offStarted(); offDone(); };
  }, [fetchResources, fetchKeyPanel, fetchAvailHistory]);

  const openPanel = async (type, title) => {
    setPanelModal({ type, title, items: null, checkedAt: 0 });
    setPanelLoading(true);
    const d = await api('GET', `/api/resources/details?type=${type}`);
    setPanelLoading(false);
    if (d && !d.error) {
      setPanelModal({ type, title, items: d.items || [], checkedAt: d.checked_at_unix || 0 });
    } else {
      setPanelModal(null);
      showToast?.(t('dashboard.panelLoadFailed'), 'error');
    }
  };

  const handleSkipResource = async (host) => {
    const r = await api('POST', '/api/zapret/skip-resources/add', { host });
    if (r?.status === 'ok') {
      showToast?.(t('dashboard.addedToSkip', { host }), 'success');
      // Ресурс уходит из панелей сразу, без ожидания следующей проверки.
      setResources((prev) => prev ? {
        ...prev,
        default: (prev.default || []).filter(x => x.name !== host),
        user: (prev.user || []).filter(x => x.name !== host),
      } : prev);
      setPanelModal((pm) => pm ? { ...pm, items: (pm.items || []).filter(x => x.name !== host) } : pm);
      fetchKeyPanel();
    } else if (r?.error) {
      showToast?.(r.error, 'error');
    }
    setFailAction(null);
  };

  const handleCheckResource = (name) => {
    const url = name.startsWith('http') ? name : `https://${name}`;
    setFailAction(null);
    onOpenCheckerWithUrl?.(url);
  };

  return (
    <div className="db-page stagger">
      <div className="page-grid-3">
        <div className={'svc-card' + (zRun ? ' on' : '')} role="button" tabIndex={0} onClick={() => onNavigate('zapret')} onKeyDown={svcKey('zapret')}>
          <div className="svc-card-icon">
            <svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 1L3 5v6c0 5.5 3.8 10.7 9 12 5.2-1.3 9-6.5 9-12V5l-9-4zm-1 14.5L7.5 12 9 10.5l2 2 4-4L16.5 10l-5.5 5.5z"/></svg>
          </div>
          <div className="svc-card-body">
            <span className="svc-card-name">{t('nav.zapret')}</span>
            <span className={'svc-card-state ' + (zRun ? 'on' : 'off')}>
              {svcState(zRun)}
            </span>
            <span className="svc-card-sub mono">{status?.zapret?.strategy?.replace('.bat', '') || '—'}</span>
          </div>
          <span className={'svc-card-dot ' + (zRun ? 'on' : 'off')} />
        </div>

        <div className={'svc-card' + (pRun ? ' on' : '')} role="button" tabIndex={0} onClick={() => onNavigate('proxy')} onKeyDown={svcKey('proxy')}>
          <div className="svc-card-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15 15 0 010 20M12 2a15 15 0 000 20"/></svg>
          </div>
          <div className="svc-card-body">
            <span className="svc-card-name">{t('nav.proxy')}</span>
            <span className={'svc-card-state ' + (pRun ? 'on' : 'off')}>{svcState(pRun)}</span>
            {status && (
              <span className="svc-card-sub svc-card-tg">
                <span className={'svc-card-tg-dot ' + (tgRun ? 'on' : 'off')} />
                {tgRun ? t('dashboard.tgOn', { port: tgPortShown }) : t('dashboard.tgOff')}
              </span>
            )}
          </div>
          <span className={'svc-card-dot ' + (pRun ? 'on' : 'off')} />
        </div>

        <div className={'svc-card' + (xRun ? ' on' : '')} role="button" tabIndex={0} onClick={() => onNavigate('xboxdns')} onKeyDown={svcKey('xboxdns')}>
          <div className="svc-card-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="2" y="3" width="20" height="6" rx="2"/><rect x="2" y="15" width="20" height="6" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
          </div>
          <div className="svc-card-body">
            <span className="svc-card-name">{t('nav.xboxdns')}</span>
            <span className={'svc-card-state ' + (xRun ? 'on' : 'off')}>{svcState(xRun)}</span>
            {dnsUi.on && dnsUi.activeCount > 0 && (
              <span className="svc-card-state dns-count">{t('dashboard.dnsCount', { n: dnsUi.activeCount })}</span>
            )}
          </div>
          <span className={'svc-card-dot ' + (xRun ? 'on' : 'off')} />
        </div>
      </div>

      {/* === Availability panel — redesigned === */}
      <div className="card-section avail-panel">
        <div className="avail-head">
          <div className="avail-title-group">
            <span className="card-section-title">{t('dashboard.resourceAvailability')}</span>
            <AvailClock checkingNow={checkingNow} lastCheckUnix={lastCheckUnix} cached={cached} intervalMin={checkIntervalMin} />
          </div>
          <button
            className={'avail-recheck' + (checkingNow ? ' busy' : '')}
            onClick={handleRecheck}
            disabled={checkingNow}
            data-tooltip={checkingNow ? t('dashboard.checkingHint') : t('dashboard.checkNow')}
            data-tooltip-pos="top"
            aria-label={t('dashboard.checkNow')}
          >
            <svg className="avail-recheck-icon" viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 2v6h-6"/><path d="M3 12a9 9 0 0 1 15-6.7L21 8"/><path d="M3 22v-6h6"/><path d="M21 12a9 9 0 0 1-15 6.7L3 16"/>
            </svg>
          </button>
        </div>

        {/* Chart */}
        <div className="avail-chart-wrap">
          {resources === null ? (
            <div className="avail-chart-skeleton skeleton skeleton-pulse"><div className="skeleton-line" /></div>
          ) : (
            <AvailChart
              series={chartSeries}
              strategyChanges={strategyChanges}
              netHist={netHist}
              t={t}
            />
          )}
        </div>

        {/* Legend + current stats — три кликабельные панели */}
        <div className="avail-legend">
          <button className="avail-chip avail-chip-click" onClick={() => openPanel('standard', t('dashboard.standard'))}>
            <span className="avail-chip-dot" style={{ background: 'var(--accent)' }} />
            <span className="avail-chip-label">{t('dashboard.standard')}</span>
            <span className="avail-chip-val" style={{ color: defPct >= 80 ? 'var(--success)' : defPct >= 50 ? 'var(--warning)' : 'var(--danger)' }}>
              {defPct >= 0 ? `${defPct}%` : '—'}
            </span>
            <span className="avail-chip-count mono">{defOk}/{defRes.length}</span>
          </button>
          <button className="avail-chip avail-chip-click" onClick={() => openPanel('key', t('dashboard.keyResources'))}>
            <span className="avail-chip-dot" style={{ background: 'var(--warning)' }} />
            <span className="avail-chip-label">{t('dashboard.keyResources')}</span>
            <span className="avail-chip-val" style={{ color: keyPanel.pct >= 80 ? 'var(--success)' : keyPanel.pct >= 50 ? 'var(--warning)' : 'var(--danger)' }}>
              {keyPanel.pct >= 0 ? `${keyPanel.pct}%` : '—'}
            </span>
            <span className="avail-chip-count mono">{keyPanel.ok}/{keyPanel.total}</span>
          </button>
          <button className="avail-chip avail-chip-click" onClick={() => openPanel('user', t('dashboard.custom'))}>
            <span className="avail-chip-dot" style={{ background: 'var(--success)' }} />
            <span className="avail-chip-label">{t('dashboard.custom')}</span>
            <span className="avail-chip-val" style={{ color: userPct >= 80 ? 'var(--success)' : userPct >= 50 ? 'var(--warning)' : 'var(--danger)' }}>
              {userPct >= 0 ? `${userPct}%` : '—'}
            </span>
            <span className="avail-chip-count mono">{userOk}/{userRes.length}</span>
          </button>
        </div>
      </div>

      {/* Unavailable resources */}
      <div className="card-section avail-fail-panel">
        <div className="avail-fail-head">
          <span className="card-section-title" style={{ color: blockedRes.length > 0 ? 'var(--danger)' : 'var(--text-tertiary)' }}>
            {t('dashboard.unavailableResources')} ({blockedRes.length})
          </span>
        </div>
        {resources === null ? (
          <div className="avail-fail-skeleton">
            <div className="skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '58%' }} /></div>
            <div className="skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '42%' }} /></div>
          </div>
        ) : blockedRes.length > 0 && !loading ? (
          <div className="avail-fail-scroll">
            <div className="avail-fail-grid">
              {blockedRes.map((r, i) => (
                <button key={i} className="avail-fail-tag" data-tooltip={r.reason || r.verdict} onClick={() => setFailAction({ name: r.name, reason: r.reason || r.verdict })}>
                  {r.name}
                </button>
              ))}
            </div>
          </div>
        ) : (
          <div className="avail-fail-empty">{t('dashboard.noProblems')}</div>
        )}
      </div>

      {failAction && (
        <OverlayPortal>          <div className="modal-overlay" onClick={() => setFailAction(null)}>
            <div className="modal modal-sm fa-modal" role="dialog" aria-modal="true" aria-label={failAction.name} onClick={e => e.stopPropagation()}>
              <div className="modal-header">
                <span className="modal-icon tone-danger"><AlertTriangle size={18} strokeWidth={2} /></span>
                <span className="modal-title">{failAction.name}</span>
              </div>
              <div className="modal-body">
                <div className="fa-modal-reason">{failAction.reason}</div>
              </div>
              <div className="modal-footer">
                <button className="btn btn-sm btn-ghost" onClick={() => setFailAction(null)}>{t('common.close')}</button>
                <button className="btn btn-sm" onClick={() => handleSkipResource(failAction.name)}>
                  {t('dashboard.addToSkip')}
                </button>
                <button className="btn btn-sm btn-accent" onClick={() => handleCheckResource(failAction.name)}>
                  {t('dashboard.checkResource')}
                </button>
              </div>
            </div>
          </div>
        </OverlayPortal>
      )}

      {/* Модалка панели ресурсов: заголовок + дата/время проверки справа,
          ниже — прокручиваемый список ресурсов со статусом и аптаймом. */}
      {panelModal && (
        <OverlayPortal>
          <div className="modal-overlay" onClick={() => setPanelModal(null)}>
            <div className="modal rp-modal" role="dialog" aria-modal="true" aria-label={panelModal.title} onClick={e => e.stopPropagation()}>
              <div className="modal-header">
                <span className="modal-icon tone-accent"><Gauge size={18} strokeWidth={2} /></span>
                <span className="modal-title">{panelModal.title}</span>
                <span className="rp-checked mono">
                  {panelModal.checkedAt > 0
                    ? new Date(panelModal.checkedAt * 1000).toLocaleString([], { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
                    : '—'}
                </span>
              </div>
              <div className="modal-body rp-body">
                {panelLoading || !panelModal.items ? (
                  <div className="rp-loading"><Spinner size={16} /></div>
                ) : panelModal.items.length === 0 ? (
                  <div className="rp-empty">{t('dashboard.panelEmpty')}</div>
                ) : (
                  <Fragment>
                    <div className="rp-head">
                      <span className="rp-h-dot" />
                      <span className="rp-h-name">{t('dashboard.colResource')}</span>
                      <span className="rp-h-uptime" data-tooltip={t('dashboard.colUptimeTip')} data-tooltip-pos="top">{t('dashboard.colUptime')}</span>
                      <span className="rp-h-latency">{t('dashboard.colLatency')}</span>
                    </div>
                    <div className="rp-list">
                    {[...panelModal.items]
                      .sort((a, b) => (b.uptime ?? -1) - (a.uptime ?? -1))
                      .map((it, i) => (
                      <div key={i} className="rp-row">
                        <span className={'rp-dot ' + (it.ok ? 'ok' : 'bad')} />
                        <span className="rp-name-wrap">
                          <span className="rp-name">{it.name}</span>
                          {it.reason && <span className="rp-info">{it.reason}</span>}
                        </span>
                        <span className={'rp-uptime mono' + (it.uptime >= 80 ? ' ok' : it.uptime >= 0 && it.uptime < 50 ? ' bad' : '')}>
                          {it.uptime >= 0 ? `${Math.round(it.uptime)}%` : '—'}
                        </span>
                        <span className="rp-latency mono">{it.latency_ms > 0 ? `${it.latency_ms} ${t('common.ms')}` : '—'}</span>
                      </div>
                    ))}
                  </div>
                  </Fragment>
                )}
              </div>
              <div className="modal-footer">
                <button className="btn btn-sm btn-accent" onClick={() => openPanel(panelModal.type, panelModal.title)}>
                  <RotateCw size={12} strokeWidth={2.2} />
                  {t('dashboard.panelRefresh')}
                </button>
                <button className="btn btn-sm btn-ghost" onClick={() => setPanelModal(null)}>{t('common.close')}</button>
              </div>
            </div>
          </div>
        </OverlayPortal>
      )}
    </div>
  );
}

// AvailClock — статусная строка «проверяем / следующий через MM:SS» с
// тултипом «обновлено N сек назад». Посекундный тик живёт только здесь:
// смена секунды не перерисовывает страницу и не пересчитывает график.
function AvailClock({ checkingNow, lastCheckUnix, cached, intervalMin }) {
  const { t } = useT();
  const now = useNow(1000);

  const fmtAgo = (unixMs) => {
    if (!unixMs) return '—';
    const sec = Math.max(0, Math.floor((now - unixMs) / 1000));
    if (sec < 5) return t('dashboard.justNow');
    if (sec < 60) return t('dashboard.secAgo', { n: sec });
    const min = Math.floor(sec / 60);
    if (min < 60) return t('dashboard.minAgo', { n: min });
    return t('dashboard.hourAgo', { n: Math.floor(min / 60) });
  };

  const intervalMs = Math.max(1000, intervalMin * 60 * 1000);
  const nextCheckMs = Math.ceil(now / intervalMs) * intervalMs;
  const nextInMs = Math.max(0, nextCheckMs - now);
  const nextIn =
    nextInMs >= 60000
      ? `${Math.floor(nextInMs / 60000)}:${String(Math.floor((nextInMs % 60000) / 1000)).padStart(2, '0')}`
      : `${Math.floor(nextInMs / 1000)}${t('dashboard.secShort')}`;

  return (
    <span className={'avail-status ' + (checkingNow ? 'busy' : 'idle')}
      data-tooltip={checkingNow || !lastCheckUnix ? undefined : `${t('dashboard.lastCheckedAt')} ${fmtAgo(lastCheckUnix)}${cached ? ' · ' + t('dashboard.cache') : ''}`}
    >
      {checkingNow ? (
        <><span className="avail-pulse" />{t('dashboard.checking')}</>
      ) : (
        <><span className="avail-live-dot" />{t('dashboard.nextCheck')} <span className="mono avail-status-time">{nextIn}</span></>
      )}
    </span>
  );
}

// AvailChart — график доступности. Шкала X — реальное время: точка рисуется
// по своему времени, окно скользит крупным тиком useNow(30000) (~0.4% ширины
// за шаг — визуально плавно), новые проверки появляются у правого края без
// пересборки всех позиций, которую давала равномерная «шкала по точкам»
// (фидбек 04.11: «график обновляется странно»). Окно «обнимает» данные:
// старт = max(сейчас-1ч, самая старая точка), иначе при редких проверках
// левая половина графика пустовала (фидбек 04.11, третий прогон). Время
// точки видно в тултипе.
// netHist — замеры Интернетметра из того же цикла проверок: тултип
// показывает скорость/пинг ближайшей точки (окно совпадения ±3 мин).
function AvailChart({ series, strategyChanges, netHist, t }) {
  const wrapRef = useRef(null);
  const tipRef = useRef(null);
  const [w, setW] = useState(0);
  const [tipW, setTipW] = useState(0);
  const [hover, setHover] = useState(null); // { x, pts: [{label,color,pct,ts}], strategyChange: {...} }
  const chartNow = useNow(30000);

  // Тултип центрируется на точке (translateX(-50%)); без замера реальной ширины
  // он вылезал за край панели на строчках с длинной скоростью/пингом.
  useEffect(() => {
    if (hover && tipRef.current) setTipW(tipRef.current.offsetWidth);
  }, [hover]);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const cw = entries[0]?.contentRect?.width;
      if (cw) setW(cw);
    });
    ro.observe(el);
    setW(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const H = 176;                  // полная высота SVG
  const padL = 30, padR = 4, padT = 8, padB = 4;
  const plotW = Math.max(0, w - padL - padR);
  const plotH = H - padT - padB;

  const data = useMemo(() => {
    const valid = series.filter((s) => s.records && s.records.length >= 1);
    if (valid.length === 0 || plotW <= 0) return null;

    const tsOf = (r) => (r.timestamp instanceof Date ? r.timestamp.getTime() : new Date(r.timestamp).getTime());

    // Окно: [max(сейчас-1ч, самая старая точка), правый край]. Правый край =
    // «сейчас», пока данные свежие (последняя точка ≤10 мин); если проверки
    // давно не было — окно обнимает данные (правый край чуть правее последней
    // точки): без этого 3 точки кучкуются слева, а плоский хвост через всю
    // ширину выглядит как сломанный график (фидбек 15.11).
    const cur = chartNow || Date.now();
    let gridEnd = cur;
    let oldest = Infinity;
    let lastTs = -Infinity;
    for (const s of valid) {
      for (const r of s.records) {
        const ts = tsOf(r);
        if (!isFinite(ts) || ts > cur) continue;
        if (ts < oldest) oldest = ts;
        if (ts > lastTs) lastTs = ts;
      }
    }
    if (isFinite(lastTs) && cur - lastTs > 10 * 60 * 1000) {
      gridEnd = lastTs + Math.max((lastTs - (isFinite(oldest) ? oldest : lastTs)) * 0.08, 90 * 1000);
    }
    const gridStart = Math.max(gridEnd - 60 * 60 * 1000, isFinite(oldest) ? oldest : gridEnd);
    // Минимальный размах — от вырождения (одна точка: span→0).
    const span = Math.max(gridEnd - gridStart, 10 * 60 * 1000);

    // Шкала X — время: x линейно зависит от ts внутри окна.
    const xOf = (ts) => padL + ((ts - gridStart) / span) * plotW;
    const yOf = (pct) => padT + (1 - Math.max(0, Math.min(100, pct)) / 100) * plotH;

    let totalPts = 0;
    const lines = valid.map((s) => {
      const pts = s.records
        .map((r) => {
          const ts = tsOf(r);
          if (!isFinite(ts) || ts < gridStart || ts > gridEnd) return null;
          return { x: xOf(ts), y: yOf(r.pct), ts, pct: Math.round(r.pct) };
        })
        .filter(Boolean)
        .sort((a, b) => a.ts - b.ts);
      totalPts += pts.length;
      // Линия дотягивается до правого края: окно кончается «сейчас», а последний
      // замер старше на интервал проверки — без хвоста график выглядел оборванным.
      const endX = padL + plotW;
      const lastPt = pts[pts.length - 1];
      const tail = lastPt ? ` L${endX.toFixed(1)},${lastPt.y.toFixed(1)}` : '';
      const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ') + tail;
      const fill = pts.length > 0 ? `${line} L${endX.toFixed(1)},${(padT + plotH).toFixed(1)} L${pts[0].x.toFixed(1)},${(padT + plotH).toFixed(1)} Z` : '';
      return { line, fill, color: s.color, label: s.label, noFill: s.noFill, pts };
    });
    if (totalPts === 0) return null;

    // Маркеры смены стратегий (оранжевые вертикальные штрихи) — в момент
    // смены: смена происходит между точками проверок.
    const stratMarkers = [];
    if (strategyChanges && strategyChanges.length > 0) {
      for (const ch of strategyChanges) {
        const ts = ch.timestamp instanceof Date ? ch.timestamp.getTime() : new Date(ch.timestamp).getTime();
        if (!isFinite(ts) || ts < gridStart || ts > gridEnd) continue;
        stratMarkers.push({
          x: xOf(ts),
          ts,
          strategy: ch.strategy || '',
          source: ch.source || '',
        });
      }
    }

    return { lines, stratMarkers };
  }, [series, strategyChanges, chartNow, plotW]);

  const onMove = (e) => {
    if (!data) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    // ближайшая точка по X
    let bestPt = null, bestDist = Infinity;
    data.lines.forEach((l) => l.pts.forEach((p) => {
      const d = Math.abs(p.x - mx);
      if (d < bestDist) { bestDist = d; bestPt = p; }
    }));
    if (!bestPt) { setHover(null); return; }
    // Строка стратегии — маркер смены стоит на своём времени (между точками
    // проверок), поэтому привязываем к дистанции курсора: в пределах 8px от
    // штриха строка показана, дальше — нет.
    const strategyChange = data.stratMarkers
      ? data.stratMarkers.find((m) => Math.abs(m.x - mx) < 8) || null
      : null;
    const pts = data.lines
      .map((l) => {
        let near = null, nd = Infinity;
        l.pts.forEach((p) => { const d = Math.abs(p.x - bestPt.x); if (d < nd) { nd = d; near = p; } });
        return near ? { label: l.label, color: l.color, pct: near.pct, ts: near.ts } : null;
      })
      .filter(Boolean);
    // Ближайший замер Интернетметра к точке проверки (замер идёт в том же цикле).
    let net = null;
    if (Array.isArray(netHist) && netHist.length > 0) {
      let nd = Infinity;
      for (const m of netHist) {
        const ts = m.timestamp instanceof Date ? m.timestamp.getTime() : new Date(m.timestamp).getTime();
        const d = Math.abs(ts - bestPt.ts);
        if (d < nd) { nd = d; net = { speed: m.speed_mbps, ping: m.ping_ms }; }
      }
      if (nd > 3 * 60 * 1000) net = null;
    }
    setHover({ x: bestPt.x, pts, net, strategyChange });
  };

  if (!data) {
    return (
      <div className="avail-chart-empty" ref={wrapRef}>
        <span>{t ? t('dashboard.noChartData') : '—'}</span>
      </div>
    );
  }

  const fmtTime = (ts) => new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

  return (
    <div className="avail-chart" ref={wrapRef}>
      <svg
        width={w || '100%'} height={H} className="avail-chart-svg"
        onMouseMove={onMove}
        onMouseLeave={() => setHover(null)}
      >
        <defs>
          {data.lines.map((l, i) => (
            <linearGradient key={i} id={`avail-grad-${i}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={l.color} stopOpacity="0.22" />
              <stop offset="100%" stopColor={l.color} stopOpacity="0" />
            </linearGradient>
          ))}
        </defs>

        {/* Горизонтальная сетка Y: 0/50/100 */}
        {[0, 50, 100].map((pct) => {
          const y = padT + (1 - pct / 100) * plotH;
          return (
            <g key={`y${pct}`}>
              <line x1={padL} y1={y} x2={padL + plotW} y2={y} stroke="var(--border-light)" strokeWidth="1" strokeDasharray={pct === 0 || pct === 100 ? '0' : '3 3'} />
              <text x={padL - 6} y={y + 3} textAnchor="end" className="avail-axis-text">{pct}%</text>
            </g>
          );
        })}

        {/* Заливки под синей и зелёной (жёлтая — без заливки, иначе каша) */}
        {data.lines.map((l, i) => (
          l.noFill ? null : <path key={`f${i}`} d={l.fill} fill={`url(#avail-grad-${i})`} />
        ))}
        {data.lines.map((l, i) => (
          <path key={`l${i}`} d={l.line} fill="none" stroke={l.color} strokeWidth="2"
            strokeLinecap="round" strokeLinejoin="round" />
        ))}

        {/* Точки-проверки */}
        {data.lines.map((l, i) => l.pts.map((p, j) => (
          <circle key={`p${i}-${j}`} cx={p.x} cy={p.y} r="2.6" fill="var(--bg-card)" stroke={l.color} strokeWidth="1.8" />
        )))}

        {/* Маркеры смены стратегий (оранжевые вертикальные штрихи) */}
        {data.stratMarkers && data.stratMarkers.map((m, i) => (
          <g key={`sm${i}`}>
            <line x1={m.x} y1={padT} x2={m.x} y2={padT + plotH}
              stroke="var(--warning)" strokeWidth="1.5" strokeDasharray="4 3" opacity="0.8" />
            <circle cx={m.x} cy={padT} r="3" fill="var(--warning)" />
          </g>
        ))}

        {/* Наведение */}
        {hover && (
          <g className="avail-hover">
            <line x1={hover.x} y1={padT} x2={hover.x} y2={padT + plotH} stroke="var(--text-tertiary)" strokeWidth="1" />
            {hover.pts.map((p, i) => {
              const y = padT + (1 - Math.max(0, Math.min(100, p.pct)) / 100) * plotH;
              return <circle key={i} cx={hover.x} cy={y} r="3.5" fill={p.color} stroke="var(--bg-card)" strokeWidth="1.5" />;
            })}
          </g>
        )}
      </svg>

      {/* Тултип при наведении */}
      {hover && (hover.pts.length > 0 || hover.strategyChange) && (() => {
        const half = (tipW || 120) / 2;
        const left = Math.min(Math.max(hover.x, half + 2), Math.max(half + 2, (w || 300) - half - 2));
        return (
        <div className="avail-tooltip" ref={tipRef} style={{ left }}>
          {hover.strategyChange && (
            <span className="avail-tooltip-strat">
              <span className="avail-tooltip-strat-dot" />
              {hover.strategyChange.strategy}
              {hover.strategyChange.source ? ` (${hover.strategyChange.source})` : ''}
            </span>
          )}
          {hover.pts.length > 0 && (
            <>
              <span className="avail-tooltip-time mono">{fmtTime(hover.pts[0].ts)}</span>
              {hover.pts.map((p, i) => (
                <span key={i} className="avail-tooltip-row">
                  <span className="avail-tooltip-dot" style={{ background: p.color }} />
                  {p.label}: <b>{p.pct}%</b>
                </span>
              ))}
              {hover.net && (
                <span className="avail-tooltip-net mono">
                  {t('dashboard.tooltipNet', { speed: hover.net.speed.toFixed(1), ping: Math.round(hover.net.ping) })}
                </span>
              )}
            </>
          )}
        </div>
        );
      })()}
    </div>
  );
}
