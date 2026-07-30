import { useState, useEffect, useCallback, useRef } from 'react';
import Sidebar from './components/layout/Sidebar';
import Footer from './components/layout/Footer';
import Toast from './components/feedback/Toast';
import RecoveryToast from './components/feedback/RecoveryToast';
import RecoveryFailedModal from './components/feedback/RecoveryFailedModal';
import LogDrawer from './components/navigation/LogDrawer';
import OfflineScreen from './components/feedback/OfflineScreen';
import ResourceChecker from './components/ResourceChecker';
import HealthCheckModal from './components/HealthCheckModal';
import AutoSelectModal from './components/AutoSelectModal';
import StartupScreen from './components/StartupScreen';
import DiagnosticsModal from './components/DiagnosticsModal';
import UpdateAvailableModal from './components/UpdateAvailableModal';
import { ConfirmProvider } from './components/ui/ConfirmDialog';
import DashboardPage from './pages/DashboardPage';
import ZapretPage from './pages/ZapretPage';
import SettingsPage from './pages/SettingsPage';
import MonitorPage from './pages/MonitorPage';
import ProxyPage from './pages/ProxyPage';
import XboxDnsPage from './pages/XboxDnsPage';
import DocsPage from './pages/DocsPage';
import ReportPage from './pages/ReportPage';
import { api, apiCall } from './api';
import { useT } from './i18n';
import { usePolling } from './hooks/usePolling';
import { useUpdateCheck, checkZpuiUpdate, checkZapretUpdate, shouldNotifyZpui, shouldNotifyZapret, setZpuiCheck, setZapretCheck } from './hooks/useUpdateCheck';
import './styles/index.css';

const PAGES = {
  dashboard: DashboardPage,
  zapret: ZapretPage,
  proxy: ProxyPage,
  xboxdns: XboxDnsPage,
  settings: SettingsPage,
  monitor: MonitorPage,
  report: ReportPage,
  docs: DocsPage,
};

