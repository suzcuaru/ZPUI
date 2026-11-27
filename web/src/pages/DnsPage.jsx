import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import Switch from '../components/ui/Switch';
import DnsCustomModal from '../components/DnsCustomModal';
import { useConfirm } from '../components/ui/ConfirmDialog';
import { api, apiCacheGet, apiCachePut } from '../api';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';
import { isValidDnsIp, isIPv6Addr } from '../utils';
import { errorText } from '../errors';
import { logError } from '../frontlog';
import { useToast } from '../context/ToastContext';
import { subscribeEvent } from '../events';
import { Plus, Trash2, Server, Network, User, PenLine, ShieldCheck, RefreshCw } from 'lucide-react';
import Spinner from '../components/ui/Spinner';
import StatusHero from '../components/ui/StatusHero';
import { formatSyncTime } from '../utils';

// Модель провайдеров: бэкенд сводит базовых провайдеров и пользовательские
// записи в один системный список. Тумблеры оптимистичные: клик мгновенно
// красит состояние, применение идёт в фоне, ошибка откатывает состояние
// (свежий конфиг приходит в ответе каждого POST). IPv6-DNS не поддерживается:
// в систему прописываются только IPv4-адреса.
// Страница — высокая, панели в одну колонку: базовые провайдеры (включая
// Xbox DNS), пользовательские записи, затем системные DNS и статический IP.
const normCfg = (d) => ({
  enabled: d?.enabled === true,
  xbox_enabled: d?.xbox_enabled === true,
  // Взаимная блокировка источников: включённые провайдеры блокируют
  // пользовательские записи (locked_custom) и наоборот (locked_providers).
  locked_custom: d?.locked_custom === true,
  locked_providers: d?.locked_providers === true,
  // Снимок доступности серверов (health) нужен индикаторам провайдеров;
  // blocked/reason — провайдер выключен или авто-заблокирован на сервере:
  // виден в списке, включить нельзя.
  health: d?.health || null,
  providers: Array.isArray(d?.providers) ? d.providers.map((p) => ({
    id: String(p?.id || ''),
    name: String(p?.name || ''),
    enabled: p?.enabled === true,
    ipv4: Array.isArray(p?.ipv4) ? p.ipv4 : [],
    doh: String(p?.doh || ''),
    blocked: p?.blocked === true,
    reason: String(p?.reason || ''),
  })) : [],
  custom: Array.isArray(d?.custom) ? d.custom.map((c) => ({
    id: String(c?.id || ''),
    name: String(c?.name || ''),
    primary: String(c?.primary || ''),
    secondary: String(c?.secondary || ''),
    doh: String(c?.doh || ''),
    enabled: c?.enabled === true,
    system: c?.system === true,
  })) : [],
});

