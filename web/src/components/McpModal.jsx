import { useState, useEffect, useCallback } from 'react';
import Modal from './ui/Modal';
import { api, apiCacheGet, apiCachePut } from '../api';
import { useT } from '../i18n';
import { useToast } from '../context/ToastContext';
import { useConfirm } from './ui/ConfirmDialog';
import { usePolling } from '../hooks/usePolling';
import { Bot, Check, CircleHelp, Copy, FolderOpen, KeyRound, Waypoints } from 'lucide-react';
import Switch from './ui/Switch';

const MCP_INFO_ROUTE = '/api/mcp/info';

// McpModal — MCP-сервер в четырёх секциях: «Сервер» (тумблер, URL, порт),
// «Агенты» (подключённые сессии), «Доступ» (ключ с копированием и
// подтверждением регенерации), «Конфиг для агента» (JSON одной кнопкой).
// Спека: Документы/Задачи/2026-11-12-mcp-server.md.
export default function McpModal({ open, onClose }) {
  const showToast = useToast();
  const { t } = useT();
  const confirm = useConfirm();
  const [info, setInfo] = useState(null);
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(null);
  const [openingSkill, setOpeningSkill] = useState(false);

  const load = useCallback(async () => {
    const r = await api('GET', MCP_INFO_ROUTE);
    if (r && !r.error) {
      apiCachePut(MCP_INFO_ROUTE, r);
      setInfo(r);
    }
  }, []);

  useEffect(() => {
    if (open) {
      const cached = apiCacheGet(MCP_INFO_ROUTE);
      if (cached) setInfo(cached);
      load();
    }
  }, [open, load]);
  usePolling(load, 10000, [load], open);

  const setEnabled = async (on) => {
    if (busy) return;
    setBusy(true);
    const r = await api('POST', '/api/mcp/enabled', { on });
    setBusy(false);
    if (r && r.error) showToast(r.error, 'error');
    else if (r && r.warning) showToast(r.warning, 'warning');
    load();
  };

  const regenKey = async () => {
    if (busy) return;
    const ok = await confirm({
      message: t('mcp.regenConfirm'),
      variant: 'danger',
      confirmText: t('mcp.regen'),
    });
    if (!ok) return;
    setBusy(true);
    const r = await api('POST', '/api/mcp/regen-key');
    setBusy(false);
    if (r && !r.error) showToast(t('mcp.regenDone'), 'success');
    load();
  };

  const copyText = async (text, field) => {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(field);
      setTimeout(() => setCopied((cur) => (cur === field ? null : cur)), 1500);
    } catch {
      showToast(t('toast.requestFailed'), 'error');
    }
  };

  const openSkillFolder = async () => {
    if (openingSkill) return;
    setOpeningSkill(true);
    const r = await api('POST', '/api/mcp/open-skill-folder');
    setOpeningSkill(false);
    if (r && r.error) showToast(t('mcp.skillOpenFail'), 'error');
  };

  const copyBtn = (field, value) => (
    <button className="btn btn-sm" onClick={() => copyText(value, field)} disabled={!value}>
      {copied === field ? <Check size={13} strokeWidth={2.2} /> : <Copy size={13} strokeWidth={2.2} />}
      {copied === field ? t('common.copied') : t('common.copy')}
    </button>
  );

  const running = !!info?.running;
  const stateKey = !info
    ? null
    : running
      ? 'mcp.stateOn'
      : info.last_error
        ? 'mcp.stateError'
        : 'mcp.stateOff';
  const sessions = info?.sessions || [];

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('mcp.title')}
      icon={<Waypoints size={18} strokeWidth={2} />}
      tone="neutral"
      size="xl"
      footer={<button className="btn btn-sm" onClick={onClose}>{t('common.close')}</button>}
    >
      <div className="mcp-body">
        <div className="mcp-section">
          <div className="section-title set-section-title">
            <span>{t('mcp.secServer')}</span>
            {stateKey && (
              <span className={'syncd-pill' + (running ? ' on' : '')}>
                <span className="syncd-pill-dot" />
                {t(stateKey)}
              </span>
            )}
          </div>

          <div className="set-row">
            <div className="set-row-info">
              <span className="set-row-title">{t('mcp.toggle')}</span>
              <span className="set-row-desc">{t('mcp.hint')}</span>
            </div>
            <span className="mcp-actions">
              <span
                className="mcp-help"
                data-tooltip={t('mcp.commands')}
                data-tooltip-pos="left"
                aria-label={t('mcp.commands')}
              >
                <CircleHelp size={15} strokeWidth={2} />
              </span>
              <Switch checked={!!info?.enabled} onChange={setEnabled} loading={busy} />
            </span>
          </div>

          {info && !info.enabled && <div className="syncd-hint">{t('mcp.hintOff')}</div>}

          {info && (
            <>
              <div className="set-row">
                <div className="set-row-info">
                  <span className="set-row-title">{t('mcp.urlLabel')}</span>
                  <span className="set-row-desc mono">{info.url}</span>
                </div>
                {copyBtn('url', info.url)}
              </div>
              <div className="set-row">
                <div className="set-row-info">
                  <span className="set-row-title">{t('mcp.port')}</span>
                  <span className="set-row-desc mono">{info.port}</span>
                </div>
              </div>
              {info.last_error && <div className="mcp-error">{info.last_error}</div>}
              <div className="set-row">
                <div className="set-row-info">
                  <span className="set-row-title">{t('mcp.skillTitle')}</span>
                  <span className="set-row-desc">{t('mcp.skillDesc')}</span>
                  {info.skill_path && (
                    <span className="set-row-desc mono mcp-skill-path">{info.skill_path}</span>
                  )}
                </div>
                <span className="mcp-actions">
                  <button className="btn btn-sm" onClick={openSkillFolder} disabled={!info.skill_path}>
                    <FolderOpen size={13} strokeWidth={2.2} />
                    {t('mcp.skillOpenFolder')}
                  </button>
                </span>
              </div>
            </>
          )}
        </div>

        <div className="mcp-section">
          <div className="section-title set-section-title">
            <span>{t('mcp.secAgents')}</span>
          </div>
          {sessions.length > 0 ? (
            <div className="mcp-agents">
              {sessions.map((s, i) => (
                <div key={i} className="mcp-agent">
                  <span className="mcp-agent-icon"><Bot size={14} strokeWidth={2} /></span>
                  <span className="mcp-agent-name">{s.name}</span>
                  <span className="mcp-agent-addr mono">{s.addr}</span>
                  <span className="mcp-agent-last">{t('mcp.sessionLast', { time: s.last })}</span>
                </div>
              ))}
            </div>
          ) : (
            info && (
              <div className="mcp-agents-empty">
                {info.running ? t('mcp.waiting') : t('mcp.noSessions')}
              </div>
            )
          )}
        </div>

        {info && (
          <div className="mcp-section">
            <div className="section-title set-section-title">
              <span>{t('mcp.secAccess')}</span>
            </div>
            <div className="set-row">
              <div className="set-row-info">
                <span className="set-row-title">{t('mcp.keyTitle')}</span>
                <span className="set-row-desc mono">{info.key}</span>
              </div>
              <span className="mcp-actions">
                {copyBtn('key', info.key)}
                <button className="btn btn-sm" onClick={regenKey} disabled={busy}>
                  <KeyRound size={13} strokeWidth={2.2} />
                  {t('mcp.regen')}
                </button>
              </span>
            </div>
          </div>
        )}

        {info && (
          <div className="mcp-section">
            <div className="section-title set-section-title">
              <span>{t('mcp.jsonTitle')}</span>
              {copyBtn('json', info.config_json)}
            </div>
            <pre className="mcp-json mono">{info.config_json}</pre>
          </div>
        )}
      </div>
    </Modal>
  );
}
