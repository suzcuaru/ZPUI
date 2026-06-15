import { useState } from 'react';
import { api, openExternal } from '../api';

export default function AboutPage({ status, showToast }) {
  const z = status?.zapret || {};
  const mod = status?.mod || {};
  const zapretRepo = mod.zapret_repo || 'https://github.com/bol-van/zapret';
  const modRepo = mod.mod_repo || 'https://github.com/bol-van/zapret';

  const [checkingZpui, setCheckingZpui] = useState(false);
  const [checkingZapret, setCheckingZapret] = useState(false);
  const [zpuiUpdate, setZpuiUpdate] = useState(null);
  const [zapretUpdate, setZapretUpdate] = useState(null);

  const checkZpuiUpdate = async () => {
    setCheckingZpui(true);
    setZpuiUpdate(null);
    try {
      const d = await api('GET', '/api/updates/check/zpui');
      if (d) {
        setZpuiUpdate(d);
        if (!d.update_available) showToast('Обновлений ZPUI нет', 'success');
      } else {
        setZpuiUpdate({ error: 'Не удалось проверить обновления' });
      }
    } catch {
      setZpuiUpdate({ error: 'Ошибка проверки' });
    }
    setCheckingZpui(false);
  };

  const checkZapretUpdate = async () => {
    setCheckingZapret(true);
    setZapretUpdate(null);
    try {
      const d = await api('GET', '/api/updates/check/zapret');
      if (d) {
        setZapretUpdate(d);
        if (!d.update_available) showToast('Обновлений Zapret нет', 'success');
      } else {
        setZapretUpdate({ error: 'Не удалось проверить обновления' });
      }
    } catch {
      setZapretUpdate({ error: 'Ошибка проверки' });
    }
    setCheckingZapret(false);
  };

  return (
    <div className="about-page">
      <div className="about-hero">
        <div className="about-hero-icon">
          <svg viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
        </div>
        <div className="about-hero-text">
          <h1 className="about-hero-title">ZPUI</h1>
          <p className="about-hero-subtitle">Модуль управления Zapret — графический интерфейс для обхода DPI</p>
        </div>
        <div className="about-hero-version">
          <span className="about-ver-badge">v{mod.version || '1.0.0'}</span>
        </div>
      </div>

      <div className="about-info-grid">
        <div className="about-info-card">
          <div className="about-info-icon">
            <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>
          </div>
          <div className="about-info-content">
            <span className="about-info-label">Версия ZPUI</span>
            <span className="about-info-value">{mod.version || '—'}</span>
          </div>
        </div>
        <div className="about-info-card">
          <div className="about-info-icon">
            <svg viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
          </div>
          <div className="about-info-content">
            <span className="about-info-label">Версия Zapret</span>
            <span className="about-info-value">{z.version || '—'}</span>
          </div>
        </div>
        <div className="about-info-card">
          <div className="about-info-icon">
            <svg viewBox="0 0 24 24"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
          </div>
          <div className="about-info-content">
            <span className="about-info-label">Стратегия</span>
            <span className="about-info-value">{z.strategy || '—'}</span>
          </div>
        </div>
        <div className="about-info-card">
          <div className="about-info-icon">
            <svg viewBox="0 0 24 24"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
          </div>
          <div className="about-info-content">
            <span className="about-info-label">Веб-порт</span>
            <span className="about-info-value mono">{mod.web_port || 8080}</span>
          </div>
        </div>
        <div className="about-info-card">
          <div className="about-info-icon">
            <svg viewBox="0 0 24 24"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>
          </div>
          <div className="about-info-content">
            <span className="about-info-label">Прокси-порт</span>
            <span className="about-info-value mono">{status?.proxy?.port || '—'}</span>
          </div>
        </div>
        <div className="about-info-card">
          <div className="about-info-icon">
            <svg viewBox="0 0 24 24"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
          </div>
          <div className="about-info-content">
            <span className="about-info-label">Путь Zapret</span>
            <span className="about-info-value mono about-path">{z.zapretDir || '—'}</span>
          </div>
        </div>
      </div>

      <div className="about-updates-grid">
        <div className="about-update-card">
          <div className="about-update-top">
            <div className="about-update-info">
              <span className="about-update-name">ZPUI</span>
              <span className="about-update-ver">Текущая: v{mod.version || '—'}</span>
            </div>
            <button className="btn btn-sm" onClick={checkZpuiUpdate} disabled={checkingZpui}>
              {checkingZpui ? 'Проверка...' : 'Проверить'}
            </button>
          </div>
          {zpuiUpdate && (
            <div className={'about-update-result ' + (zpuiUpdate.error ? 'error' : zpuiUpdate.update_available ? 'available' : 'latest')}>
              {zpuiUpdate.error ? zpuiUpdate.error : zpuiUpdate.update_available
                ? <>Доступна версия: <strong>v{zpuiUpdate.latest_version}</strong></>
                : 'Установлена последняя версия ✓'}
            </div>
          )}
        </div>
        <div className="about-update-card">
          <div className="about-update-top">
            <div className="about-update-info">
              <span className="about-update-name">Zapret</span>
              <span className="about-update-ver">Текущая: {z.version || '—'}</span>
            </div>
            <button className="btn btn-sm" onClick={checkZapretUpdate} disabled={checkingZapret}>
              {checkingZapret ? 'Проверка...' : 'Проверить'}
            </button>
          </div>
          {zapretUpdate && (
            <div className={'about-update-result ' + (zapretUpdate.error ? 'error' : zapretUpdate.update_available ? 'available' : 'latest')}>
              {zapretUpdate.error ? zapretUpdate.error : zapretUpdate.update_available
                ? <>Доступна версия: <strong>{zapretUpdate.latest_version}</strong></>
                : 'Установлена последняя версия ✓'}
            </div>
          )}
        </div>
      </div>

      <div className="about-update-card" style={{ gridColumn: '1 / -1' }}>
        <div className="settings-card-header">
          <h3>Ссылки</h3>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <a className="about-link" href="#" onClick={e => { e.preventDefault(); openExternal(zapretRepo); }}>
            <div className="about-link-icon">
              <svg width="16" height="16" viewBox="0 0 24 24"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>
            </div>
            <div className="about-link-text">
              <span>Zapret на GitHub</span>
              <span className="about-link-url mono">{zapretRepo}</span>
            </div>
            <svg width="14" height="14" viewBox="0 0 24 24"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
          </a>
          <a className="about-link" href="#" onClick={e => { e.preventDefault(); openExternal(modRepo); }}>
            <div className="about-link-icon">
              <svg width="16" height="16" viewBox="0 0 24 24"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>
            </div>
            <div className="about-link-text">
              <span>ZPUI на GitHub</span>
              <span className="about-link-url mono">{modRepo}</span>
            </div>
            <svg width="14" height="14" viewBox="0 0 24 24"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
          </a>
          <a className="about-link" href="#" onClick={e => { e.preventDefault(); openExternal('https://github.com/bol-van/zapret/issues'); }}>
            <div className="about-link-icon">
              <svg width="16" height="16" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/></svg>
            </div>
            <div className="about-link-text">
              <span>Сообщить о проблеме</span>
              <span className="about-link-url mono">GitHub Issues</span>
            </div>
            <svg width="14" height="14" viewBox="0 0 24 24"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
          </a>
        </div>
      </div>
    </div>
  );
}
