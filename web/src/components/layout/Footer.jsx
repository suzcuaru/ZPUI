import { useState, useEffect, useLayoutEffect, useMemo, useRef, useCallback } from 'react';
import { formatBytes } from '../../utils';
import { api } from '../../api';
import { useT } from '../../i18n';
import { usePolling } from '../../hooks/usePolling';
import { subscribeEvent } from '../../events';

// Порядок DOM-чипов и порядок скрытия при нехватке места: хост/IP →
// сессионные итоги → скорости → Интернетметр → стратегия.
// «Оператор» и «Сервер» не прячутся никогда.
const CHIP_KEYS = ['op', 'yandex', 'speed', 'totals', 'strat', 'host', 'server'];
const HIDE_ORDER = ['host', 'totals', 'speed', 'yandex', 'strat'];
const FT_GAP = 6;
const FT_PAD = 24;
const FT_SLACK = 2;

export default function Footer({ status, authEnabled }) {
  const { t } = useT();
  const [sync, setSync] = useState(null);
  // Оператор: лучший из доступного — имя → ISP/Org → ASN; пока данных нет —
  // «Определяется…» с пульсирующей точкой. Детектор дорабатывается параллельно,
  // поэтому все поля читаются дефенсивно, «Неизвестно» не показываем никогда.
  const [op, setOp] = useState(null);
  const [opPending, setOpPending] = useState(true);
  // Интернетметр (последний сэмпл из базы): скорость + пинг.
  const [yandex, setYandex] = useState(null);

  // Статус канала ZPUI Sync (как на «Данных и диагностике»): online из
  // /api/sync/info, лёгкий опрос (30 сек), результат только для точки.
  // Канал выключен (тумблер или нет авторизации) — нейтральная точка.
  // Авторизация выключена — канал не работает, опроса нет.
  const checkSync = useCallback(async () => {
    if (!authEnabled) return;
    const s = await api('GET', '/api/sync/info');
    setSync(s && !s.error ? s : { enabled: false, online: false });
  }, [authEnabled]);
  useEffect(() => {
    if (!authEnabled) setSync(null);
  }, [authEnabled]);
  usePolling(checkSync, authEnabled ? 30000 : 0);

  // Оператор: /api/isp/current (имя) + обогащение из /api/isp/history
  // (последняя запись по key несёт isp/org/asn). Обновление — раз в 60с
  // и по событию operator:changed от детектора.
  const loadOperator = useCallback(async () => {
    const cur = await api('GET', '/api/isp/current');
    let next = null;
    if (cur && !cur.error && (cur.key || cur.name)) {
      next = { key: cur.key || '', name: String(cur.name || '').trim() };
      const h = await api('GET', '/api/isp/history');
      if (h && !h.error && Array.isArray(h.operators)) {
        const info = h.operators.find((x) => x && x.key === cur.key) || h.operators[0] || null;
        if (info) {
          next = {
            ...next,
            isp: String(info.isp || '').trim(),
            org: String(info.org || '').trim(),
            asn: String(info.asn || '').trim(),
          };
        }
      }
    }
    setOp(next);
    setOpPending(!next || !(next.name || next.isp || next.org || next.asn));
  }, []);
  usePolling(loadOperator, 60000);
  useEffect(() => subscribeEvent('operator:changed', () => loadOperator()), [loadOperator]);

  // Яндекс-метрики: лёгкий опрос (60с, как Мониторинг по сути чаще не нужен),
  // берём только последний сэмпл. Нет данных — «—», без дёрганья.
  usePolling(useCallback(async () => {
    const d = await api('GET', '/api/yandex/metrics');
    if (d && !d.error) setYandex(d.latest || null);
  }, []), 60000);

  const opLabel = op
    ? (op.name || op.isp || op.org || op.asn || '').trim()
    : '';
  const opTip = op
    ? [op.name, op.isp || op.org, op.asn].filter(Boolean).join(' · ')
    : '';
  const isVpn = !!op && /vpn/i.test([op.key, op.name, op.isp, op.org].filter(Boolean).join(' '));

  const ySpeed = yandex && typeof yandex.speed_mbps === 'number' && isFinite(yandex.speed_mbps)
    ? yandex.speed_mbps.toFixed(1)
    : null;
  const yPing = yandex && typeof yandex.ping_ms === 'number' && isFinite(yandex.ping_ms)
    ? String(Math.round(yandex.ping_ms))
    : null;

  // Точка канала: online → зелёная, требуется вход/проверка канала → жёлтая,
  // канал включён, но связи нет → красная, выключен → нейтральная. Пока
  // данных нет — пульс. auth_state приходит с /api/sync/info (ok/auth_required/
  // relogin): клиент больше не штормит 401-ами, статусы честные.
  const syncOnline = !!sync && sync.online === true;
  const syncOff = !!sync && sync.enabled === false;
  const syncRelogin = !!sync && sync.auth_state === 'relogin';
  const syncAuthReq = !!sync && sync.auth_state === 'auth_required';
  const syncDotCls = !sync
    ? ''
    : (syncRelogin || syncAuthReq)
      ? ' warn'
      : syncOnline ? ' ok' : syncOff ? '' : ' err';
  const syncLabel = syncRelogin
    ? t('sync.stateRelogin')
    : syncAuthReq
      ? t('sync.stateAuthRequired')
      : t('footer.server');
  const syncTip = !sync
    ? t('footer.serverChecking')
    : (syncRelogin || syncAuthReq)
      ? syncLabel
      : syncOnline
        ? t('footer.serverOk')
        : syncOff
          ? t('footer.serverOff')
          : t('footer.serverErr');

  const zRun = status?.zapret?.status === 'running';
  const strategy = status?.zapret?.strategy || '—';
  const mon = status?.monitor || {};
  const dlSpeed = mon.dl_speed_fmt || '0 B/s';
  const ulSpeed = mon.ul_speed_fmt || '0 B/s';
  const dlTotal = formatBytes(mon.download_bytes || 0);
  const ulTotal = formatBytes(mon.upload_bytes || 0);
  const hostname = status?.network?.hostname || '';
  const ips = status?.network?.ips || [];
  const hasStatus = !!status;

  // Скрытие чипов — по фактическому переполнению строки, а не media-запросами:
  // ширины зависят от контента, DPI и зума --ui-k. Естественные ширины снимаем
  // со всех чипов за кадром (useLayoutEffect отрабатывает до отрисовки), затем
  // прячем чипы из HIDE_ORDER, пока строка не влезет в доступное место.
  const rootRef = useRef(null);
  const chipRefs = useRef({});
  const [avail, setAvail] = useState(0);
  const [widths, setWidths] = useState(null);
  const [measuring, setMeasuring] = useState(true);

  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) return undefined;
    const update = () => setAvail(root.clientWidth);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(root);
    return () => ro.disconnect();
  }, [hasStatus]);

  useLayoutEffect(() => {
    if (document.fonts?.ready) document.fonts.ready.then(() => setMeasuring(true));
  }, []);

  useLayoutEffect(() => {
    setMeasuring(true);
  }, [opLabel, opPending, ySpeed, yPing, strategy, hostname, ips[0], authEnabled, sync, hasStatus]);

  useLayoutEffect(() => {
    if (!measuring) return;
    const w = {};
    for (const k of CHIP_KEYS) {
      const el = chipRefs.current[k];
      w[k] = el ? el.offsetWidth : 0;
    }
    setWidths(w);
    setMeasuring(false);
  }, [measuring]);

  const hiddenSet = useMemo(() => {
    const set = new Set();
    if (!widths || avail <= 0) return set;
    const mounted = authEnabled ? CHIP_KEYS.length : CHIP_KEYS.length - 1;
    const inner = avail - FT_PAD - FT_SLACK;
    for (let n = 0; n <= HIDE_ORDER.length; n++) {
      const vis = HIDE_ORDER.slice(n);
      const gaps = mounted - 1 - n;
      let sum = (widths.op || 0) + (widths.server || 0) + gaps * FT_GAP;
      for (const k of vis) sum += widths[k] || 0;
      if (sum <= inner) return new Set(HIDE_ORDER.slice(0, n));
    }
    return new Set(HIDE_ORDER);
  }, [widths, avail, authEnabled]);

  if (!status) return <div className="footer" />;

  const hideCls = (k) => (!measuring && hiddenSet.has(k) ? ' ft-hidden' : '');
  const chipRef = (k) => (el) => { chipRefs.current[k] = el; };

  return (
    <div
      ref={rootRef}
      className={'footer' + (measuring ? ' measuring' : '')}
      onDragStart={(e) => e.preventDefault()}
    >
      <div className="ft-right">
        <div className="ft-chip ft-op" ref={chipRef('op')} data-tooltip={opPending ? t('footer.opDetecting') : opTip} data-tooltip-pos="left">
          {opPending ? (
            <>
              <span className="ft-dot wait" />
              <span className="ft-label">{t('footer.opDetecting')}</span>
            </>
          ) : (
            <>
              <span className="ft-label">{t('footer.operator')}</span>
              {isVpn && <span className="ft-vpn-badge">VPN</span>}
              <span className="ft-val">{opLabel}</span>
            </>
          )}
        </div>

        <div className={'ft-chip ft-yandex' + hideCls('yandex')} ref={chipRef('yandex')} data-tooltip={t('footer.netTip')} data-tooltip-pos="bottom">
          {ySpeed !== null && yPing !== null ? (
            <>
              <span className="ft-val accent ft-y-speed">{ySpeed} {t('monitor.mbps')}</span>
              <span className="ft-y-sep">·</span>
              <span className="ft-val ok ft-y-ping">{yPing} {t('common.ms')}</span>
            </>
          ) : (
            <span className="ft-val">—</span>
          )}
        </div>

        <div className={'ft-chip ft-speed' + hideCls('speed')} ref={chipRef('speed')}>
          <span className="ft-tag dl">DL</span>
          <span className="ft-val accent">{dlSpeed}</span>
          <span className="ft-tag ul">UL</span>
          <span className="ft-val ok">{ulSpeed}</span>
        </div>

        <div className={'ft-chip ft-totals' + hideCls('totals')} ref={chipRef('totals')}>
          <span className="ft-tag muted">DL</span>
          <span className="ft-val">{dlTotal}</span>
          <span className="ft-tag muted">UL</span>
          <span className="ft-val">{ulTotal}</span>
        </div>

        <div className={'ft-chip ft-strat' + hideCls('strat')} ref={chipRef('strat')}>
          <span className="ft-key">{t('footer.strategy')}</span>
          <span className="ft-val" style={{ maxWidth: 140 }}>{strategy}</span>
        </div>

        <div className={'ft-chip ft-host' + hideCls('host')} ref={chipRef('host')}>
          <span className="ft-val">{hostname && `${hostname} · `}{ips[0] || '127.0.0.1'}</span>
        </div>

        {authEnabled && (
          <div className="ft-chip" ref={chipRef('server')} data-tooltip={syncTip} data-tooltip-pos="left">
            <span className={'ft-dot' + syncDotCls} />
            <span className="ft-label">{syncLabel}</span>
          </div>
        )}
      </div>
    </div>
  );
}
