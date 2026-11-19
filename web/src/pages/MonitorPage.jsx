import { useState, useEffect, useCallback } from 'react';
import { Send } from 'lucide-react';
import { api, apiCacheGet, apiCachePut } from '../api';
import { formatBytes } from '../utils';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';
import PowerPausedNotice from '../components/ui/PowerPausedNotice';
import TrafficChart from '../components/ui/TrafficChart';

const MAX_SAMPLES = 60;

export default function MonitorPage({ status, onNavigate }) {
  const { t } = useT();
  const [liveHistory, setLiveHistory] = useState({ dl: [], ul: [] });
  const [histTimes, setHistTimes] = useState([]);
  const [trafficLoaded, setTrafficLoaded] = useState(false);
  const [devices, setDevices] = useState([]);
  const [devicesLoaded, setDevicesLoaded] = useState(false);
  const [yandex, setYandex] = useState({ hist: [], latest: null });
  const [yandexLoaded, setYandexLoaded] = useState(false);

  const m = status?.monitor || {};
  // Статистика прокси = локальный Telegram-мост (SOCKS5 — см. вкладку «Прокси»).
  const proxyStats = status?.tg_proxy?.stats || {};

  // История трафика живёт в бэкенде (кольцо 1ч, тик 5с). Раньше точки
  // дополнялись по смене объекта status — после дифф-оптимизации статуса
  // сэмплы ложились неравномерно и график «дёргался». Теперь равномерная
  // каденция 5с прямо из кольца; берём последние MAX_SAMPLES точек.
  const applyTraffic = useCallback((r) => {
    if (!r || !Array.isArray(r.samples) || r.samples.length === 0) return;
    apiCachePut('/api/monitor/traffic-history', r);
    const recent = r.samples.slice(-MAX_SAMPLES);
    setLiveHistory({ dl: recent.map((s) => s.dl), ul: recent.map((s) => s.ul) });
    setHistTimes(recent.map((s) => s.ts * 1000));
    setTrafficLoaded(true);
  }, []);

  // Устройства (каждый тик на бэке спавнит arp -a) — редкий опрос 15с;
  // история трафика — живой график, остаётся на 5с.
  const loadDevices = useCallback(async () => {
    const cached = apiCacheGet('/api/monitor/devices');
    if (cached?.devices) setDevices(cached.devices);
    const devs = await api('GET', '/api/monitor/devices');
    if (devs?.devices) {
      apiCachePut('/api/monitor/devices', devs);
      setDevices(devs.devices);
    }
    setDevicesLoaded(true);
  }, []);

  const loadTraffic = useCallback(async () => {
    applyTraffic(await api('GET', '/api/monitor/traffic-history'));
  }, [applyTraffic]);

  // Мгновенная отрисовка из кэша до первого тика опроса.
  useEffect(() => {
    applyTraffic(apiCacheGet('/api/monitor/traffic-history'));
  }, [applyTraffic]);

  // Интернетметр: замеры скорости/пинга идут внутри цикла проверки ресурсов,
  // здесь читаем историю из базы.
  const loadYandex = useCallback(async () => {
    const cached = apiCacheGet('/api/yandex/metrics');
    if (cached && cached.metrics) setYandex({ hist: cached.metrics, latest: cached.latest });
    const d = await api('GET', '/api/yandex/metrics');
    if (d && d.metrics) {
      apiCachePut('/api/yandex/metrics', d);
      setYandex({ hist: d.metrics, latest: d.latest });
    }
    setYandexLoaded(true);
  }, []);

  usePolling(loadTraffic, 5000);
  usePolling(loadDevices, 15000);
  usePolling(loadYandex, 60000);

  const proxyDl = proxyStats.bytes_in || 0;
  const proxyUl = proxyStats.bytes_out || 0;
  const proxyConns = proxyStats.active || 0;
  // Секция устройств — это клиенты локального Telegram-прокси; когда мост
  // выключен, списка не будет — показываем подсказку вместо «пусто».
  const tgProxyOn = status?.tg_proxy?.running === true;

  const paused = status?.power_saving_paused === true;
  // До первого /api/status метрик ещё нет — показываем нейтральное «—»,
  // а не уверенные нули («Остановлен / 0 B»).
  const hasStatus = !!status;
  return (
    <div className={'mon-page stagger' + (paused ? ' pp-greyed' : '')}>
      <div className="mon-grid">
        <div className="card-section mon-chart-cell">
          <div className="mon-chart-header">
            <span className="card-section-title">{t('monitor.trafficTitle')}</span>
            <span className="mon-chart-val mon-chart-vals">
              <span className="mono mon-chart-dl">↓ {hasStatus ? (m.dl_speed_fmt || '0 B/s') : '—'}</span>
              <span className="mono mon-chart-ul">↑ {hasStatus ? (m.ul_speed_fmt || '0 B/s') : '—'}</span>
            </span>
          </div>
          {liveHistory.dl.length > 0 ? (
            <TrafficChart
              height={150}
              metric="rate"
              times={histTimes}
              series={[
                { data: liveHistory.dl, color: 'var(--accent)', label: 'dl' },
                { data: liveHistory.ul, color: 'var(--success)', label: 'ul' },
              ]}
            />
          ) : (
            <div className="mon-chart-skeleton skeleton skeleton-pulse" style={{ height: 150 }}>
              <div className="skeleton-line" />
            </div>
          )}
        </div>

        <div className="card-section mon-chart-cell">
          <div className="mon-chart-header">
            <span className="card-section-title">{t('monitor.meterTitle')}</span>
            <span className="mon-chart-val mono">
              {yandex.latest
                ? `${yandex.latest.speed_mbps.toFixed(1)} ${t('monitor.mbps')} · ${yandex.latest.ping_ms}${t('common.ms')}`
                : '—'}
            </span>
          </div>
          <div className="mon-meter-duo">
            <div className="mon-meter-sub">
              <span className="mon-meter-sub-label">{t('monitor.meterSpeed')}</span>
              {yandex.hist.length > 0 ? (
                <TrafficChart data={yandex.hist.map(x => x.speed_mbps)} color="var(--warning)" label="ysp" height={60} metric="mbps" />
              ) : (
                <div className="mon-chart-skeleton skeleton skeleton-pulse" style={{ height: 60 }}>
                  <div className="skeleton-line" />
                </div>
              )}
            </div>
            <div className="mon-meter-sub">
              <span className="mon-meter-sub-label">{t('monitor.meterPing')}</span>
              {yandex.hist.length > 0 ? (
                <TrafficChart data={yandex.hist.map(x => x.ping_ms)} color="var(--danger)" label="yping" height={60} metric="ms" />
              ) : (
                <div className="mon-chart-skeleton skeleton skeleton-pulse" style={{ height: 60 }}>
                  <div className="skeleton-line" />
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="mon-stats-row">
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.totalDown')}</span>
          <span className="mon-stat-val mono mon-stat-dl">{hasStatus ? (m.download_fmt || '0 B') : '—'}</span>
        </div>
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.totalUp')}</span>
          <span className="mon-stat-val mono mon-stat-ul">{hasStatus ? (m.upload_fmt || '0 B') : '—'}</span>
        </div>
        <div className="mon-stat">
          <span className="mon-stat-label">{t('monitor.viaProxy')}</span>
          <div className="mon-stat-split">
            <div className="mon-stat-split-item">
              <span className="mon-stat-split-tag dl">DL</span>
              <span className="mono">{hasStatus ? formatBytes(proxyDl) : '—'}</span>
            </div>
            <div className="mon-stat-split-item">
              <span className="mon-stat-split-tag ul">UL</span>
              <span className="mono">{hasStatus ? formatBytes(proxyUl) : '—'}</span>
            </div>
          </div>
        </div>
        <div className="mon-stat mon-stat-sm">
          <span className="mon-stat-label">{t('monitor.devices')}</span>
          <span className="mon-stat-val">{devicesLoaded ? devices.length : '—'}</span>
        </div>
      </div>

      <div className="section mon-devices-section">
        <div className="section-title">
          {t('monitor.tgDevicesTitle')}
          {proxyConns > 0 && <span className="mon-device-conns-badge">{proxyConns} {t('monitor.activeConns')}</span>}
        </div>
        {!devicesLoaded ? (
          <div className="mon-devices-skeleton">
            <div className="skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '62%' }} /></div>
            <div className="skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '48%' }} /></div>
            <div className="skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '55%' }} /></div>
          </div>
        ) : devices.length > 0 ? (
          <div className="mon-devices">
            {devices.map(d => (
              <div key={(d.mac || '') + d.ip} className="mon-device">
                <span className="mon-device-dot" />
                <span className="mon-device-name">{d.hostname || d.ip}</span>
                <span className="mon-device-ip">{d.ip}</span>
                {d.tg_connections > 0 && (
                  <span className="mon-device-tg" data-tooltip={t('tgproxy.title')}><Send size={10} strokeWidth={2.2} /> {d.tg_connections}</span>
                )}
                <span className="mon-device-conns">{d.connections || 0}</span>
              </div>
            ))}
          </div>
        ) : (
          <div className="proxy-empty">
            {tgProxyOn
              ? t('monitor.noTgDevices')
              : t('monitor.tgProxyOff')}
          </div>
        )}
      </div>
      {paused && <PowerPausedNotice titleKey="powerPaused.monitorTitle" onNavigate={onNavigate} />}
    </div>
  );
}