export default function App() {
  const { t } = useT();
  const { zpuiCheck, zapretCheck } = useUpdateCheck();
  const [startupDone, setStartupDone] = useState(false);
  const [activePage, setActivePage] = useState('dashboard');
  const [status, setStatus] = useState(null);
  const [toasts, setToasts] = useState([]);
  const [logsOpen, setLogsOpen] = useState(false);
  const [logErrorCode, setLogErrorCode] = useState(null);
  const [checkerOpen, setCheckerOpen] = useState(false);
  const [checkerInitialUrl, setCheckerInitialUrl] = useState('');
  const [autoSelectOpen, setAutoSelectOpen] = useState(false);
  const [backendOnline, setBackendOnline] = useState(true);
  const [theme, setTheme] = useState('dark');
  const [themeMode, setThemeMode] = useState('system');
  const [healthOpen, setHealthOpen] = useState(false);
  const [healthWarn, setHealthWarn] = useState(null);
  const [hasLogErrors, setHasLogErrors] = useState(false);
  const [diagOpen, setDiagOpen] = useState(false);
  const [zapretLoading, setZapretLoading] = useState(false);
  const [recovery, setRecovery] = useState({ open: false, steps: [], done: false, success: false });
  const [recoveryReport, setRecoveryReport] = useState(null);
  const [autoSelectAutoStart, setAutoSelectAutoStart] = useState(false);
  const [autoUpdateCheck, setAutoUpdateCheck] = useState(true);
  const [updateModal, setUpdateModal] = useState(null);
  const failCountRef = useRef(0);
  const themeInitRef = useRef(false);

  const showToast = useCallback((msg, type, opts) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, msg, type, ...(opts || {}) }]);
  }, []);

  const removeToast = useCallback((id) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  const showProgressToast = useCallback((msg) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, msg, type: 'download', progress: 0, persistent: true }]);
    return id;
  }, []);

  const updateProgressToast = useCallback((id, progress, msg) => {
    setToasts(prev => prev.map(t => t.id === id ? { ...t, progress: progress || t.progress, msg: msg || t.msg } : t));
  }, []);

  const completeProgressToast = useCallback((id, msg) => {
    setToasts(prev => prev.map(t => t.id === id ? { ...t, progress: 100, msg: msg || t.msg, completed: true, persistent: false } : t));
  }, []);

  const handleOpenLogs = useCallback((errorCode) => {
    setLogErrorCode(errorCode || null);
    setLogsOpen(true);
  }, []);

  // Единый обработчик переключения Запрета для всех кнопок (sidebar dot,
  // большая кнопка на вкладке). Гарантирует синхронизацию состояния и
  // корректную реакцию на запуск восстановления.
  const onZapretToggle = useCallback(async () => {
    if (zapretLoading) return;
    const zRun = status?.zapret?.status === 'running';
    setZapretLoading(true);
    try {
      if (zRun) {
        const result = await api('POST', '/api/zapret/stop');
        if (result?.error) showToast(result.error, 'error');
        else showToast(t('header.zapretStopped'), 'success');
        setZapretLoading(false);
      } else {
        const result = await api('POST', '/api/zapret/start');
        if (result?.status === 'recovering') {
          // Бэкенд запустил восстановление — тост и кнопки управляются
          // событиями zapret:recovery:*. Не сбрасываем loading здесь.
          showToast(t('recovery.startMessage', { reason: result.reason || '' }), 'warning');
          return;
        }
        if (result?.error) {
          showToast(result.error, 'error');
          setZapretLoading(false);
          return;
        }
        // Бэкенд уже верифицировал живость — успех подтверждён.
        showToast(t('header.zapretStarted'), 'success');
        setZapretLoading(false);
      }
      await apiCall(() => api('POST', '/api/component-states'));
    } catch {
      if (showToast) showToast(t('common.error'), 'error');
      setZapretLoading(false);
    }
  }, [status, zapretLoading, showToast, t]);

  // Единая точка сохранения темы в конфиг бэкенда.
  const saveThemeConfig = useCallback((themeValue) => {
    api('POST', '/api/config', { theme: themeValue });
  }, []);

  const toggleTheme = useCallback(() => {
    const next = theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
    document.documentElement.setAttribute('data-theme', next);
    try { localStorage.setItem('zpui-theme', next); } catch {}
    // При выходе из режима "system" переходим на явную тему.
    if (themeMode === 'system') {
      setThemeMode('manual');
    }
    saveThemeConfig(next);
  }, [theme, themeMode, saveThemeConfig]);

  // Theme init — run once after startup
  useEffect(() => {
    if (!startupDone || themeInitRef.current || !status?.mod) return;
    themeInitRef.current = true;
    const savedTheme = status.mod.theme || 'system';
    if (savedTheme === 'system') {
      try { localStorage.removeItem('zpui-theme'); } catch {}
      api('GET', '/api/system-theme').then(sysTheme => {
        if (sysTheme) {
          const resolved = sysTheme === 'dark' ? 'dark' : 'light';
          setTheme(resolved);
          document.documentElement.setAttribute('data-theme', resolved);
        }
      });
    } else {
      try { localStorage.setItem('zpui-theme', savedTheme); } catch {}
      setTheme(savedTheme === 'dark' ? 'dark' : 'light');
      setThemeMode(savedTheme);
      document.documentElement.setAttribute('data-theme', savedTheme);
    }
  }, [startupDone, status]);

  // Status polling (only after startup)
  const pollStatus = async () => {
    if (!startupDone) return;
    const data = await api('GET', '/api/status');
    if (data) {
      if (failCountRef.current > 0) console.log('[App] Backend reconnected');
      failCountRef.current = 0;
      setBackendOnline(true);
      setStatus(data);
    } else {
      failCountRef.current++;
      if (failCountRef.current === 3) {
        setBackendOnline(false);
      }
    }
  };
  usePolling(pollStatus, startupDone ? 2000 : 0);

  // Health check polling
  const pollHealth = async () => {
    if (!startupDone) return;
    const h = await api('GET', '/api/health');
    setHealthWarn(h && h.overall && h.overall !== 'healthy' ? h : null);
  };
  usePolling(pollHealth, startupDone ? 30000 : 0);

  // Log error snapshot check for red sidebar icon
  const pollLogErrors = useCallback(async () => {
    if (!startupDone) return;
    const d = await api('GET', '/api/logs/errors');
    setHasLogErrors(d?.files?.length > 0);
  }, [startupDone]);
  usePolling(pollLogErrors, startupDone ? 15000 : 0);

  // Проверка обновлений управляется единым тумблером «проверка обновлений».
  useEffect(() => {
    if (!startupDone) return;
    api('GET', '/api/config').then(c => {
      if (c) setAutoUpdateCheck(c.auto_update_check !== false);
    });
  }, [startupDone]);

  // Update checks: initial delayed + hourly periodic (only if auto_update_check)
  useEffect(() => {
    if (!startupDone || !autoUpdateCheck) return;
    const initId = setTimeout(() => {
      checkZpuiUpdate();
      checkZapretUpdate();
    }, 2000);
    const hourlyId = setInterval(() => {
      checkZpuiUpdate();
      checkZapretUpdate();
    }, 3600000);
    return () => { clearTimeout(initId); clearInterval(hourlyId); };
  }, [startupDone, autoUpdateCheck]);

  // Модалка «доступно обновление» при обнаружении (дедуп по версии за сессию).
  useEffect(() => {
    if (zpuiCheck.state === 'available' && zpuiCheck.latest && shouldNotifyZpui(zpuiCheck.latest)) {
      setUpdateModal({ component: 'ZPUI', current: zpuiCheck.current, latest: zpuiCheck.latest });
    }
  }, [zpuiCheck.state, zpuiCheck.latest]);

  useEffect(() => {
    if (zapretCheck.state === 'available' && zapretCheck.latest && shouldNotifyZapret(zapretCheck.latest)) {
      setUpdateModal({ component: 'zapret', current: zapretCheck.current, latest: zapretCheck.latest });
    }
  }, [zapretCheck.state, zapretCheck.latest]);

  const launchUpdater = useCallback(async () => {
    setUpdateModal(null);
    await api('POST', '/api/updates/launch-updater');
  }, []);

  // Update available notification (dedup by version) + files-missing
  useEffect(() => {
    if (!startupDone) return;
    const handler = (data) => {
      if (!data?.latest) return;
      // Только обновляем состояние — модалка показывается отдельным эффектом
      // с дедупликацией по версии.
      if (data.component === 'ZPUI') {
        setZpuiCheck({ state: 'available', current: data.current, latest: data.latest });
      } else if (data.component === 'zapret') {
        setZapretCheck({ state: 'available', current: data.current, latest: data.latest });
      }
    };
    const filesHandler = (data) => {
      if (data?.missing?.length > 0) {
        showToast(t('toast.filesMissing', { count: data.missing.length }), 'warning');
      }
    };
    const recoveryHandler = () => {
      showToast(t('zapret.recoveringFiles'), 'info');
    };
    const recoveryCompleteHandler = () => {
      showToast(t('zapret.recoveryComplete'), 'success');
    };
    const recoveryFailedHandler = (data) => {
      showToast(t('zapret.recoveryFailed', { error: data?.error || '' }), 'error');
    };
    const autoSelectNeededHandler = () => {
      setAutoSelectAutoStart(false);
      setAutoSelectOpen(true);
    };
    const zapretNeedsAutoSelectHandler = () => {
      setAutoSelectAutoStart(false);
      setAutoSelectOpen(true);
    };
    if (window.runtime?.EventsOn) {
      window.runtime.EventsOn('update:available', handler);
      window.runtime.EventsOn('zapret:files-missing', filesHandler);
      window.runtime.EventsOn('zapret:recovering', recoveryHandler);
      window.runtime.EventsOn('zapret:recovery_complete', recoveryCompleteHandler);
      window.runtime.EventsOn('zapret:recovery_failed', recoveryFailedHandler);
      window.runtime.EventsOn('operator:auto_select_needed', autoSelectNeededHandler);
      window.runtime.EventsOn('zapret:needs_autoselect', zapretNeedsAutoSelectHandler);
    }
    return () => {
      if (window.runtime?.EventsOff) {
        window.runtime.EventsOff('update:available');
        window.runtime.EventsOff('zapret:files-missing');
        window.runtime.EventsOff('zapret:recovering');
        window.runtime.EventsOff('zapret:recovery_complete');
        window.runtime.EventsOff('zapret:recovery_failed');
        window.runtime.EventsOff('operator:auto_select_needed');
        window.runtime.EventsOff('zapret:needs_autoselect');
      }
    };
  }, [startupDone, showToast, t]);

  // Recovery procedure events — единый жёлтый тост с этапами + модалка при провале.
  useEffect(() => {
    if (!startupDone) return;
    let hideTimer = null;
    const upsertStep = (step) => {
      setRecovery((prev) => {
        const steps = [...prev.steps];
        const idx = steps.findIndex((s) => s.key === step.key);
        if (idx >= 0) steps[idx] = step; else steps.push(step);
        return { ...prev, steps };
      });
    };
    const onStart = () => {
      setZapretLoading(true);
      setRecovery({ open: true, steps: [], done: false, success: false });
    };
    const onStep = (step) => { if (step) upsertStep(step); };
    const onDone = (data) => {
      const success = !!(data && data.success);
      const report = data && data.report;
      setRecovery((prev) => ({ ...prev, done: true, success }));
      setZapretLoading(false);
      if (success) {
        if (hideTimer) clearTimeout(hideTimer);
        hideTimer = setTimeout(() => setRecovery({ open: false, steps: [], done: false, success: false }), 3500);
      } else {
        // Провал — показываем модалку с отчётом.
        setRecoveryReport(report || { error_code: data?.error_code, reason: data?.reason });
      }
    };
    if (window.runtime?.EventsOn) {
      window.runtime.EventsOn('zapret:recovery:start', onStart);
      window.runtime.EventsOn('zapret:recovery:step', onStep);
      window.runtime.EventsOn('zapret:recovery:done', onDone);
    }
    return () => {
      if (hideTimer) clearTimeout(hideTimer);
      if (window.runtime?.EventsOff) {
        window.runtime.EventsOff('zapret:recovery:start');
        window.runtime.EventsOff('zapret:recovery:step');
        window.runtime.EventsOff('zapret:recovery:done');
      }
    };
  }, [startupDone]);

  const handleStartupComplete = useCallback(() => {
    setStartupDone(true);
    if (!localStorage.getItem('zpui-manual-seen')) {
      localStorage.setItem('zpui-manual-seen', '1');
      setActivePage('docs');
    }
  }, []);

  // Show startup screen when not done
  if (!startupDone) {
    return (
      <>
        <StartupScreen onComplete={handleStartupComplete} />
        <Toast toasts={toasts} onRemove={removeToast} version={status?.mod?.version} />
      </>
    );
  }

  const PageComponent = PAGES[activePage];

  const pageContent = backendOnline ? (
    <div className="main-area page-fade" key={activePage}>
        {PageComponent ? (
         <PageComponent status={status} showToast={showToast} showProgressToast={showProgressToast} updateProgressToast={updateProgressToast} completeProgressToast={completeProgressToast} onNavigate={setActivePage} onOpenLogs={() => setLogsOpen(true)} onOpenDiagnostics={() => setDiagOpen(true)} onOpenCheckerWithUrl={(url) => { setCheckerInitialUrl(url); setCheckerOpen(true); }} zapretLoading={zapretLoading} onZapretToggle={onZapretToggle} />
       ) : null}
    </div>
  ) : (
    <div className="main-area">
      <OfflineScreen onRetry={async () => {
        const d = await api('GET', '/api/status');
        if (d) {
          setBackendOnline(true);
          failCountRef.current = 0;
          setStatus(d);
        } else {
          showToast(t('offline.backendUnavailable'), 'error');
        }
      }} />
    </div>
  );

  return (
    <ConfirmProvider>
    <div className="app">
        <Sidebar
          activePage={activePage}
          onNavigate={setActivePage}
          onOpenChecker={() => setCheckerOpen(true)}
          onAutoSelect={() => { setAutoSelectAutoStart(true); setAutoSelectOpen(true); }}
          onOpenHealth={() => setHealthOpen(true)}
          onOpenHelp={() => setActivePage('docs')}
          healthWarn={healthWarn}
          hasLogErrors={hasLogErrors}
          status={status}
          showToast={showToast}
          onOpenLogs={() => setLogsOpen(true)}
          zapretLoading={zapretLoading}
          onZapretToggle={onZapretToggle}
        />
      <div className="app-body">
        {pageContent}
        <Footer status={status} />
      </div>
      <LogDrawer open={logsOpen} onClose={() => { setLogsOpen(false); setLogErrorCode(null); }} scrollToError={logErrorCode} onGenerateReport={() => { setLogsOpen(false); setActivePage('report'); }} showToast={showToast} />
      {checkerOpen && <ResourceChecker onClose={() => setCheckerOpen(false)} showToast={showToast} initialUrl={checkerInitialUrl} />}
      {healthOpen && <HealthCheckModal onClose={() => setHealthOpen(false)} />}
      <AutoSelectModal open={autoSelectOpen} onClose={() => setAutoSelectOpen(false)} showToast={showToast} autoStart={autoSelectAutoStart} />
      <DiagnosticsModal open={diagOpen} onClose={() => setDiagOpen(false)} showToast={showToast} />
      <UpdateAvailableModal
        data={updateModal}
        onLaunch={launchUpdater}
        onClose={() => setUpdateModal(null)}
      />
      <RecoveryToast
        open={recovery.open}
        steps={recovery.steps}
        done={recovery.done}
        success={recovery.success}
        onClose={() => setRecovery({ open: false, steps: [], done: false, success: false })}
      />
      {recoveryReport && (
        <RecoveryFailedModal
          report={recoveryReport}
          version={status?.mod?.version}
          onClose={() => setRecoveryReport(null)}
          onOpenReport={() => { setRecoveryReport(null); setActivePage('report'); }}
          showToast={showToast}
        />
      )}
      <Toast toasts={toasts} onRemove={removeToast} version={status?.mod?.version} onOpenLogs={handleOpenLogs} />
    </div>
    </ConfirmProvider>
  );
}
