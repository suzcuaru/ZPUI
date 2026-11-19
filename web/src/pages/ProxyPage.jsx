import { useState, useCallback, useEffect, useRef } from 'react';
import Switch from '../components/ui/Switch';
import QRCode from 'qrcode';
import { api, apiCacheGet, apiCachePut } from '../api';
import { useT } from '../i18n';
import { usePolling } from '../hooks/usePolling';
import { useDebouncedSave } from '../hooks/useDebouncedSave';
import { errorText } from '../errors';
import { formatBytes } from '../utils';
import { useToast } from '../context/ToastContext';
import { Send, Copy, Check, RefreshCw, Settings2, QrCode, Wifi, WifiOff, Cable, Link2, ChevronDown } from 'lucide-react';
import Spinner from '../components/ui/Spinner';

const PENDING_TIMEOUT = 20000;
const PENDING_POLL = 700;

// Прокси = только Telegram MTProto через локальный мост: мост поднимается
// на этом компьютере, телефон подключается из ЛВС или по ссылке/QR.
export default function ProxyPage({ status }) {
  const showToast = useToast();
  const { t } = useT();

  const [tgStatus, setTgStatus] = useState(null);
  const [tgConfig, setTgConfig] = useState(null);
  const [tgBusy, setTgBusy] = useState(false);
  const [tgConnecting, setTgConnecting] = useState(false);
  const [copied, setCopied] = useState(null);
  const [enablingLan, setEnablingLan] = useState(false);
  const qrCanvasRef = useRef(null);

  // Pending-состояние тумблера: держим спиннер до реальной смены состояния.
  const [tgPending, setTgPending] = useState(false);
  const [tgFlash, setTgFlash] = useState('');
  const pendingTimer = useRef(null);
  // TG-порт: черновик ввода + валидация (без молчаливой подмены на 1443).
  const [tgPortDraft, setTgPortDraft] = useState(null);
  // DC IP: черновик списка "dc:ip" + валидация (сохраняем только корректные).
  const [dcIpDraft, setDcIpDraft] = useState(null);
  // SOCKS5 для домашней сети: статус + черновик порта.
  const [socks, setSocks] = useState(null);
  const [socksPortDraft, setSocksPortDraft] = useState(null);
  // SOCKS-тумблер: pending до смены enabled в статусе.
  const [socksPending, setSocksPending] = useState(false);
  const [socksFlash, setSocksFlash] = useState('');
  // «Дополнительные настройки» моста — сворачиваемый блок под TG-секцией.
  const [advOpen, setAdvOpen] = useState(false);
  // Размонтирование страницы (уход со вкладки): pollUntil прекращает опрос,
  // таймеры гасятся — никаких api-вызовов и setState после unmount.
  const aliveRef = useRef(true);
  const pollTimer = useRef(null);
  useEffect(() => () => {
    aliveRef.current = false;
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
    if (pollTimer.current) clearTimeout(pollTimer.current);
  }, []);

  const tgRun = tgStatus?.running === true;
  const tgPort = tgConfig?.port || 1443;

  const saveTgConfig = useDebouncedSave('/api/tgproxy/config', 500);

  const loadData = useCallback(async () => {
    const cSt = apiCacheGet('/api/tgproxy/status');
    const cCfg = apiCacheGet('/api/tgproxy/config');
    if (cSt) setTgStatus(cSt);
    if (cCfg) setTgConfig(prev => (prev ? { ...prev, ...cCfg } : cCfg));
    const [tgSt, tgCfg] = await Promise.all([
      api('GET', '/api/tgproxy/status'),
      api('GET', '/api/tgproxy/config'),
    ]);
    if (tgSt) {
      apiCachePut('/api/tgproxy/status', tgSt);
      setTgStatus(tgSt);
    }
    if (tgCfg) {
      apiCachePut('/api/tgproxy/config', tgCfg);
      setTgConfig(prev => (prev ? { ...prev, ...tgCfg } : tgCfg));
    }
  }, []);

  const loadSocks = useCallback(async () => {
    const cached = apiCacheGet('/api/socks/status');
    if (cached) setSocks(cached);
    const r = await api('GET', '/api/socks/status');
    if (r) {
      apiCachePut('/api/socks/status', r);
      setSocks(r);
    }
  }, []);

  // Один 5с-тик на обе загрузки (мост + SOCKS): два параллельных поллера
  // держали два независимых таймера на один интервал.
  usePolling(async () => {
    await loadData().catch(() => {});
    await loadSocks().catch(() => {});
  }, 5000);

  // Быстрый опрос статуса до смены состояния — мгновенный отклик тумблера
  // в обе стороны (не ждём медленного общего 5s-поллинга).
  const pollUntil = async (getStatus, isDone, onDone) => {
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
    if (pollTimer.current) clearTimeout(pollTimer.current);
    pendingTimer.current = setTimeout(() => onDone(false), PENDING_TIMEOUT);
    const started = Date.now();
    const poll = async () => {
      if (!aliveRef.current) return;
      if (Date.now() - started > PENDING_TIMEOUT) {
        onDone(false);
        return;
      }
      const st = await getStatus();
      if (!aliveRef.current) return;
      if (st && isDone(st)) {
        if (pendingTimer.current) clearTimeout(pendingTimer.current);
        onDone(true);
        return;
      }
      pollTimer.current = setTimeout(poll, PENDING_POLL);
    };
    poll();
  };

  const toggleTg = async () => {
    const target = !tgRun;
    setTgPending(true);
    const r = await api('POST', target ? '/api/tgproxy/start' : '/api/tgproxy/stop');
    if (r?.error) {
      setTgPending(false);
      showToast(r.error, 'error');
      return;
    }
    pollUntil(
      () => api('GET', '/api/tgproxy/status'),
      (st) => st.running === target,
      (reached) => {
        setTgPending(false);
        loadData();
        showToast(reached ? (target ? t('proxy.started') : t('proxy.stopped')) : t('common.error'), reached ? 'success' : 'error');
        setTgFlash(reached ? 'ok' : 'err');
        setTimeout(() => setTgFlash(''), 600);
      },
    );
  };

  // SOCKS5 управляемый: статика — рекомендация, а не условие. Включение
  // без статики проходит, бэкенд вернёт warning=static_required.
  const toggleSocks = async () => {
    const target = !(socks?.enabled === true);
    setSocksPending(true);
    const r = await api('POST', '/api/socks/enabled', { on: target });
    if (r?.error) {
      setSocksPending(false);
      showToast(errorText(t, r.error), 'error');
      loadSocks();
      return;
    }
    pollUntil(
      () => api('GET', '/api/socks/status'),
      (st) => st.enabled === target,
      (reached) => {
        setSocksPending(false);
        loadSocks();
        if (reached && r?.warning === 'static_required') {
          showToast(t('socks.staticRequired'), 'warning');
          return;
        }
        showToast(
          reached ? t(target ? 'socks.enabledToast' : 'socks.disabledToast') : t('common.error'),
          reached ? 'success' : 'error',
        );
        setSocksFlash(reached ? 'ok' : 'err');
        setTimeout(() => setSocksFlash(''), 600);
      },
    );
  };

  // Валидация SOCKS-порта: 1–65535; конфиг обновляется только корректным значением.
  const socksPortValue = socksPortDraft ?? (socks?.port != null ? String(socks.port) : '');
  const socksPortInvalid = socksPortDraft !== null && (
    !/^\d{1,5}$/.test(socksPortDraft.trim())
    || !(Number(socksPortDraft) >= 1 && Number(socksPortDraft) <= 65535)
  );
  const handleSocksPortChange = (e) => {
    const raw = e.target.value;
    setSocksPortDraft(raw);
    const n = parseInt(raw, 10);
    if (/^\d{1,5}$/.test(raw.trim()) && n >= 1 && n <= 65535 && socks && n !== socks.port) {
      api('POST', '/api/socks/port', { port: n }).then(loadSocks);
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

  // Список DC IP "dc:ip" через запятую; пусто — дефолт апстрима (DC2/DC4).
  const DC_IP_RE = /^\d{1,5}:(\d{1,3}\.){3}\d{1,3}$/;
  const dcIpValue = dcIpDraft ?? (tgConfig?.dc_ip || []).join(', ');
  const dcIpInvalid = dcIpDraft !== null && dcIpDraft.trim() !== ''
    && dcIpDraft.split(',').map(s => s.trim()).filter(Boolean).some(s => !DC_IP_RE.test(s));
  const handleDcIpChange = (e) => {
    const raw = e.target.value;
    setDcIpDraft(raw);
    const tokens = raw.split(',').map(s => s.trim()).filter(Boolean);
    if (tokens.length === 0) {
      if ((tgConfig?.dc_ip || []).length > 0) updateTgConfig({ dc_ip: [] });
      return;
    }
    if (tokens.every(s => DC_IP_RE.test(s))) updateTgConfig({ dc_ip: tokens });
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

  // Кнопка «Подключить в Telegram»: открывает tg://-ссылку с анимацией загрузки.
  const connectInTelegram = async (link) => {
    if (!link || tgConnecting) return;
    setTgConnecting(true);
    const r = await api('POST', '/api/telegram/open', { url: link });
    await new Promise(res => setTimeout(res, 1500));
    setTgConnecting(false);
    if (r?.error) {
      showToast(r.error, 'error');
      return;
    }
    showToast(t('tgproxy.connectStarted'), 'info');
  };

  // Доступ телефона из локальной сети: мост на 0.0.0.0.
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

  const tgStats = tgStatus?.stats || {};
  const link = tgStatus?.link || '';
  const linkEE = tgStatus?.link_ee || '';
  const fakeTLSDomain = (tgConfig?.fake_tls_domain || '').trim();
  const tgLanOpen = (tgConfig?.bind_host || '127.0.0.1') === '0.0.0.0';
  // Адрес моста для строки статуса: при открытом для ЛВС бинде показываем
  // LAN-IP компьютера — телефон подключается по нему, а не по 127.0.0.1.
  const tgHost = tgLanOpen && (socks?.lan_ip || tgStatus?.lan_ip) ? (socks?.lan_ip || tgStatus?.lan_ip) : '127.0.0.1';
  // SOCKS5 — самостоятельный сервис (к Telegram отношения не имеет),
  // теперь с мастер-тумблером: enabled — конфиг, running — факт.
  const socksOn = socks?.enabled === true;
  const socksRunning = socks?.running === true;
  const socksAddr = socks?.lan_ip && socks?.port ? `${socks.lan_ip}:${socks.port}` : '';
  const socksDesc = !socksOn
    ? t('socks.stateOff')
    : (!socksRunning
      ? (socks?.power_paused ? t('socks.statePower') : t('socks.stateStarting'))
      : t('socks.stateOn'));
  // QR кодирует сразу tg://-ссылку: сканирование добавляет прокси без настроек.
  const qrReady = !!link;
  useEffect(() => {
    if (!qrReady || !qrCanvasRef.current) return;
    QRCode.toCanvas(qrCanvasRef.current, link, {
      width: 200,
      margin: 1,
      color: { dark: '#000000', light: '#ffffff' },
    }).catch(() => {});
  }, [qrReady, link]);

  return (
    <div className="proxy-page stagger">
      {/* Герой: Telegram MTProto — свой сервис, отдельный от SOCKS5 ниже */}
      <div className={'dns-hero' + (tgRun ? ' on' : '')}>
        <div className="dns-hero-left">
          <div className={'dns-hero-icon' + (tgRun ? ' on' : '')}>
            <Send size={24} strokeWidth={2} />
          </div>
          <div className="dns-hero-info">
            <span className="dns-hero-title">{t('tgproxy.title')}</span>
            <span className={'dns-hero-desc' + (tgRun ? ' on' : '')}>
              {!tgRun && tgStatus && tgStatus.installed === false
                ? t('tgproxy.notInstalled')
                : (tgRun ? <>mtproto://{tgHost}:{tgPort}</> : t('tgproxy.stopped'))}
              {tgStatus?.version ? ` · v${tgStatus.version}` : ''}
            </span>
          </div>
        </div>
        <div className="dns-hero-state">
          {tgRun && (
            <div className="proxy-top-stats">
              <div className="pts"><span className="pts-val">{tgStats.active ?? 0}</span><span className="pts-label">{t('proxy.sessionsSuffix')}</span></div>
              <div className="pts"><span className="pts-val">{formatBytes(tgStats.bytes_out || 0)}</span><span className="mon-stat-split-tag ul">UL</span></div>
            </div>
          )}
          <Switch size="lg" checked={tgRun} onChange={toggleTg} loading={tgPending} hideState flash={tgFlash} />
        </div>
      </div>

      <div className="svc-grid">
        <div className="section">
          <div className="dns-section-head">
            <Link2 size={13} strokeWidth={2} />
            <span className="section-title">{t('tgproxy.connectBlock')}</span>
          </div>
          <p className="dns-section-sub">{t('tgproxy.connectLinkDesc')}</p>
          <button
            className={'btn btn-accent tgp-connect-btn' + (tgConnecting ? ' loading' : '')}
            onClick={() => connectInTelegram(tgStatus?.link)}
            disabled={!tgRun || tgConnecting}
            data-tooltip={!tgRun ? t('tgproxy.connectNeedRunning') : undefined}
            data-tooltip-pos="bottom"
          >
            {tgConnecting ? <Spinner size={14} /> : <Send size={13} strokeWidth={2.2} />}
            {tgConnecting ? t('tgproxy.connecting') : t('tgproxy.connectInTelegram')}
          </button>
          <div className="tgp-link-box">
            <code className="tgp-link" data-tooltip={tgStatus?.link} data-tooltip-pos="bottom">{tgStatus?.link || '—'}</code>
            <div className="tgp-link-actions">
              <button className="btn btn-icon" onClick={() => copyText(tgStatus?.link || '', 'link')} disabled={!tgStatus?.link}
                data-tooltip={t('tgproxy.copyLink')} aria-label={t('tgproxy.copyLink')} data-tooltip-pos="bottom">
                {copied === 'link' ? <Check size={12} strokeWidth={2.5} /> : <Copy size={12} strokeWidth={2} />}
              </button>
            </div>
            {fakeTLSDomain && linkEE && <span className="tgp-link-tag">{t('tgproxy.eeTls')}</span>}
          </div>
          {fakeTLSDomain && linkEE && (
            <div className="tgp-link-box">
              <code className="tgp-link" data-tooltip={linkEE} data-tooltip-pos="bottom">{linkEE}</code>
              <div className="tgp-link-actions">
                <button className="btn btn-icon" onClick={() => copyText(linkEE, 'link_ee')} disabled={!linkEE}
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
              {tgBusy ? <Spinner size={14} /> : <RefreshCw size={12} strokeWidth={2} />}
            </button>
          </div>
          <div className={'tgp-lan ' + (tgLanOpen ? 'ok' : 'off')}>
            {tgLanOpen ? <Wifi size={12} strokeWidth={2.2} /> : <WifiOff size={12} strokeWidth={2.2} />}
            <span>{tgLanOpen ? t('tgproxy.lanOn') : t('tgproxy.lanOff')}</span>
            {!tgLanOpen && (
              <button className="btn btn-accent btn-sm" onClick={enableTgLan} disabled={enablingLan}>
                {enablingLan && <Spinner size={14} />}
                {t('tgproxy.lanEnable')}
              </button>
            )}
          </div>
        </div>
        <div className="tgp-qr-wrap">
          {qrReady ? (
            <canvas ref={qrCanvasRef} />
          ) : (
            <div className="tgp-qr-empty" data-tooltip={t('tgproxy.qrNeedRun')} data-tooltip-pos="bottom">
              <QrCode size={22} strokeWidth={1.8} />
            </div>
          )}
        </div>
      </div>

      {/* Дополнительные настройки моста — сворачиваемый блок в секции Telegram */}
      <div className="section tg-adv">
        <button type="button" className="tg-adv-head" onClick={() => setAdvOpen(o => !o)} aria-expanded={advOpen}>
          <Settings2 size={13} strokeWidth={2} />
          <span className="section-title">{t('proxy.advTitle')}</span>
          <span className={'tg-adv-chev' + (advOpen ? ' open' : '')}>
            <ChevronDown size={13} strokeWidth={2.2} />
          </span>
        </button>
        {advOpen && (
          <div className="tg-adv-body">
            <div className="pp-field">
              <span className="pp-label">{t('tgproxy.port')}</span>
              <input type="number" className={'pp-input' + (tgPortInvalid ? ' input-error' : '')} value={tgPortValue} min="1" max="65535"
                aria-invalid={tgPortInvalid}
                onChange={handleTgPortChange}
                onBlur={() => setTgPortDraft(null)} />
              {tgPortInvalid && <span className="pp-input-error">{t('proxy.portInvalid')}</span>}
            </div>
            <div className="pp-field">
              <span className="pp-label" data-tooltip={t('tgproxy.fakeTlsDesc')} data-tooltip-pos="bottom">{t('tgproxy.fakeTls')}</span>
              <input type="text" className="pp-input" value={fakeTLSDomain}
                placeholder="—" onChange={e => updateTgConfig({ fake_tls_domain: e.target.value })} />
            </div>

            <div className="pp-group-title">{t('tgproxy.advGroupFallback')}</div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.fallbackCFDesc')} data-tooltip-pos="bottom">{t('tgproxy.fallbackCF')}</span>
              <Switch checked={tgConfig?.fallback_cf !== false} onChange={() => updateTgConfig({ fallback_cf: !(tgConfig?.fallback_cf !== false) })} />
            </div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.h2MediaDesc')} data-tooltip-pos="bottom">{t('tgproxy.h2Media')}</span>
              <Switch checked={tgConfig?.no_h2 !== true} onChange={() => updateTgConfig({ no_h2: !(tgConfig?.no_h2 === true) })} />
            </div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.noSecureDesc')} data-tooltip-pos="bottom">{t('tgproxy.noSecure')}</span>
              <Switch checked={tgConfig?.no_secure === true} onChange={() => updateTgConfig({ no_secure: !(tgConfig?.no_secure === true) })} />
            </div>
            <div className="pp-field">
              <span className="pp-label" data-tooltip={t('tgproxy.cfUsersDesc')} data-tooltip-pos="bottom">{t('tgproxy.cfUsers')}</span>
              <input type="text" className="pp-input" value={(tgConfig?.cf_user_domains || []).join(', ')}
                placeholder="—" onChange={e => updateTgConfig({ cf_user_domains: e.target.value.split(',').map(s => s.trim()).filter(Boolean) })} />
            </div>
            <div className="pp-field">
              <span className="pp-label" data-tooltip={t('tgproxy.cfWorkersDesc')} data-tooltip-pos="bottom">{t('tgproxy.cfWorkers')}</span>
              <input type="text" className="pp-input" value={(tgConfig?.cf_worker_domains || []).join(', ')}
                placeholder="worker.example.workers.dev, …" onChange={e => updateTgConfig({ cf_worker_domains: e.target.value.split(',').map(s => s.trim()).filter(Boolean) })} />
            </div>

            <div className="pp-group-title">{t('tgproxy.advGroupConn')}</div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.poolSizeDesc')} data-tooltip-pos="bottom">{t('tgproxy.poolSize')}</span>
              <input type="number" className="pp-input pp-input-num" min="1" max="64" value={tgConfig?.pool_size ?? 4}
                onChange={e => { const n = parseInt(e.target.value, 10); if (n >= 1) updateTgConfig({ pool_size: n }); }}
                onBlur={e => { const n = parseInt(e.target.value, 10); if (!(n >= 1)) updateTgConfig({ pool_size: 4 }); }} />
            </div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.bufKbDesc')} data-tooltip-pos="bottom">{t('tgproxy.bufKb')}</span>
              <input type="number" className="pp-input pp-input-num" min="4" max="4096" value={tgConfig?.buf_kb ?? 256}
                onChange={e => { const n = parseInt(e.target.value, 10); if (n >= 4) updateTgConfig({ buf_kb: n }); }}
                onBlur={e => { const n = parseInt(e.target.value, 10); if (!(n >= 4)) updateTgConfig({ buf_kb: 256 }); }} />
            </div>

            <div className="pp-group-title">{t('tgproxy.advGroupDiag')}</div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.dcIpDesc')} data-tooltip-pos="bottom">{t('tgproxy.dcIp')}</span>
            </div>
            <div className="pp-field">
              <input type="text" className={'pp-input' + (dcIpInvalid ? ' input-error' : '')} value={dcIpValue}
                placeholder="2:149.154.167.220, 4:149.154.167.220" aria-invalid={dcIpInvalid}
                onChange={handleDcIpChange} onBlur={() => setDcIpDraft(null)} />
              {dcIpInvalid && <span className="pp-input-error">{t('tgproxy.dcIpInvalid')}</span>}
            </div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.proxyProtocolDesc')} data-tooltip-pos="bottom">{t('tgproxy.proxyProtocol')}</span>
              <Switch checked={tgConfig?.proxy_protocol === true} onChange={() => updateTgConfig({ proxy_protocol: !(tgConfig?.proxy_protocol === true) })} />
            </div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.forceTestDcDesc')} data-tooltip-pos="bottom">{t('tgproxy.forceTestDc')}</span>
              <Switch checked={tgConfig?.force_test_dc === true} onChange={() => updateTgConfig({ force_test_dc: !(tgConfig?.force_test_dc === true) })} />
            </div>
            <div className="pp-row">
              <span className="pp-row-title" data-tooltip={t('tgproxy.verboseDesc')} data-tooltip-pos="bottom">{t('tgproxy.verbose')}</span>
              <Switch checked={tgConfig?.verbose === true} onChange={() => updateTgConfig({ verbose: !(tgConfig?.verbose === true) })} />
            </div>
          </div>
        )}
      </div>

      {/* SOCKS5 — самостоятельный прокси домашней сети (к Telegram отношения
          не имеет): герой в общем стиле вкладки, ниже — адрес и порт;
          статический IP настраивается на вкладке DNS. */}
      <div className={'dns-hero' + (socksOn ? ' on' : '')}>
        <div className="dns-hero-left">
          <div className={'dns-hero-icon' + (socksOn ? ' on' : '')}>
            <Cable size={24} strokeWidth={2} />
          </div>
          <div className="dns-hero-info">
            <span className="dns-hero-title">{t('socks.title')}</span>
            <span className={'dns-hero-desc' + (socksOn ? ' on' : '')}>
              {socksOn && socksRunning && socksAddr ? <>socks5://{socksAddr}</> : socksDesc}
            </span>
          </div>
        </div>
        <div className="dns-hero-state">
          <Switch size="lg" checked={socksOn} onChange={toggleSocks} loading={socksPending} hideState flash={socksFlash} />
        </div>
      </div>

      <div className="section socks">
        <div className="tgp-link-box">
          <code className="tgp-link" data-tooltip={socksAddr} data-tooltip-pos="bottom">{socksAddr || '—'}</code>
          <div className="tgp-link-actions">
            <button className="btn btn-icon" onClick={() => copyText(socksAddr, 'socks')} disabled={!socksAddr}
              data-tooltip={t('common.copy')} aria-label={t('common.copy')} data-tooltip-pos="bottom">
              {copied === 'socks' ? <Check size={12} strokeWidth={2.5} /> : <Copy size={12} strokeWidth={2} />}
            </button>
          </div>
          <span className="tgp-link-tag">{t('socks.phoneTag')}</span>
        </div>
        <div className="pp-row">
          <span className="pp-row-title">{t('socks.port')}</span>
          <input type="number" className={'pp-input' + (socksPortInvalid ? ' input-error' : '')} value={socksPortValue} min="1" max="65535"
            aria-invalid={socksPortInvalid} onChange={handleSocksPortChange} onBlur={() => setSocksPortDraft(null)} />
        </div>
        <p className="tun-phone-hint">{t('socks.staticRef')}</p>
        <p className="tun-phone-hint">{t('socks.hint')}</p>
      </div>

      {tgRun && (
        <div className="proxy-foot">
          <span className="tgp-hint">{t('tgproxy.stats', { active: tgStats.active ?? 0, total: tgStats.total ?? 0 })} · {t('tgproxy.traffic', { in: formatBytes(tgStats.bytes_in || 0), out: formatBytes(tgStats.bytes_out || 0) })}</span>
        </div>
      )}
    </div>
  );
}
