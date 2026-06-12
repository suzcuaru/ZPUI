import { useState, useEffect } from 'react';
import Card from '../components/Card';
import Switch from '../components/Switch';
import { api } from '../api';
import { LANG } from '../lang';

export default function ListsPage({ status, showToast }) {
  const [gameFilter, setGameFilter] = useState('disabled');
  const [ipsetStatus, setIpsetStatus] = useState('loaded');
  const [autoUpdate, setAutoUpdate] = useState(true);

  useEffect(() => { loadFilters(); }, []);

  const loadFilters = async () => {
    const gf = await api('GET', '/api/zapret/game-filter');
    if (gf) setGameFilter(gf.mode || 'disabled');
    const ip = await api('GET', '/api/zapret/ipset-status');
    if (ip) setIpsetStatus(ip.status || 'loaded');
    const au = await api('GET', '/api/zapret/auto-update-status');
    if (au) setAutoUpdate(au.enabled !== false);
  };

  const handleGameFilter = async (mode) => {
    setGameFilter(mode);
    await apiCall(() => api('POST', '/api/zapret/game-filter', { mode }), LANG.gameFilterSet, showToast);
  };

  const handleIpsetToggle = async () => {
    const next = ipsetStatus === 'loaded' ? 'none' : ipsetStatus === 'none' ? 'any' : 'loaded';
    await apiCall(() => api('POST', '/api/zapret/ipset-toggle', { mode: next }), 'IPSet обновлён', showToast);
    loadFilters();
  };

  const handleAutoUpdate = async () => {
    const next = !autoUpdate;
    setAutoUpdate(next);
    await apiCall(() => api('POST', '/api/zapret/auto-update-toggle', { enabled: next }), 'Настройка обновлений сохранена', showToast);
  };

  return (
    <>
      <Card className="settings-card">
        <div className="settings-card-header">
          <h3>Фильтры и настройки</h3>
          <p>Game Filter, IPSet, автообновления</p>
        </div>
        <div className="settings-section-label">Game Filter</div>
        <div className="radio-group settings-radio">
          {[
            { v: 'disabled', l: 'Выключен', d: 'Не добавлять игровые порты' },
            { v: 'all', l: 'TCP + UDP', d: 'Добавить все порты 1024-65535' },
            { v: 'tcp', l: 'Только TCP', d: 'Только TCP-порты' },
            { v: 'udp', l: 'Только UDP', d: 'Только UDP-порты' },
          ].map(o => (
            <label key={o.v} className="radio-item">
              <input type="radio" name="gf" checked={gameFilter === o.v} onChange={() => handleGameFilter(o.v)} />
              <div className="radio-content">
                <span className="radio-label">{o.l}</span>
                {gameFilter === o.v && <span className="radio-desc">{o.d}</span>}
              </div>
            </label>
          ))}
        </div>
        <div className="settings-divider"></div>
        <div className="settings-section-label">IPSet Filter</div>
        <div className="settings-switch-row">
          <div className="settings-switch-info">
            <span className="settings-switch-title">IPSet фильтр</span>
            <span className="settings-switch-desc">Текущий статус: <strong>{ipsetStatus}</strong></span>
          </div>
          <button className="btn btn-sm" onClick={handleIpsetToggle}>
            {ipsetStatus === 'loaded' ? 'Отключить' : ipsetStatus === 'none' ? 'Включить (пустой)' : 'Загрузить'}
          </button>
        </div>
        <div className="settings-divider"></div>
        <div className="settings-section-label">Auto-Update</div>
        <div className="settings-switch-row">
          <div className="settings-switch-info">
            <span className="settings-switch-title">Автопроверка обновлений</span>
            <span className="settings-switch-desc">Проверять обновления при запуске</span>
          </div>
          <Switch checked={autoUpdate} onChange={handleAutoUpdate} />
        </div>
      </Card>
    </>
  );
}
