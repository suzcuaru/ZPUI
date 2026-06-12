import { useState, useEffect, useCallback, useRef } from 'react';
import Sidebar from './components/Sidebar';
import Toast from './components/Toast';
import LogDrawer from './components/LogDrawer';
import OfflineScreen from './components/OfflineScreen';
import MonitorPage from './pages/MonitorPage';
import StrategyPage from './pages/StrategyPage';
import ListsPage from './pages/ListsPage';
import DiagnosticsPage from './pages/DiagnosticsPage';
import GeneralPage from './pages/GeneralPage';
import AboutPage from './pages/AboutPage';
import { api } from './api';
import { logSnapshot, cleanOld } from './db';
import './App.css';

const PAGES = {
  monitor: MonitorPage,
  strategy: StrategyPage,
  lists: ListsPage,
  diagnostics: DiagnosticsPage,
  general: GeneralPage,
  about: AboutPage,
};

export default function App() {
  const [activePage, setActivePage] = useState('monitor');
  const [status, setStatus] = useState(null);
  const [toasts, setToasts] = useState([]);
  const [logsOpen, setLogsOpen] = useState(false);
  const [backendOnline, setBackendOnline] = useState(true);
  const failCountRef = useRef(0);

  const showToast = useCallback((msg, type) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, msg, type }]);
    if (type !== 'error') {
      setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 3000);
    }
  }, []);

  const removeToast = useCallback((id) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  useEffect(() => {
    let alive = true;
    const poll = async () => {
      const data = await api('GET', '/api/status');
      if (!alive) return;
      if (data) {
        if (failCountRef.current > 0) console.log('[App] Backend reconnected');
        failCountRef.current = 0;
        setBackendOnline(true);
        setStatus(data);
        try {
          const m = data.monitor || {};
          const p = data.proxy || {};
          logSnapshot({
            dl: m.dl_speed || 0,
            ul: m.ul_speed || 0,
            dlFmt: m.dl_speed_fmt || '0 B/s',
            ulFmt: m.ul_speed_fmt || '0 B/s',
            totalDl: m.download || 0,
            totalUl: m.upload || 0,
            conns: p.connections || 0,
            zRunning: data.zapret?.status === 'running',
            pRunning: p.running === true,
          });
        } catch {}
      } else {
        failCountRef.current++;
        if (failCountRef.current === 3) {
          console.error('[App] Backend unreachable (3 failures)');
          setBackendOnline(false);
        }
      }
    };
    console.log('[App] Initializing ZPUI');
    poll();
    const iv = setInterval(poll, 2000);
    const cleanIv = setInterval(cleanOld, 60000);
    return () => { alive = false; clearInterval(iv); clearInterval(cleanIv); };
  }, []);

  const PageComponent = PAGES[activePage];

  if (!backendOnline) {
    return (
      <div className="app">
        <Sidebar
          activePage={activePage}
          onNavigate={setActivePage}
          status={status}
          showToast={showToast}
          onOpenLogs={() => setLogsOpen(true)}
        />
        <main className="main-content">
          <OfflineScreen onRetry={async () => {
            const d = await api('GET', '/api/status');
            if (d) {
              setBackendOnline(true);
              failCountRef.current = 0;
              setStatus(d);
            } else {
              showToast('Бэкенд по-прежнему недоступен', 'error');
            }
          }} />
        </main>
      </div>
    );
  }

  return (
    <div className="app">
      <Sidebar
        activePage={activePage}
        onNavigate={setActivePage}
        status={status}
        showToast={showToast}
        onOpenLogs={() => setLogsOpen(true)}
      />
      <main className="main-content">
        <div className="content-inner page-fade" key={activePage}>
          {PageComponent && <PageComponent status={status} showToast={showToast} />}
        </div>
      </main>
      <button className="fab-logs" onClick={() => setLogsOpen(true)} title="Логи">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
      </button>
      <LogDrawer open={logsOpen} onClose={() => setLogsOpen(false)} />
      <Toast toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
