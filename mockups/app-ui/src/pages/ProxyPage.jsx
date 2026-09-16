import { useState, useCallback, useRef } from 'react';
import Switch from '../components/ui/Switch';
import PhoneQrModal from '../components/PhoneQrModal';
import { api } from '../api';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';
import { useServiceToggle } from '../hooks/useServiceToggle';
import { useActionFeedback } from '../hooks/useActionFeedback';
import { useDebouncedSave } from '../hooks/useDebouncedSave';
import { formatBytes } from '../utils';
import { Send, Copy, Check, RefreshCw, Settings2, Link2, Smartphone, Lock } from 'lucide-react';

const PENDING_TIMEOUT = 20000;
const PENDING_POLL = 700;

export default function ProxyPage({ status, showToast }) {
  const { t } = useT();
  const [devices, setDevices] = useState([]);
  const [config, setConfig] = useState({ port: 1080, bind_host: '0.0.0.0', username: '', password: '', auto_start: false });

  const [tgStatus, setTgStatus] = useState(null);
  const [tgConfig, setTgConfig] = useState(null);
  const [tgBusy, setTgBusy] = useState(false);
  const [tgConnecting, setTgConnecting] = useState(false);
  const [copied, setCopied] = useState(null);
  const [phoneQrOpen, setPhoneQrOpen] = useState(false);
  const [enablingLan, setEnablingLan] = useState(false);

  // Pending-состояние тумблера: держим спиннер до реальной смены состояния.
  const [pPending, setPPending] = useState(false);
  const [pFlash, setPFlash] = useState('');
  const pendingTimer = useRef(null);
  // Порт: черновик ввода + ошибка валидации (показывается только во время ввода).
  const [portDraft, setPortDraft] = useState(null);
  // TG-порт: та же схема, что у SOCKS-порта — без молчаливой подмены на 1443.
  const [tgPortDraft, setTgPortDraft] = useState(null);

  const pRun = status?.proxy?.running === true;
  const port = config.port || status?.proxy?.port || 1080;
  const bindHost = config.bind_host || '0.0.0.0';
  const proxyAddr = `${bindHost}:${port}`;

  const tgRun = tgStatus?.running === true;
  const tgPort = tgConfig?.port || 1443;

  const proxy = useServiceToggle('proxy', pRun, showToast);
  const fb = useActionFeedback(showToast);
  const saveProxyConfig = useDebouncedSave('/api/proxy/config', 500);
  const saveTgConfig = useDebouncedSave('/api/tgproxy/config', 500);

  const loadData = useCallback(async () => {
    const [devs, cfg, tgSt, tgCfg] = await Promise.all([
      api('GET', '/api/proxy/connections'),
      api('GET', '/api/proxy/config'),
      api('GET', '/api/tgproxy/status'),
      api('GET', '/api/tgproxy/config'),
    ]);
    if (devs) {
      const deviceInfo = devs.device_info || {};
      const socksMap = devs.by_client || {};
      const tgMap = devs.tg_by_client || {};
      // Одна карточка на устройство: SOCKS- и Telegram-подключения вместе.
      const ips = new Set([...Object.keys(socksMap), ...Object.keys(tgMap)]);
      const deviceList = [...ips].map(ip => ({
        ip,
        hostname: deviceInfo[ip]?.hostname || ip,
        mac: deviceInfo[ip]?.mac || '',
        connections: Array.isArray(socksMap[ip]) ? socksMap[ip].length : (socksMap[ip] || 0),
        tg: tgMap[ip] || 0,
      }));
      setDevices(deviceList);
    }
    if (cfg) setConfig(cfg);
    if (tgSt) setTgStatus(tgSt);
    if (tgCfg) setTgConfig(prev => (prev ? { ...prev, ...tgCfg } : tgCfg));
  }, []);

  usePolling(loadData, 5000);

  // Быстрый опрос статуса до смены running на target — мгновенный отклик
  // тумблера в обе стороны (не ждём медленного общего 5s-поллинга).
  const pollUntil = async (getStatus, apply, target, clearPending) => {
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
    pendingTimer.current = setTimeout(clearPending, PENDING_TIMEOUT);
    const started = Date.now();
    const poll = async () => {
      if (Date.now() - started > PENDING_TIMEOUT) {
        clearPending();
        return;
      }
      const st = await getStatus();
      if (st) {
        apply(st);
        if (st.running === target) {
          if (pendingTimer.current) clearTimeout(pendingTimer.current);
          clearPending();
          return;
        }
      }
      setTimeout(poll, PENDING_POLL);
    };
    poll();
  };

  // Единый тумблер: SOCKS5 + Telegram-мост стартуют/останавливаются вместе.
  // Спиннер держится до смены состояния SOCKS, затем догружаем TG-статус.
  const toggleSocks = async () => {
    const target = !pRun;
    let reached = false;
    setPPending(true);
    await proxy.toggle();
    pollUntil(
      () => api('GET', '/api/proxy/status'),
      (st) => { reached = st?.running === target; },
      target,
      () => {
        setPPending(false);
        loadData();
        // Уведомление и флеш — только после подтверждения состояния от бэка.
        if (reached) {
          showToast(target ? t('proxy.started') : t('proxy.stopped'), 'success');
          setPFlash('ok');
        } else {
          setPFlash('err');
        }
        setTimeout(() => setPFlash(''), 600);
      },
    );
  };

  const updateConfig = (patch) => {
    const next = { ...config, ...patch };
    setConfig(next);
    saveProxyConfig(patch, next);
  };

  // Валидация порта: 1–65535. Черновик живёт только пока поле в фокусе ввода;
  // конфиг обновляется только корректным значением.
  const portValue = portDraft ?? String(config.port ?? '');
  const portInvalid = portDraft !== null && !/^\d{1,5}$/.test(portDraft.trim())
    ? true
    : !(Number(portDraft ?? config.port) >= 1 && Number(portDraft ?? config.port) <= 65535);
  const handlePortChange = (e) => {
    const raw = e.target.value;
    setPortDraft(raw);
    const n = parseInt(raw, 10);
    if (/^\d{1,5}$/.test(raw.trim()) && n >= 1 && n <= 65535 && n !== config.port) {
      updateConfig({ port: n });
    }
  };

  // Валидация TG-порта: 1–65535; конфиг обновляется только корректным значением.
  const tgPortValue = tgPortDraft ?? (tgConfig?.port != null ? String(tgConfig.port) : '');
  const tgPortInvalid = tgPortDraft !== null && (
    !/^\d{1,5}$/.test(tgPortDraft.trim())
    || !(Number(tgPortDraft) >= 1 && Number(tgPortDraft) <= 65535)
  );
  const handleTgPortChange = (e) => {
    const raw = e.target.value;
    setTgPortDraft(raw);
    const n = parseInt(raw, 10);
    if (/^\d{1,5}$/.test(raw.trim()) && n >= 1 && n <= 65535 && tgConfig && n !== tgConfig.port) {
      updateTgConfig({ port: n });
    }
  };

  const updateTgConfig = (patch) => {
    setTgConfig(prev => {
      const next = { ...prev, ...patch };
      saveTgConfig(patch, next);
      return next;
    });
  };

  const copyText = async (text, key) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(key);
      setTimeout(() => setCopied(null), 1500);
    } catch {
      showToast(t('toast.requestFailed'), 'error');
    }
  };

  const regenSecret = async () => {
    if (tgBusy) return;
    setTgBusy(true);
    const r = await api('POST', '/api/tgproxy/regenerate-secret');
    setTgBusy(false);
    if (r?.error) {
      showToast(r.error, 'error');
      return;
    }
    showToast(t('tgproxy.secretRegenerated'), 'success');
    if (r?.secret && tgConfig) setTgConfig({ ...tgConfig, secret: r.secret });
    loadData();
  };

  // Кнопка «Подключить в Telegram»: открывает tg://-ссылку (Telegram предложит
  // добавить прокси) с анимацией загрузки. Активна только при работающем мосте.
  const connectInTelegram = async () => {
    if (!tgRun || tgConnecting) return;
    setTgConnecting(true);
    const r = await api('POST', '/api/tgproxy/open-link');
    // Держим анимацию, чтобы пользователь видел отклик действия.
    await new Promise(res => setTimeout(res, 2000));
    setTgConnecting(false);
    if (r?.error) {
      showToast(r.error, 'error');
      return;
    }
    showToast(t('tgproxy.connectStarted'), 'info');
  };

  // Доступ телефона из локальной сети: привязка моста на 0.0.0.0
  // (мост перезапускается на лету, ссылка получит IP этого ПК).
  const enableTgLan = async () => {
    setEnablingLan(true);
    const r = await api('POST', '/api/tgproxy/config', { bind_host: '0.0.0.0' });
    setEnablingLan(false);
    if (r?.error) {
      showToast(r.error, 'error');
      return;
    }
    showToast(t('tgproxy.lanEnabled'), 'success');
    loadData();
  };

  const totalConns = devices.reduce((sum, d) => sum + d.connections, 0);
  const tgStats = tgStatus?.stats || {};
  const tgLink = tgStatus?.link || '';
  const tgLinkEE = tgStatus?.link_ee || '';
  const fakeTLSDomain = (tgConfig?.fake_tls_domain || '').trim();
  const tgLanOpen = (tgConfig?.bind_host || '127.0.0.1') === '0.0.0.0';

  return (
    <div className="proxy-page stagger">
      {/* ── Левая панель: SOCKS5 ── */}
      <div className={'proxy-panel' + (pRun ? ' running' : '')}>
        <div className="proxy-top">
          <div className="proxy-top-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15 15 0 010 20M12 2a15 15 0 000 20"/></svg>
          </div>
          <div className="proxy-top-body">
            <span className="proxy-top-title">{t('proxy.socks5Proxy')}</span>
            <span className="proxy-top-status">
              {pRun ? <>socks5://{proxyAddr}</> : t('proxy.stopped')}
            </span>
          </div>
          {pRun && (
            <div className="proxy-top-stats">
              <div className="pts"><span className="pts-val">{devices.length}</span><span className="pts-label">{t('proxy.devicesSuffix')}</span></div>
              <div className="pts"><span className="pts-val">{totalConns}</span><span className="pts-label">{t('proxy.connSuffix')}</span></div>
            </div>
          )}
          <Switch checked={pRun} onChange={toggleSocks} loading={proxy.loading || pPending} hideState flash={pFlash} />
        </div>

        <div className="proxy-body">
          <div className="pp-block" style={{ flex: 1 }}>
            <span className="pp-subtitle">{t('proxy.connectedDevices')}</span>
            <div className="pp-devices">
              {devices.length === 0 ? (
                <div className="proxy-empty">{t('proxy.noDevices')}</div>
              ) : (
                devices.map(d => (
                  <div key={d.ip} className="proxy-device">
                    <span className="pd-status" />
                    <div className="pd-info">
                      <span className="pd-host">{d.hostname || d.ip}</span>
                      <span className="pd-ip">{d.ip}{d.mac && ' · ' + d.mac}</span>
                    </div>
                    {d.tg > 0 && (
                      <span className="pd-tg-badge" data-tooltip={t('tgproxy.title')}><Send size={9} strokeWidth={2.4} />{d.tg}</span>
                    )}
                    <div className="pd-latency">
                      <span className="pd-latency-val">{d.connections}</span>
                      <span className="pd-latency-label">{t('proxy.connSuffix')}</span>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>

          <div className="pp-block">
            <span className="pp-subtitle">{t('proxy.proxySettings')}</span>
            <div className="pp-grid">
              <div className="pp-field">
                <span className="pp-label">{t('proxy.port')}</span>
                <input type="number" className={'pp-input' + (portInvalid ? ' input-error' : '')} value={portValue} min="1" max="65535"
                  aria-invalid={portInvalid}
                  onChange={handlePortChange}
                  onBlur={() => setPortDraft(null)} />
                {portInvalid && <span className="pp-input-error">{t('proxy.portInvalid')}</span>}
              </div>
              <div className="pp-field">
                <span className="pp-label">{t('proxy.address')}</span>
                <input type="text" className="pp-input" value={config.bind_host || '0.0.0.0'}
                  placeholder="0.0.0.0" onChange={e => updateConfig({ bind_host: e.target.value })} />
              </div>
              <div className="pp-field">
                <span className="pp-label">{t('proxy.username')}</span>
                <input type="text" className="pp-input" value={config.username || ''} placeholder={t('proxy.noAuth')}
                  onChange={e => updateConfig({ username: e.target.value })} />
              </div>
              <div className="pp-field">
                <span className="pp-label">{t('proxy.password')}</span>
                <input type="password" className="pp-input" value={config.password || ''} placeholder={t('proxy.noAuth')}
                  onChange={e => updateConfig({ password: e.target.value })} />
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* ── Правая панель: Telegram MTProto ── */}
      <div className={'proxy-panel tgp' + (tgRun ? ' running' : '')}>
        <div className="proxy-top">
          <div className="proxy-top-icon">
            <Send size={17} strokeWidth={2} />
          </div>
          <div className="proxy-top-body">
            <span className="proxy-top-title">{t('tgproxy.title')}</span>
            <span className="proxy-top-status">
              {tgRun ? <>mtproto://127.0.0.1:{tgPort}</> : t('tgproxy.stopped')}
            </span>
          </div>
          {tgRun && (
            <div className="proxy-top-stats">
              <div className="pts"><span className="pts-val">{tgStats.active ?? 0}</span><span className="pts-label">{t('proxy.connSuffix')}</span></div>
              <div className="pts"><span className="pts-val">{formatBytes(tgStats.bytes_out || 0)}</span><span className="mon-stat-split-tag ul">UL</span></div>
            </div>
          )}
          <span
            className={'tgp-linked-pill' + (tgRun ? ' on' : '')}
            data-tooltip={t('tgproxy.linkedToSocks')}
            data-tooltip-pos="bottom"
          >
            <Lock size={9} strokeWidth={2.4} />
            {tgRun ? t('tgproxy.running') : t('tgproxy.stopped')}
          </span>
        </div>

        <div className="proxy-body">
          {/* Подключение — главный блок, действие первым */}
          <div className="pp-block">
            <span className="pp-subtitle-with-icon"><Link2 size={11} strokeWidth={2.2} /> {t('tgproxy.connectBlock')}</span>
            <div className="tgp-connect-row">
              <button
                className={'btn btn-accent tgp-connect-btn' + (tgConnecting ? ' loading' : '')}
                onClick={connectInTelegram}
                disabled={!tgRun || tgConnecting}
                data-tooltip={!tgRun ? t('tgproxy.connectNeedRunning') : undefined}
                data-tooltip-pos="bottom"
              >
                {tgConnecting ? <span className="mini-spin" /> : <Send size={13} strokeWidth={2.2} />}
                {tgConnecting ? t('tgproxy.connecting') : t('tgproxy.connectInTelegram')}
              </button>
              <button
                className="btn tgp-connect-btn"
                onClick={() => setPhoneQrOpen(true)}
                disabled={!tgRun}
                data-tooltip={!tgRun ? t('tgproxy.connectNeedRunning') : undefined}
                data-tooltip-pos="bottom"
              >
                <Smartphone size={13} strokeWidth={2.2} />
                {t('tgproxy.connectPhone')}
              </button>
            </div>
            <div className="tgp-link-box">
              <code className="tgp-link" data-tooltip={tgLink} data-tooltip-pos="bottom">{tgLink || '—'}</code>
              <div className="tgp-link-actions">
                <button className="btn btn-icon" onClick={() => copyText(tgLink, 'link')} disabled={!tgLink}
                  data-tooltip={t('tgproxy.copyLink')} aria-label={t('tgproxy.copyLink')} data-tooltip-pos="bottom">
                  {copied === 'link' ? <Check size={12} strokeWidth={2.5} /> : <Copy size={12} strokeWidth={2} />}
                </button>
              </div>
              {fakeTLSDomain && tgLinkEE && <span className="tgp-link-tag">{t('tgproxy.eeTls')}</span>}
            </div>
            {fakeTLSDomain && tgLinkEE && (
              <div className="tgp-link-box">
                <code className="tgp-link" data-tooltip={tgLinkEE} data-tooltip-pos="bottom">{tgLinkEE}</code>
                <div className="tgp-link-actions">
                  <button className="btn btn-icon" onClick={() => copyText(tgLinkEE, 'link_ee')} disabled={!tgLinkEE}
                    data-tooltip={t('tgproxy.copyLink')} aria-label={t('tgproxy.copyLink')} data-tooltip-pos="bottom">
                    {copied === 'link_ee' ? <Check size={12} strokeWidth={2.5} /> : <Copy size={12} strokeWidth={2} />}
                  </button>
                </div>
              </div>
            )}
            <div className="tgp-secret-row">
              <span className="tgp-secret-label">secret</span>
              <code className="tgp-secret">{tgConfig?.secret || '—'}</code>
              <button className="btn btn-icon" onClick={() => copyText(tgConfig?.secret || '', 'secret')}
                disabled={!tgConfig?.secret} data-tooltip={t('common.copy')} aria-label={t('common.copy')} data-tooltip-pos="bottom">
                {copied === 'secret' ? <Check size={12} strokeWidth={2.5} /> : <Copy size={12} strokeWidth={2} />}
              </button>
              <button className="btn btn-icon" onClick={regenSecret} disabled={tgBusy}
                data-tooltip={t('tgproxy.regenerateSecret')} aria-label={t('tgproxy.regenerateSecret')} data-tooltip-pos="bottom">
                {tgBusy ? <span className="mini-spin" /> : <RefreshCw size={12} strokeWidth={2} />}
              </button>
            </div>
          </div>

          {/* Настройки — сетка: значения вместе, тумблеры вместе */}
          <div className="pp-block">
            <span className="pp-subtitle-with-icon"><Settings2 size={11} strokeWidth={2.2} /> {t('tgproxy.settings')}</span>
            <div className="pp-grid">
              <div className="pp-field">
                <span className="pp-label">{t('tgproxy.port')}</span>
                <input type="number" className={'pp-input' + (tgPortInvalid ? ' input-error' : '')} value={tgPortValue} min="1" max="65535"
                  aria-invalid={tgPortInvalid}
                  onChange={handleTgPortChange}
                  onBlur={() => setTgPortDraft(null)} />
                {tgPortInvalid && <span className="pp-input-error">{t('proxy.portInvalid')}</span>}
              </div>
              <div className="pp-field">
                <span className="pp-label">{t('tgproxy.fakeTls')}</span>
                <input type="text" className="pp-input" value={fakeTLSDomain}
                  placeholder="—" onChange={e => updateTgConfig({ fake_tls_domain: e.target.value })} />
              </div>
            </div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.fallbackCFDesc')} data-tooltip-pos="bottom">{t('tgproxy.fallbackCF')}</span>
              <Switch checked={tgConfig?.fallback_cf !== false} onChange={() => updateTgConfig({ fallback_cf: !(tgConfig?.fallback_cf !== false) })} />
            </div>
            <div className="pp-field">
              <span className="pp-label">{t('tgproxy.cfWorkers')}</span>
              <input type="text" className="pp-input" value={(tgConfig?.cf_worker_domains || []).join(', ')}
                placeholder="worker.example.workers.dev, …" onChange={e => updateTgConfig({ cf_worker_domains: e.target.value })} />
            </div>
          </div>

          {/* Статистика — только при работе */}
          {tgRun ? (
            <div className="tgp-stats">
              <span className="tgp-stats-line">{t('tgproxy.stats', { active: tgStats.active ?? 0, total: tgStats.total ?? 0 })}</span>
              <span className="tgp-stats-line">{t('tgproxy.traffic', { in: formatBytes(tgStats.bytes_in || 0), out: formatBytes(tgStats.bytes_out || 0) })}</span>
              <span className="tgp-stats-line">{t('tgproxy.wsStats', { ok: tgStats.ws_ok ?? 0, fail: tgStats.ws_fail ?? 0 })}</span>
              <span className="tgp-stats-line">{t('tgproxy.fbStats', { cf: tgStats.cfproxy ?? 0, tcp: tgStats.tcp_fallback ?? 0, pool: tgStats.pool_hits ?? 0 })}</span>
            </div>
          ) : (
            <span className="tgp-hint">{t('tgproxy.linkHint', { port: tgPort })}</span>
          )}
        </div>
      </div>

      <PhoneQrModal
        open={phoneQrOpen}
        onClose={() => setPhoneQrOpen(false)}
        webUrl={tgStatus?.web_url || ''}
        lanOpen={tgLanOpen}
        enablingLan={enablingLan}
        onEnableLan={enableTgLan}
      />
    </div>
  );
}
