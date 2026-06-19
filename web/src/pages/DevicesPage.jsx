import { useState, useEffect, useRef, useCallback } from 'react';
import Modal from '../components/ui/Modal';
import { api } from '../api';

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function DeviceConnections({ mac }) {
  const [connections, setConnections] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const load = async () => {
      const r = await api('GET', `/api/devices/${encodeURIComponent(mac)}/connections?limit=50`);
      if (r) setConnections(r.connections || []);
      setLoading(false);
    };
    load();
  }, [mac]);

  if (loading) return <div className="ov-empty" style={{ padding: 12 }}>Загрузка соединений...</div>;
  if (connections.length === 0) return <div className="ov-empty" style={{ padding: 12 }}>Нет данных о соединениях</div>;

  return (
    <div className="dd-connections">
      {connections.map((c, i) => (
        <div key={c.id || i} className="dd-conn-item">
          <div className="dd-conn-host">{c.dst_host || '—'}:{c.dst_port}</div>
          <div className="dd-conn-traffic">
            ↓{formatBytes(c.bytes_dl)} ↑{formatBytes(c.bytes_ul)}
          </div>
          <div className="dd-conn-time">
            {new Date(c.started_at).toLocaleTimeString()}
          </div>
        </div>
      ))}
    </div>
  );
}

