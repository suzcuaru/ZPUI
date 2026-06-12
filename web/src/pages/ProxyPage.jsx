import { useState, useEffect, useRef, useCallback } from 'react';
import Card from '../components/Card';
import Switch from '../components/Switch';
import CopyBtn from '../components/CopyBtn';
import Skeleton from '../components/Skeleton';
import { api, apiCall } from '../api';
import { LANG } from '../lang';
import { formatBytes } from '../utils';

export default function ProxyPage({ status, showToast }) {
  const [config, setConfig] = useState({ port: 1080, username: '', password: '', auto_start: false });
  const [connections, setConnections] = useState([]);
  const [devices, setDevices] = useState([]);
  const [deviceInfo, setDeviceInfo] = useState({});
  const [lists, setLists] = useState([]);
  const [editingList, setEditingList] = useState(null);
  const [editContent, setEditContent] = useState('');
  const [savingList, setSavingList] = useState(false);
  const saveTimer = useRef(null);

  const p = status?.proxy || {};
  const net = status?.network || {};
  const pRun = p.running === true;
  const port = p.port || config.port;

  useEffect(() => { loadConfig(); loadLists(); }, []);

  useEffect(() => {
    if (!pRun) { setConnections([]); setDevices([]); setDeviceInfo({}); return; }
    const load = async () => {
      const [c, d] = await Promise.all([
        api('GET', '/api/proxy/connections'),
        api('GET', '/api/monitor/devices'),
      ]);
      if (c) { setConnections(c.connections || []); setDeviceInfo(c.device_info || {}); }
      if (d) setDevices(d.devices || []);
    };
    load();
    const iv = setInterval(load, 4000);
    return () => clearInterval(iv);
  }, [pRun]);

  const loadConfig = async () => { const d = await api('GET', '/api/proxy/config'); if (d) setConfig({ port: d.port || 1080, username: d.username || '', password: d.password || '', auto_start: d.auto_start || false }); };
  const loadLists = async () => { const d = await api('GET', '/api/zapret/lists'); if (d) setLists(d.lists || []); };

  const buildURL = (ip) => {
    let u = 'socks5://';
    if (config.username) u += encodeURIComponent(config.username) + ':' + encodeURIComponent(config.password) + '@';
    return u + ip + ':' + port;
  };

  const lanIP = net.ips?.[0] || '127.0.0.1';

  const save = useCallback(async (cfg) => {
    await apiCall(() => api('POST', '/api/proxy/config', cfg), LANG.saved, showToast);
  }, [showToast]);

  const updateConfig = useCallback((patch) => {
    setConfig(prev => {
      const next = { ...prev, ...patch };
      clearTimeout(saveTimer.current);
      saveTimer.current = setTimeout(() => save(next), 500);
      return next;
    });
  }, [save]);

  const openListEditor = (list) => { setEditingList(list.name); setEditContent(list.lines.join('\n')); };
  const handleSaveList = async () => {
    setSavingList(true);
    await apiCall(() => api('POST', '/api/zapret/lists/save', { name: editingList, content: editContent + '\n' }), LANG.saved, showToast);
    setSavingList(false); setEditingList(null); loadLists();
  };

  const totalConns = connections.length;
  const totalDown = connections.reduce((s, c) => s + (c.bytes_recv || 0), 0);
  const totalUp = connections.reduce((s, c) => s + (c.bytes_sent || 0), 0);

  const listLabels = {
    'list-general.txt': 'Домены (общий)', 'list-general-user.txt': 'Домены (пользовательский)',
    'list-exclude.txt': 'Исключения (общий)', 'list-exclude-user.txt': 'Исключения (пользовательский)',
    'ipset-all.txt': 'IP-адреса (все)', 'ipset-exclude.txt': 'IP исключения (общий)',
    'ipset-exclude-user.txt': 'IP исключения (пользовательский)',
  };

  return (
    <div className="page-fade">
      <div className="page-title">Прокси</div>
      <div className="ov-grid">

        {/* ── Stats Row ───────────────────── */}
        <div className="ov-speed-card">
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">Статус</span>
              <span className="ov-speed-value" style={{ color: pRun ? 'var(--success)' : 'var(--danger)', fontSize: 14 }}>{pRun ? 'Работает' : 'Остановлен'}</span>
            </div>
            <span className="ov-speed-total">Порт {port}</span>
          </div>
          <div className="ov-speed-divider"></div>
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">Подключения</span>
              <span className="ov-speed-value">{totalConns}</span>
            </div>
            <span className="ov-speed-total">Устройств {devices.length}</span>
          </div>
          <div className="ov-speed-divider"></div>
          <div className="ov-speed-item">
            <div className="ov-speed-head">
              <span className="ov-speed-label">↓{formatBytes(totalDown)}</span>
              <span className="ov-speed-label">↑{formatBytes(totalUp)}</span>
            </div>
            <span className="ov-speed-total">Всего трафика</span>
          </div>
        </div>

        {/* ── Config + URL ────────────────── */}
        <div className="ov-config-row">
          <div className="ov-config-card">
            <div className="ov-config-title">Настройки</div>
            <div className="settings-form">
              <div className="form-group">
                <label>Порт</label>
                <input type="number" className="form-input" value={config.port} min="1" max="65535" onChange={e => updateConfig({ port: parseInt(e.target.value) || 1080 })} />
              </div>
              <div className="form-group">
                <label>Пользователь</label>
                <input type="text" className="form-input" placeholder="Не обязательно" value={config.username} onChange={e => updateConfig({ username: e.target.value })} />
              </div>
              <div className="form-group">
                <label>Пароль</label>
                <input type="password" className="form-input" placeholder="Не обязательно" autoComplete="new-password" value={config.password} onChange={e => updateConfig({ password: e.target.value })} />
              </div>
              <div className="form-row">
                <span>Автозапуск</span>
                <Switch checked={config.auto_start} onChange={() => updateConfig({ auto_start: !config.auto_start })} />
              </div>
            </div>
          </div>

          {pRun && (
            <div className="ov-config-card">
              <div className="ov-config-title">Подключение</div>
              <div className="socks-url"><span>{buildURL(lanIP)}</span><CopyBtn text={buildURL(lanIP)} onCopied={() => showToast(LANG.copied, 'success')} /></div>
              {net.ips?.length > 1 && net.ips.filter(ip => ip !== lanIP).map(ip => (
                <div key={ip} className="socks-url secondary"><span>{buildURL(ip)}</span><CopyBtn text={buildURL(ip)} onCopied={() => showToast(LANG.copied, 'success')} /></div>
              ))}
              {devices.length > 0 && (
                <div className="ov-connected-devices">
                  {devices.map(dev => {
                    const host = dev.hostname || deviceInfo[dev.ip]?.hostname || '';
                    const mac = deviceInfo[dev.ip]?.mac || '';
                    return (
                      <div key={dev.ip} className="ov-cdevice">
                        <span className="ov-cdevice-ip mono">{dev.ip}</span>
                        {host && <span className="ov-cdevice-host">{host}</span>}
                        {mac && <span className="ov-cdevice-mac mono">{mac}</span>}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}
        </div>

        {/* ── Filter Lists ────────────────── */}
        <div className="ov-conns-card">
          <div className="ov-conns-header">
            <span className="ov-conns-title">Списки фильтрации ({lists.length})</span>
          </div>
          {editingList ? (
            <div className="ov-list-editor">
              <div className="ov-list-editor-head">
                <span className="ov-list-editor-name">{listLabels[editingList] || editingList}</span>
                <div className="btn-row"><button className="btn btn-sm" onClick={() => setEditingList(null)}>Отмена</button><button className="btn btn-accent btn-sm" onClick={handleSaveList} disabled={savingList}>{savingList ? '...' : 'Сохранить'}</button></div>
              </div>
              <textarea className="form-input list-editor" value={editContent} onChange={e => setEditContent(e.target.value)} placeholder="Один домен или IP на строку" rows={8} />
            </div>
          ) : (
            <div className="ov-lists">
              {lists.map((list, i) => (
                <div key={i} className="ov-list-item">
                  <div className="ov-list-info">
                    <span className="ov-list-name">{listLabels[list.name] || list.name}</span>
                    <span className="ov-list-count">{list.count}</span>
                  </div>
                  {list.editable ? (
                    <button className="btn btn-sm" onClick={() => openListEditor(list)}>Ред.</button>
                  ) : (
                    <div className="ov-list-lines">
                      {list.lines.slice(0, 4).map((l, j) => <span key={j} className="ov-list-tag mono">{l}</span>)}
                      {list.count > 4 && <span className="ov-list-tag more">+{list.count - 4}</span>}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