export default function DnsPage({ status }) {
  const showToast = useToast();
  const { t } = useT();
  const confirm = useConfirm();
  const [cfg, setCfg] = useState(null);
  const [current, setCurrent] = useState(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [editEntry, setEditEntry] = useState(null);
  const [busy, setBusy] = useState(false);
  const [staticBusy, setStaticBusy] = useState(false);
  // Метаданные системного списка провайдеров (version/synced_at из
  // GET /api/dns/remote): показываются в панели базовых провайдеров.
  // Поллит не нужно — сетевой pull на каждый тик лишний; обновляется
  // на монтировании и после принудительного обновления списков.
  const [sysMeta, setSysMeta] = useState(null);
  const [sysRefreshing, setSysRefreshing] = useState(false);
  // Строка, чьё применение DNS идёт прямо сейчас (scope от бэкенда):
  // тумблер блокируется, рядом крутится спиннер «Включение/Отключение…».
  const [pending, setPending] = useState(null);
  const busyRef = useRef(false);
  // Момент последнего успешного применения: статус /api/status может
  // запаздывать, поэтому сразу после POST верим конфигу, а не dnsUi.
  const touchedAtRef = useRef(0);

  const load = useCallback(async () => {
    if (busyRef.current) return;
    const cached = apiCacheGet('/api/dns/config');
    if (cached) setCfg(normCfg(cached));
    const d = await api('GET', '/api/dns/config');
    if (d && !d.error) {
      apiCachePut('/api/dns/config', d);
      setCfg(normCfg(d));
    }
  }, []);

  // Серверный список теперь живёт в dns-system.json (синк с сервером заменяет
  // файл целиком) и приходит через /api/dns/config — отдельный «remote»-запрос
  // больше не нужен.
  const loadCurrent = useCallback(async () => {
    if (document.hidden) return;
    const cached = apiCacheGet('/api/dns/current');
    if (cached) setCurrent(cached);
    const d = await api('GET', '/api/dns/current');
    if (d && !d.error) {
      apiCachePut('/api/dns/current', d);
      setCurrent(d);
    }
  }, []);

  useEffect(() => { load(); loadCurrent(); }, [load, loadCurrent]);
  // Внешние изменения DNS (руками в Windows / другим ПО) — состояние гаснет.
  // Тот же поллинг держит свежим факт статического IP (static_ip в ответе).
  usePolling(loadCurrent, 12000);
  usePolling(load, 12000);

  // Итог асинхронного применения DNS (бэкенд): снимаем pending, показываем
  // факт; ошибка — тостом.
  useEffect(() => subscribeEvent('dns:apply', (d) => {
    setPending(null);
    if (d && d.phase === 'failed') {
      showToast(errorText(t, d.error || ''), 'error');
    }
    load();
    loadCurrent();
  }), [load, loadCurrent, showToast, t]);

  // Бэкенд сам выключил источник (взаимная блокировка с другим включённым
  // источником) — перечитываем конфиг, тумблеры и блокировки обновятся.
  useEffect(() => subscribeEvent('xboxdns:source_auto_off', () => load()), [load]);

  const loadMeta = useCallback(async () => {
    const d = await api('GET', '/api/dns/remote');
    if (d && !d.error) {
      setSysMeta({ version: Number(d.version) || 0, syncedAt: String(d.synced_at || '') });
    }
  }, []);

  useEffect(() => { loadMeta(); }, [loadMeta]);

  // Принудительное обновление системных списков (DNS + исключения) с сервера,
  // затем перечитываем всё, от чего зависит страница.
  const refreshLists = async () => {
    if (sysRefreshing) return;
    setSysRefreshing(true);
    const r = await api('POST', '/api/lists/refresh', {});
    setSysRefreshing(false);
    if (r && r.error) {
      logError('dns', r.error, '/api/lists/refresh');
      showToast(errorText(t, r.error), 'error');
    } else {
      showToast(t('common.listsRefreshed'), 'success');
    }
    load();
    loadCurrent();
    loadMeta();
  };

  // POST + мгновенный отклик: успешный ответ несёт свежий конфиг. Запросы не
  // отбрасываются, а выстраиваются в цепочку: раньше второй тумблер во время
  // первого запроса молча терялся (busyRef), а ответ первого затирал его
  // оптимистичное состояние — «включался только один».
  const pushTailRef = useRef(Promise.resolve());
  const pushDepthRef = useRef(0);
  const push = useCallback((route, body, scope) => {
    const run = async () => {
      pushDepthRef.current++;
      busyRef.current = true; // фоновый load не перетирает оптимистичные тумблеры
      try {
        const r = await api('POST', route, body);
        if (r && r.error) {
          logError('dns', r.error, route);
          if (r.error === 'dns_busy') showToast(t('dns.applyingBusy'), 'error');
          else if (r.error === 'dns_unreachable') showToast(t('dns.unreachable'), 'error');
          else if (r.error === 'source_locked') showToast(t('dns.sourceLocked'), 'error');
          else showToast(errorText(t, r.error), 'error');
          setPending(null);
          await load();
          loadCurrent();
          return false;
        }
        if (r && r.applying) {
          // Применение ушло в фон: строка остаётся в «Включение…» до события
          // dns:apply; оптимистичное состояние не трогаем.
          setPending(scope || r.scope || null);
          loadCurrent();
          return true;
        }
        if (r && r.status === 'ok' && r.providers) {
          touchedAtRef.current = Date.now();
          apiCachePut('/api/dns/config', r);
          setCfg(normCfg(r));
        }
        setPending(null);
        loadCurrent();
        return true;
      } finally {
        pushDepthRef.current--;
        if (pushDepthRef.current === 0) busyRef.current = false;
      }
    };
    const p = pushTailRef.current.then(run, run);
    pushTailRef.current = p.catch(() => {});
    return p;
  }, [load, loadCurrent, showToast, t]);

  // Включение источника при выключенном мастере неявно включает мастер
  // (так же ведёт себя бэкенд: источники есть → master=true). Оптимистично
  // поднимаем мастер вместе с тумблером — иначе переключатель мигал.
  const toggleProvider = (p) => {
    setCfg((c) => ({
      ...c,
      enabled: c.enabled || !p.enabled,
      providers: c.providers.map((x) => (x.id === p.id ? { ...x, enabled: !p.enabled } : x)),
    }));
    setPending('provider:' + p.id);
    push('/api/dns/provider/toggle', { id: p.id, on: !p.enabled }, 'provider:' + p.id);
  };

  const toggleCustom = (entry) => {
    setCfg((c) => ({
      ...c,
      enabled: c.enabled || !entry.enabled,
      custom: c.custom.map((x) => (x.id === entry.id ? { ...x, enabled: !entry.enabled } : x)),
    }));
    setPending('custom:' + entry.id);
    push('/api/dns/custom/toggle', { id: entry.id, on: !entry.enabled }, 'custom:' + entry.id);
  };

  const saveCustom = async (data) => {
    setModalOpen(false);
    await push('/api/dns/custom/save', data);
  };

  const removeCustom = async (entry) => {
    const ok = await confirm({
      title: t('dns.deleteTitle'),
      message: t('dns.deleteConfirm', { name: entry.name }),
      variant: 'danger',
      confirmText: t('common.delete'),
    });
    if (!ok) return;
    // Оптимистично убираем строку; ошибка вернёт конфиг из ответа/GET.
    setCfg((c) => ({ ...c, custom: c.custom.filter((x) => x.id !== entry.id) }));
    setPending('custom:' + entry.id);
    push('/api/dns/custom/delete', { id: entry.id }, 'custom:' + entry.id);
  };

  // Статический IP — факт адаптера (EnableDHCP реестра): включение фиксирует
  // текущий IPv4/DNS как статические, выключение возвращает DHCP.
  // Предупреждение при включении — только на ноутбуках (есть аккумулятор):
  // на стационарном ПК статика включается без подтверждения.
  const isLaptop = status?.power_battery?.has !== false;
  const toggleStatic = async () => {
    if (staticBusy) return;
    const target = !(current?.static_ip === true);
    if (target && isLaptop) {
      const ok = await confirm({
        title: t('dns.staticConfirmTitle'),
        message: t('dns.staticConfirmText'),
        confirmText: t('common.enable'),
        variant: 'warning',
      });
      if (!ok) return;
    }
    setStaticBusy(true);
    const r = await api('POST', '/api/socks/static-ip', { on: target });
    setStaticBusy(false);
    if (r?.error) {
      showToast(r.error, 'error');
      return;
    }
    showToast(target ? t('socks.staticApplied') : t('socks.staticRestored'), 'success');
    loadCurrent();
  };

  const openAdd = () => { setEditEntry(null); setModalOpen(true); };
  const openEdit = (entry) => { setEditEntry(entry); setModalOpen(true); };

  const sysAddrs = useMemo(
    () => [...(current?.ipv4 || []), ...(current?.ipv6 || [])].map((a) => String(a).trim().toLowerCase()),
    [current]
  );

  // Контакты в панели состояния показываем ЧЕЛОВЕЧЕСКИМИ именами: адрес,
  // принадлежащий известному провайдеру/записи, подписывается именем
  // (IP — в тултипе). Незнакомые адреса (роутер по DHCP и пр.) — как есть.
  const addrName = useMemo(() => {
    const m = new Map();
    if (!cfg) return m;
    for (const p of cfg.providers) {
      for (const ip of p.ipv4) m.set(String(ip).trim().toLowerCase(), t('dns.provider.' + p.id));
    }
    for (const c of cfg.custom) {
      for (const ip of [c.primary, c.secondary]) {
        if (ip) m.set(String(ip).trim().toLowerCase(), c.name);
      }
    }
    return m;
  }, [cfg, t]);

  const enabledAddrs = useMemo(() => {
    if (!cfg) return [];
    const out = [];
    for (const p of cfg.providers) {
      if (!p.enabled) continue;
      out.push(...p.ipv4);
    }
    for (const c of cfg.custom) {
      if (!c.enabled) continue;
      out.push(c.primary, c.secondary);
    }
    return out.filter((a) => isValidDnsIp(a) && !isIPv6Addr(a))
      .map((a) => a.trim().toLowerCase());
  }, [cfg]);

  const appliedOk = useMemo(() => {
    if (!cfg?.enabled || enabledAddrs.length === 0) return false;
    return enabledAddrs.every((a) => sysAddrs.includes(a));
  }, [cfg, enabledAddrs, sysAddrs]);

  // Доступность источников по последнему снимку health: имя провайдера
  // светится зелёным, если его серверы отвечают, красным — если нет;
  // тумблер мёртвого источника заблокирован.
  const healthMap = useMemo(() => {
    const m = new Map();
    for (const s of cfg?.health?.servers || []) {
      if (s?.server) m.set(String(s.server).trim().toLowerCase(), s.ok === true);
    }
    return m;
  }, [cfg]);

  // 'ok' — отвечает хотя бы один адрес, 'dead' — все не отвечают,
  // null — данных ещё нет (источник не успели проверить).
  const srcState = useMemo(() => {
    return (ips) => {
      const known = (ips || [])
        .map((ip) => String(ip).trim().toLowerCase())
        .filter((ip) => healthMap.has(ip));
      if (!known.length) return null;
      return known.some((ip) => healthMap.get(ip)) ? 'ok' : 'dead';
    };
  }, [healthMap]);

  const stateClass = (st) => (st === 'ok' ? ' ok' : st === 'dead' ? ' dead' : '');

  if (!cfg) {
    return (
      <div className="dns-page">
        <div className="dns-hero skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '45%' }} /></div>
        <div className="section skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '70%' }} /></div>
        <div className="section skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '70%' }} /></div>
      </div>
    );
  }

  // hero: мастер = конфиг + ФАКТ применения из /api/dns/current (active —
  // наш DNS стоит на адаптере; источник живее /api/status, который отстаёт).
  // Сразу после своего применения (окно свежести) верим конфигу.
  // Пока факт ещё не загружен (netsh отвечает с задержкой), верим только
  // конфигу: иначе герой показывает «выключен» и перещёлкивается после
  // загрузки факта.
  const appliedNow = current?.active === true;
  const fresh = Date.now() - touchedAtRef.current < 8000;
  const on = cfg.enabled && (current == null ? true : (appliedNow || fresh));

  const stateLabel = on ? t('dns.stateUser') : t('dns.stateOff');

  // Взаимная блокировка источников: пока включены пользовательские записи,
  // тумблеры провайдеров заблокированы (и наоборот).
  const lockedCustom = cfg.locked_custom === true;
  const lockedProviders = cfg.locked_providers === true;

  return (
    <div className="dns-page stagger">
      <StatusHero on={on} icon={<Network size={24} strokeWidth={2} />} title={t('dns.title')} desc={stateLabel}>
        {/* Чип применения показываем только когда факт загружен: до того
            он мигал «Не применён» перед «Применён» */}
        {on && current != null && (
          <span
            className={'dns-apply-chip' + (appliedOk ? ' ok' : '')}
            data-tooltip={t('dns.applyStateTip')}
          >
            {t(appliedOk ? 'dns.applied' : 'dns.notApplied')}
          </span>
        )}
      </StatusHero>

      {/* Состояние: провайдеры в системе + статический IP — одна панель сверху.
          Пока факт не загружен (netsh отвечает с задержкой) — скелетоны:
          интерфейс не прыгает в момент появления данных. */}
      <div className="dns-status">
        <div className="dns-status-col">
          <div className="dns-section-head">
            <Server size={14} strokeWidth={2} />
            <span className="section-title">{t('dns.currentTitle')}</span>
          </div>
          <div className="dns-current">
            {current == null ? (
              <>
                <span className="dns-current-skel" />
                <span className="dns-current-skel" style={{ width: 64 }} />
              </>
            ) : (() => {
              // Только известные провайдеры/записи: адрес роутера по DHCP
              // и прочий «мусор» в панель не попадает. Имя — чип, адреса — тултип.
              const byName = new Map();
              for (const a of sysAddrs) {
                const label = addrName.get(a);
                if (!label) continue;
                if (!byName.has(label)) byName.set(label, []);
                byName.get(label).push(a);
              }
              const pills = [...byName.entries()];
              if (pills.length === 0) {
                return <span className="dns-current-none">{t('dns.currentNone')}</span>;
              }
              return pills.map(([label, ips]) => (
                <span key={label} className="dns-current-addr mono" data-tooltip={ips.join(', ')}>
                  {label}
                </span>
              ));
            })()}
          </div>
        </div>
        <div className="dns-status-col">
          <div className="dns-switch-row" style={{ marginTop: 0 }}>
            <div>
              <div className="dns-switch-label">{t('dns.staticLabel')}</div>
            </div>
            <Switch size="sm" checked={current?.static_ip === true} disabled={current == null} loading={staticBusy} onChange={toggleStatic} />
          </div>
          {current == null ? (
            <div className="dns-static-skel" style={{ width: '72%' }} />
          ) : (
            <div className={'dns-section-note mono' + (current?.static_ip === true ? ' ok' : '')}>
              {t('dns.staticNet', { ip: current?.ip || '—', subnet: current?.subnet || '—' })}
            </div>
          )}
        </div>
      </div>

      <div className="dns-cols">
        {/* DNS: только пользовательские записи — наши сервисы живут в базовых провайдерах */}
        <div className="section dns-section dns-panel">
          <div className="dns-section-head">
            <User size={14} strokeWidth={2} />
            <span className="section-title">{t('dns.customTitle')}</span>
            <button className="btn btn-sm dns-add-btn" onClick={openAdd}>
              <Plus size={12} strokeWidth={2.4} />
              {t('dns.add')}
            </button>
          </div>

          <div className="dns-panel-list">
            {cfg.custom.map((entry) => {
              const st = srcState([entry.primary, entry.secondary]);
              const dead = st === 'dead';
              return (
                <div key={entry.id} className={'dns-prow dns-prow-wide' + (entry.enabled ? ' on' : '')}>
                  <div className="dns-prow-main">
                    <span
                      className={'dns-prow-name' + stateClass(st)}
                      title={entry.name}
                      data-tooltip={st ? t(st === 'ok' ? 'dns.stateOkTip' : 'dns.stateDeadTip') : undefined}
                    >
                      {entry.name}
                    </span>
                    <span className="dns-prow-addr mono">
                      {[entry.primary, entry.secondary].filter(Boolean).join(' · ')}
                    </span>
                  </div>
                  <div className="dns-prow-actions">
                    {entry.doh && (
                      <span className="dns-badge" data-tooltip={t('dns.dohBadgeTip')}>
                        <ShieldCheck size={10} strokeWidth={2.2} />
                        DoH
                      </span>
                    )}
                    <button className="dns-row-del" aria-label={t('common.edit')} onClick={() => openEdit(entry)}>
                      <PenLine size={12} strokeWidth={2} />
                    </button>
                    <button className="dns-row-del" aria-label={t('common.delete')} onClick={() => removeCustom(entry)}>
                      <Trash2 size={13} strokeWidth={2} />
                    </button>
                    <span className="dns-lock-wrap" data-tooltip={lockedCustom ? t('dns.sourceLocked') : undefined}>
                      <Switch size="sm" checked={entry.enabled} disabled={dead || !!pending || lockedCustom} loading={pending === 'custom:' + entry.id} onChange={() => toggleCustom(entry)} hideState />
                    </span>
                  </div>
                </div>
              );
            })}
            {cfg.custom.length === 0 && <div className="dns-empty">{t('dns.customEmpty')}</div>}
          </div>
        </div>

        {/* Базовые провайдеры: публичные пары (список с сервера) */}
        <div className="section dns-section dns-panel">
          <div className="dns-section-head">
            <Network size={14} strokeWidth={2} />
            <span className="section-title">{t('dns.baseTitle')}</span>
            <span className="dns-head-actions">
              {sysMeta && sysMeta.version > 0 && (
                <span
                  className="dns-sys-meta mono"
                  data-tooltip={t('dns.sysVersionTip', { date: formatSyncTime(sysMeta.syncedAt) })}
                >
                  v{sysMeta.version}{sysMeta.syncedAt ? ` · ${formatSyncTime(sysMeta.syncedAt)}` : ''}
                </span>
              )}
              <button
                className="refresh-icon-btn"
                aria-label={t('common.refresh')}
                onClick={refreshLists}
                disabled={sysRefreshing}
              >
                {sysRefreshing ? <Spinner size={12} /> : <RefreshCw size={12} strokeWidth={2} />}
              </button>
            </span>
          </div>
          <p className="dns-section-sub">{t('dns.baseDesc')}</p>

          <div className="dns-panel-list">
            {cfg.providers.map((p) => {
              // имя: локаль для известных провайдеров, серверное name для
              // подгруженных с сервера (у них ключа локали нет)
              const key = 'dns.provider.' + p.id;
              const localized = t(key);
              const label = localized !== key ? localized : (p.name || p.id);
              const blocked = !!p.blocked;
              const st = srcState(p.ipv4);
              const dead = st === 'dead';
              return (
                <div key={p.id}
                  className={'dns-prow dns-prow-wide' + (p.enabled ? ' on' : '') + (blocked ? ' blocked' : '')}
                  data-tooltip={blocked ? t('dns.blockedTip', { reason: p.reason || '' }) : undefined}>
                  <div className="dns-prow-main">
                    <span
                      className={'dns-prow-name' + stateClass(st)}
                      data-tooltip={st ? t(st === 'ok' ? 'dns.stateOkTip' : 'dns.stateDeadTip') : undefined}
                    >
                      {label}
                      {p.doh && (
                        <span className="dns-badge" data-tooltip={t('dns.dohBadgeTip')}>
                          <ShieldCheck size={10} strokeWidth={2.2} />
                          DoH
                        </span>
                      )}
                      {blocked && (
                        <span className="dns-badge" data-tooltip={t('dns.blockedTip', { reason: p.reason || '' })}>
                          {t('dns.blockedBadge')}
                        </span>
                      )}
                    </span>
                    <span className="dns-prow-addr mono">{p.ipv4.join(' · ')}</span>
                  </div>
                  <span className="dns-lock-wrap" data-tooltip={lockedProviders ? t('dns.sourceLocked') : undefined}>
                    <Switch size="sm" checked={p.enabled} disabled={blocked || dead || !!pending || lockedProviders} loading={pending === 'provider:' + p.id} onChange={() => toggleProvider(p)} hideState />
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      </div>

      <DnsCustomModal
        open={modalOpen}
        entry={editEntry}
        busy={busy}
        onSave={saveCustom}
        onClose={() => setModalOpen(false)}
      />
    </div>
  );
}