export default function DevicesPage({ showToast }) {
  const [devices, setDevices] = useState([]);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState('all');
  const [selectedDevice, setSelectedDevice] = useState(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [pingingMAC, setPingingMAC] = useState(null);
  const [pingResult, setPingResult] = useState(null);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState('overview');
  const timerRef = useRef(null);

  const fetchDevices = useCallback(async () => {
    const d = await api('GET', '/api/devices');
    if (d) {
      setDevices(d.devices || []);
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchDevices();
    timerRef.current = setInterval(fetchDevices, 10000);
    return () => clearInterval(timerRef.current);
  }, [fetchDevices]);

  const handlePing = async (mac) => {
    setPingingMAC(mac);
    setPingResult(null);
    const r = await api('POST', `/api/devices/${encodeURIComponent(mac)}/ping`);
    setPingResult(r);
    setPingingMAC(null);
    if (r?.success) {
      showToast?.(`Ping ${r.avg_ms.toFixed(1)} мс`, 'success');
    } else {
      showToast?.('Ping не удался', 'error');
    }
  };

  const handleDelete = async (mac) => {
    await api('DELETE', `/api/devices/${encodeURIComponent(mac)}`);
    showToast?.('Устройство удалено', 'success');
    fetchDevices();
    setDrawerOpen(false);
  };

  const openDrawer = (device) => {
    setSelectedDevice(device);
    setDrawerOpen(true);
    setPingResult(null);
    setActiveTab('overview');
  };

  const filtered = devices.filter(d => {
    if (filter === 'online' && !d.is_online) return false;
    if (filter === 'offline' && d.is_online) return false;
    if (search) {
      const q = search.toLowerCase();
      return (d.mac || '').toLowerCase().includes(q) ||
             (d.ip || '').toLowerCase().includes(q) ||
             (d.hostname || '').toLowerCase().includes(q);
    }
    return true;
  });

  const onlineCount = devices.filter(d => d.is_online).length;
  const offlineCount = devices.length - onlineCount;

  return (
    <>
      <div className="devices-toolbar">
        <div className="devices-stats">
          <span style={{ color: 'var(--success)', fontSize: 12, fontWeight: 500 }}>● {onlineCount}</span>
          <span style={{ color: 'var(--text-tertiary)', fontSize: 12, fontWeight: 500 }}>● {offlineCount}</span>
        </div>
        <div className="devices-search">
          <input
            type="text"
            className="form-input devices-search-input"
            placeholder="Поиск по MAC, IP, имени..."
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
        </div>
        <div className="devices-filters">
          {['all', 'online', 'offline'].map(f => (
            <button key={f} className={'btn btn-sm' + (filter === f ? ' btn-accent' : '')}
              onClick={() => setFilter(f)}>
              {f === 'all' ? 'Все' : f === 'online' ? 'Онлайн' : 'Офлайн'}
            </button>
          ))}
        </div>
      </div>

      {loading ? (
        <div className="ov-empty">Загрузка устройств...</div>
      ) : filtered.length === 0 ? (
        <div className="ov-empty">
          <p>Устройства не обнаружены</p>
          <p style={{ fontSize: 12, marginTop: 4, opacity: 0.6 }}>Попробуйте обновить страницу или подождать</p>
        </div>
      ) : (
        <div className="devices-grid">
          {filtered.map(d => (
            <div key={d.mac} className={'device-card' + (d.is_online ? ' online' : ' offline')}
              onClick={() => openDrawer(d)}>
              <div className="dc-status-dot" />
              <div className="dc-info">
                <div className="dc-hostname">{d.hostname || d.mac}</div>
                <div className="dc-ip">{d.ip || '—'}</div>
              </div>
              <div className="dc-meta">
                {d.is_online && <span className="badge badge-success" style={{ fontSize: 10 }}>онлайн</span>}
                <span className="dc-traffic">↓{formatBytes(d.total_dl)}</span>
              </div>
            </div>
          ))}
        </div>
      )}

      <Modal open={drawerOpen} onClose={() => setDrawerOpen(false)} title={selectedDevice?.hostname || selectedDevice?.mac || 'Устройство'}>
        {selectedDevice && (
          <div className="device-drawer">
            <div className="dd-tabs">
              {['overview', 'connections'].map(t => (
                <button key={t} className={'dd-tab' + (activeTab === t ? ' active' : '')}
                  onClick={() => setActiveTab(t)}>
                  {t === 'overview' ? 'Обзор' : 'Соединения'}
                </button>
              ))}
            </div>

            {activeTab === 'overview' && (
              <div className="dd-overview">
                <div className="dd-field">
                  <span className="dd-label">MAC</span>
                  <span className="dd-value">{selectedDevice.mac}</span>
                </div>
                <div className="dd-field">
                  <span className="dd-label">IP</span>
                  <span className="dd-value">{selectedDevice.ip || '—'}</span>
                </div>
                <div className="dd-field">
                  <span className="dd-label">Имя</span>
                  <span className="dd-value">{selectedDevice.hostname || '—'}</span>
                </div>
                <div className="dd-field">
                  <span className="dd-label">Статус</span>
                  <span className={'dd-value ' + (selectedDevice.is_online ? 'status-online' : 'status-offline')}>
                    {selectedDevice.is_online ? 'Онлайн' : 'Офлайн'}
                  </span>
                </div>
                <div className="dd-field">
                  <span className="dd-label">Трафик</span>
                  <span className="dd-value">↓{formatBytes(selectedDevice.total_dl)} ↑{formatBytes(selectedDevice.total_ul)}</span>
                </div>
                <div className="dd-field">
                  <span className="dd-label">Видели</span>
                  <span className="dd-value">{new Date(selectedDevice.last_seen).toLocaleString()}</span>
                </div>

                <div className="dd-actions">
                  <button className="btn btn-accent btn-sm"
                    onClick={() => handlePing(selectedDevice.mac)}
                    disabled={pingingMAC === selectedDevice.mac}>
                    {pingingMAC === selectedDevice.mac ? 'Пингую...' : 'Ping'}
                  </button>
                  <button className="btn btn-danger btn-sm" onClick={() => handleDelete(selectedDevice.mac)}>
                    Забыть
                  </button>
                </div>

                {pingResult && (
                  <div className={'dd-ping-result ' + (pingResult.success ? 'ok' : 'fail')}>
                    {pingResult.success ? (
                      <>Средняя задержка: <strong>{pingResult.avg_ms.toFixed(1)} мс</strong></>
                    ) : (
                      <>Ping не удался: {pingResult.error}</>
                    )}
                  </div>
                )}
              </div>
            )}

            {activeTab === 'connections' && (
              <DeviceConnections mac={selectedDevice.mac} />
            )}
          </div>
        )}
      </Modal>
    </>
  );
}
