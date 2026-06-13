import { useState, useEffect, useCallback, useRef } from 'react';
import Sidebar from './components/Sidebar';
import Toast from './components/Toast';
import LogDrawer from './components/LogDrawer';
import OfflineScreen from './components/OfflineScreen';
import MonitorPage from './pages/MonitorPage';
import DevicesPage from './pages/DevicesPage';
import FiltersPage from './pages/FiltersPage';
import DiagnosticsPage from './pages/DiagnosticsPage';
import GeneralPage from './pages/GeneralPage';
import AboutPage from './pages/AboutPage';
import { api } from './api';
import './App.css';

const PAGES = {
  monitor: MonitorPage,
  devices: DevicesPage,
  filters: FiltersPage,
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
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
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
    return () => { alive = false; clearInterval(iv); };
  }, []);

  const PageComponent = PAGES[activePage];

  if (!backendOnline) {
    return (
      <div className={'app' + (sidebarCollapsed ? ' sidebar-collapsed' : '')}>
        <Sidebar
          activePage={activePage}
          onNavigate={setActivePage}
          status={status}
          showToast={showToast}
          onOpenLogs={() => setLogsOpen(true)}
          collapsed={sidebarCollapsed}
          onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
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
    <div className={'app' + (sidebarCollapsed ? ' sidebar-collapsed' : '')}>
      <Sidebar
        activePage={activePage}
        onNavigate={setActivePage}
        status={status}
        showToast={showToast}
        onOpenLogs={() => setLogsOpen(true)}
        collapsed={sidebarCollapsed}
        onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
      />
      <main className="main-content">
        <div className="content-inner page-fade" key={activePage}>
          {PageComponent && <PageComponent status={status} showToast={showToast} />}
        </div>
      </main>
      <LogDrawer open={logsOpen} onClose={() => setLogsOpen(false)} />
      <Toast toasts={toasts} onRemove={removeToast} />
    </div>
  );
}