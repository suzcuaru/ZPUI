import { useState, useEffect, useCallback, useRef } from 'react';
import Sidebar from './components/layout/Sidebar';
import Footer from './components/layout/Footer';
import TitleBar from './components/layout/TitleBar';
import Toast from './components/feedback/Toast';
import RecoveryModal from './components/feedback/RecoveryModal';
import RecoveryFailedModal from './components/feedback/RecoveryFailedModal';
import LogDrawer from './components/navigation/LogDrawer';
import ServerComponentsModal from './components/ui/ServerComponentsModal';
import AuthGate from './components/ui/AuthGate';
import BannerModal from './components/ui/BannerModal';
import OfflineScreen from './components/feedback/OfflineScreen';
import ResourceChecker from './components/ResourceChecker';
import HealthCheckModal from './components/HealthCheckModal';
import AutoSelectModal from './components/AutoSelectModal';
import StartupScreen from './components/StartupScreen';
import DiagnosticsModal from './components/DiagnosticsModal';
import UpdateAvailableModal from './components/UpdateAvailableModal';
import UpdateProgressModal from './components/UpdateProgressModal';
import { initUpdateEvents } from './hooks/useUpdateFlow';
import { useActionFeedback } from './hooks/useActionFeedback';
import { ConfirmProvider } from './components/ui/ConfirmDialog';
import DashboardPage from './pages/DashboardPage';
import ZapretPage from './pages/ZapretPage';
import SettingsPage from './pages/SettingsPage';
import MonitorPage from './pages/MonitorPage';
import ProxyPage from './pages/ProxyPage';
import XboxDnsPage from './pages/XboxDnsPage';
import DocsPage from './pages/DocsPage';
import ReportPage from './pages/ReportPage';
import UserPage from './pages/UserPage';
import { api, apiCall } from './api';
import { useT } from './i18n';
import { applyCustomTheme, removeCustomTheme } from './themeCustom';
import { usePolling } from './hooks/usePolling';
import { useUpdateCheck, checkZpuiUpdate, checkZapretUpdate, shouldNotifyZpui, shouldNotifyZapret, setZpuiCheck, setZapretCheck } from './hooks/useUpdateCheck';
import './styles/index.css';

// Стенд: модалку обновления открывает только чип в тайтлбаре, без авто-показа.
const standAutoUpdateModal = false;

const PAGES = {
  dashboard: DashboardPage,
  zapret: ZapretPage,
  proxy: ProxyPage,
  xboxdns: XboxDnsPage,
  monitor: MonitorPage,
  user: UserPage,
  settings: SettingsPage,
  report: ReportPage,
  docs: DocsPage,
};

// Клампинг подсказок: не выходить за левый/правый край окна. Сдвиг задаётся
// переменной --tt-ml (её читает CSS подсказки), ширина текста измеряется canvas.
let ttMeasureCtx = null;
function tooltipMarginLeft(el) {
  const text = el.getAttribute('data-tooltip') || '';
  if (!ttMeasureCtx) ttMeasureCtx = document.createElement('canvas').getContext('2d');
  // Измеряем тем же шрифтом, каким подсказку рисует CSS (::after), иначе клампинг врёт.
  const cs = getComputedStyle(el, '::after');
  ttMeasureCtx.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`;
  const textW = ttMeasureCtx.measureText(text).width;
  const tipW = Math.min(Math.max(textW, 40) + 26, Math.min(300, window.innerWidth * 0.84));
  const rect = el.getBoundingClientRect();
  const center = rect.left + rect.width / 2;
  const clamped = Math.max(tipW / 2 + 8, Math.min(window.innerWidth - tipW / 2 - 8, center));
  return Math.round(clamped - center);
}

export default function App() {
  const { t } = useT();
  const { zpuiCheck, zapretCheck } = useUpdateCheck();
  const [startupDone, setStartupDone] = useState(true); // стенд: без мастера первого запуска
  const [authState, setAuthState] = useState('loading');

  // События обновлений (update:download/install/open-modal) — один раз на сессию.
  useEffect(() => { initUpdateEvents(); }, []);

  // AuthGate: при старте проверяем, привязан ли аккаунт к серверу ZPUI.
  // «Продолжить без авторизации» хранится в sessionStorage — спрашиваем при каждом запуске.
  useEffect(() => {
    if (!startupDone) return;
    let cancelled = false;
    const skipped = (() => {
      try { return sessionStorage.getItem('zpui-auth-skip') === '1'; } catch { return false; }
    })();
    api('GET', '/api/server/status').then((s) => {
      if (cancelled) return;
      if (!s || s.error || typeof s.account_set !== 'boolean') {
        setAuthState('ok');
        return;
      }
      setAuthState(s.account_set || skipped ? (s.account_set ? 'ok' : 'skipped') : 'needed');
    });
    return () => { cancelled = true; };
  }, [startupDone]);


  // При показе окна (из трея / повторном открытии) — всегда главная страница.
  useEffect(() => {
    const rt = window.runtime;
    if (!rt?.EventsOn) return;
    const onShown = () => setActivePage('dashboard');
    rt.EventsOn('window:shown', onShown);
    return () => {
      try { rt.EventsOff('window:shown'); } catch {}
    };
  }, []);
  const [activePage, setActivePage] = useState('dashboard');
  const [status, setStatus] = useState(null);
  const [winMax, setWinMax] = useState(true);
  const [toasts, setToasts] = useState([]);
  const [logsOpen, setLogsOpen] = useState(false);
  const [logErrorCode, setLogErrorCode] = useState(null);
  const [checkerOpen, setCheckerOpen] = useState(false);
  const [checkerInitialUrl, setCheckerInitialUrl] = useState('');
  const [autoSelectOpen, setAutoSelectOpen] = useState(false);
  const [backendOnline, setBackendOnline] = useState(true);
  const [theme, setTheme] = useState('dark');
  const [themeMode, setThemeMode] = useState('system');
  const [customBase, setCustomBase] = useState('dark');
  const [healthOpen, setHealthOpen] = useState(false);
  const [healthWarn, setHealthWarn] = useState(null);
  const [diagOpen, setDiagOpen] = useState(false);
  const [srvComp, setSrvComp] = useState(null); // {items} — модалка обновлений компонентов с сервера
  const [zapretLoading, setZapretLoading] = useState(false);
  const [recovery, setRecovery] = useState({ open: false, steps: [], phase: 'progress' });
  const [recoveryReport, setRecoveryReport] = useState(null);
  const [autoSelectAutoStart, setAutoSelectAutoStart] = useState(false);
  // Экономный режим: настройки из конфига + работа от батареи (для автоматики).
  // Источник истины — /api/status от бэкенда (он переживает переоткрытие окна);
  // локальные настройки нужны лишь до первого опроса статуса.
  const [powerCfg, setPowerCfg] = useState({ enabled: false, level: 2, threshold: 0 });
  const [onBattery, setOnBattery] = useState(false);
  const powerSaving = status?.power_saving === true
    || (!status && powerCfg.enabled
      && (powerCfg.threshold === 0 || (powerCfg.threshold === -1 && onBattery)));
  const [autoUpdateCheck, setAutoUpdateCheck] = useState(true);
  const [updateModal, setUpdateModal] = useState(null);
  const failCountRef = useRef(0);
  const themeInitRef = useRef(false);

  const showToast = useCallback((msg, type, opts) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, msg, type, ...(opts || {}) }]);
  }, []);

  const { run: runClose } = useActionFeedback(showToast);

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
  // Асинхронный старт Запрета: опрашиваем статус до живого состояния.
  // Если бэкенд запустит восстановление — событие zapret:recovery:start
  // перехватит управление спиннером и открытием окна.
  const pollZapretStart = useCallback(async () => {
    const started = Date.now();
    const poll = async () => {
      if (Date.now() - started > 30000) return;
      const st = await api('GET', '/api/status');
      if (st?.zapret?.status === 'running') {
        showToast(t('header.zapretStarted'), 'success');
        setZapretLoading(false);
        return;
      }
      setTimeout(poll, 700);
    };
    poll();
  }, [showToast, t]);

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
        if (result?.status === 'starting') {
          // Запуск идёт асинхронно: держим спиннер до живого статуса;
          // если движок не оживёт — бэкенд сам откроет окно восстановления.
          pollZapretStart();
          return;
        }
        if (result?.error) {
          showToast(result.error, 'error');
          setZapretLoading(false);
          return;
        }
      }
      await apiCall(() => api('POST', '/api/component-states'));
    } catch {
      if (showToast) showToast(t('common.error'), 'error');
      setZapretLoading(false);
    }
  }, [status, zapretLoading, showToast, t, pollZapretStart]);

  // Единая точка сохранения темы в конфиг бэкенда.
  const saveThemeConfig = useCallback((themeValue) => {
    api('POST', '/api/config', { theme: themeValue });
  }, []);

  // Единственный применитель темы: тайтлбар вызывает напрямую, карточки
  // настроек присылают событие zpui:theme-changed — оба контрола синхронны.
  // Обычные темы обязаны полностью снимать кастомные переопределения (style-тег).
  const applyTheme = useCallback((themeValue) => {
    if (themeValue === 'custom') return;
    removeCustomTheme();
    const apply = (resolved) => {
      setTheme(resolved);
      document.documentElement.setAttribute('data-theme', resolved);
    };
    if (themeValue === 'system') {
      try { localStorage.removeItem('zpui-theme'); } catch {}
      setThemeMode('system');
      api('GET', '/api/system-theme').then(sys => {
        if (sys) apply(sys === 'dark' ? 'dark' : 'light');
      });
    } else {
      try { localStorage.setItem('zpui-theme', themeValue); } catch {}
      setThemeMode('manual');
      apply(themeValue === 'dark' ? 'dark' : 'light');
    }
    saveThemeConfig(themeValue);
  }, [saveThemeConfig]);

  // Кастомная тема: style-тег с :root[data-theme="custom"] + режим 'custom'
  // в конфиге. База запоминается для иконки/переключателя тайтлбара.
  const activateCustomTheme = useCallback((themeData) => {
    if (!themeData || typeof themeData !== 'object') return;
    applyCustomTheme(themeData);
    setCustomBase(themeData.base === 'light' ? 'light' : 'dark');
    try { localStorage.setItem('zpui-theme', 'custom'); } catch {}
    setThemeMode('manual');
    setTheme('custom');
    document.documentElement.setAttribute('data-theme', 'custom');
    saveThemeConfig('custom');
  }, [saveThemeConfig]);

  const toggleTheme = useCallback(() => {
    // Активная кастомная тема: переключатель тайтлбара возвращает на её базу.
    if (theme === 'custom') { applyTheme(customBase); return; }
    applyTheme(theme === 'dark' ? 'light' : 'dark');
  }, [theme, customBase, applyTheme]);

  useEffect(() => {
    const onThemeChanged = (e) => {
      const v = e.detail?.theme;
      if (v === 'custom') {
        api('GET', '/api/custom-theme').then(r => {
          if (r && r.theme) activateCustomTheme(r.theme);
        });
      } else if (v === 'system' || v === 'dark' || v === 'light') applyTheme(v);
    };
    window.addEventListener('zpui:theme-changed', onThemeChanged);
    return () => window.removeEventListener('zpui:theme-changed', onThemeChanged);
  }, [applyTheme, activateCustomTheme]);

  // Theme init — run once after startup
  useEffect(() => {
    if (!startupDone || themeInitRef.current || !status?.mod) return;
    themeInitRef.current = true;
    const savedTheme = status.mod.theme || 'system';
    if (savedTheme === 'custom') {
      // В конфиге кастомная тема: применяем сохранённую, без неё — системную.
      api('GET', '/api/custom-theme').then(r => {
        if (r && r.theme) activateCustomTheme(r.theme);
        else applyTheme('system');
      });
    } else if (savedTheme === 'system') {
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
  }, [startupDone, status, applyTheme, activateCustomTheme]);

  // Status polling (only after startup)
  const pollStatus = async () => {
    if (!startupDone) return;
    // Окно скрыто (свернуто/в трее) — опрос приостанавливаем: WebView2
    // не гарантирует троттлинг таймеров, а каждый запрос будит CPU.
    if (document.hidden) return;
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
  usePolling(pollStatus, startupDone ? (powerSaving ? 5000 : 2000) : 0);

  // Настройки экономного режима: из конфига (при старте и при изменении).
  useEffect(() => {
    if (!startupDone) return;
    const load = () => api('GET', '/api/config').then(c => {
      if (c) setPowerCfg({
        enabled: c.power_saving === true,
        level: c.power_level || 2,
        threshold: c.power_threshold || 0,
      });
    });
    load();
    window.addEventListener('zpui:config-changed', load);
    return () => window.removeEventListener('zpui:config-changed', load);
  }, [startupDone]);

  // Батарея: navigator.getBattery доступен в WebView2 (Chromium). Нужен
  // только для мгновенной реакции до первого /api/status при режимах
  // «от АКБ» и «процент порога».
  useEffect(() => {
    if (!powerCfg.enabled || powerCfg.threshold === 0 || !navigator.getBattery) return;
    let bat = null;
    const update = () => setOnBattery(bat ? bat.charging === false : false);
    navigator.getBattery().then(b => {
      bat = b;
      update();
      b.addEventListener('chargingchange', update);
    }).catch(() => {});
    return () => { if (bat) bat.removeEventListener('chargingchange', update); };
  }, [powerCfg.enabled, powerCfg.threshold]);

  // Экономный режим глушит декоративные CSS-анимации (пульсы, shimmer).
  useEffect(() => {
    document.body.classList.toggle('power-save', powerSaving);
  }, [powerSaving]);

  // Health check polling
  const pollHealth = async () => {
    if (!startupDone) return;
    const h = await api('GET', '/api/health');
    setHealthWarn(h && h.overall && h.overall !== 'healthy' ? h : null);
  };
  usePolling(pollHealth, startupDone ? 30000 : 0);

  // Проверка обновлений управляется единым тумблером «проверка обновлений».
  useEffect(() => {
    if (!startupDone) return;
    api('GET', '/api/config').then(c => {
      if (c) setAutoUpdateCheck(c.auto_update_check !== false);
    });
  }, [startupDone]);

  // Подсказки не должны выезжать за края окна: при наведении считаем сдвиг.
  // Клампинг --tt-ml нужен только подсказкам «сверху по центру»; у боковых
  // (pos="right") он уводил подсказку далеко вправо от кнопки.
  useEffect(() => {
    const onOver = (e) => {
      const el = e.target.closest ? e.target.closest('[data-tooltip]') : null;
      if (!el) return;
      const pos = el.getAttribute('data-tooltip-pos');
      // Клампинг --tt-ml нужен только центрированным подсказкам; «right» и
      // «top-left» позиционируются от края элемента и сдвиг не применяют.
      if (pos === 'right' || pos === 'top-left') return;
      el.style.setProperty('--tt-ml', tooltipMarginLeft(el) + 'px');
    };
    document.addEventListener('pointerover', onOver, true);
    return () => document.removeEventListener('pointerover', onOver, true);
  }, []);

  // Update checks: только при запуске приложения и только если включён
  // тумблер «проверка обновлений». Ретрай нужен: первый запуск гоняется
  // со сборкой кэша версий на бэкенде. Никаких периодических проверок.
  useEffect(() => {
    if (!startupDone || !autoUpdateCheck) return;
    const runChecks = () => { checkZpuiUpdate(); checkZapretUpdate(); };
    const initId = setTimeout(runChecks, 2000);
    const retryId = setTimeout(runChecks, 15000);
    return () => { clearTimeout(initId); clearTimeout(retryId); };
  }, [startupDone, autoUpdateCheck]);

  // Модалка «доступно обновление» при обнаружении (дедуп по версии за сессию).
  // На стенде модалку не навязываем — её открывает чип обновления в тайтлбаре.
  useEffect(() => {
    if (standAutoUpdateModal && zpuiCheck.state === 'available' && zpuiCheck.latest && shouldNotifyZpui(zpuiCheck.latest)) {
      setUpdateModal({ component: 'ZPUI', current: zpuiCheck.current, latest: zpuiCheck.latest });
    }
  }, [zpuiCheck.state, zpuiCheck.latest]);

  useEffect(() => {
    if (standAutoUpdateModal && zapretCheck.state === 'available' && zapretCheck.latest && shouldNotifyZapret(zapretCheck.latest)) {
      setUpdateModal({ component: 'zapret', current: zapretCheck.current, latest: zapretCheck.latest });
    }
  }, [zapretCheck.state, zapretCheck.latest]);

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

  // Recovery procedure events — модальное окно с этапами по центру.
  // Провал: первый раз — чистый перезапуск приложения; если и после перезапуска
  // восстановление не удалось — финальная ошибка с просьбой обратиться в поддержку.
  useEffect(() => {
    if (!startupDone) return;
    let hideTimer = null;
    const MARKER = 'zpui-recovery-restart';
    const MARKER_TTL = 10 * 60 * 1000;
    const markerAge = () => {
      const ts = Number(localStorage.getItem(MARKER) || 0);
      return ts > 0 ? Date.now() - ts : -1;
    };
    if (markerAge() > MARKER_TTL) localStorage.removeItem(MARKER);
    const upsertStep = (step) => {
      setRecovery((prev) => {
        const steps = [...prev.steps];
        const idx = steps.findIndex((s) => s.key === step.key);
        if (idx >= 0) steps[idx] = step; else steps.push(step);
        return { ...prev, steps };
      });
    };
    const closeRecovery = () => setRecovery({ open: false, steps: [], phase: 'progress' });
    const onStart = () => {
      setZapretLoading(true);
      setRecovery({ open: true, steps: [], phase: 'progress' });
    };
    const onStep = (step) => { if (step) upsertStep(step); };
    const onDone = async (data) => {
      const success = !!(data && data.success);
      // Пользователь начал деструктивное действие (стоп/перестановка) —
      // просто закрываем окно восстановления без ошибок и перезапусков.
      if (data && data.cancelled) {
        closeRecovery();
        setZapretLoading(false);
        return;
      }
      const report = data && data.report;
      setZapretLoading(false);
      if (success) {
        localStorage.removeItem(MARKER);
        setRecovery((prev) => ({ ...prev, phase: 'success' }));
        if (hideTimer) clearTimeout(hideTimer);
        hideTimer = setTimeout(closeRecovery, 2500);
        return;
      }
      const retried = markerAge() >= 0 && markerAge() <= MARKER_TTL;
      if (retried) {
        // Второй провал подряд (после перезапуска) — финальная ошибка.
        localStorage.removeItem(MARKER);
        closeRecovery();
        setRecoveryReport({ ...(report || {}), error_code: report?.error_code || data?.error_code, reason: report?.reason || data?.reason, second_failure: true });
        return;
      }
      localStorage.setItem(MARKER, String(Date.now()));
      setRecovery((prev) => ({ ...prev, phase: 'restarting' }));
      const r = await api('POST', '/api/app/restart-clean');
      if (r?.error) {
        // Перезапустить не вышло — сразу показываем финальную ошибку.
        localStorage.removeItem(MARKER);
        closeRecovery();
        setRecoveryReport({ ...(report || {}), error_code: report?.error_code || data?.error_code, reason: report?.reason || data?.reason, second_failure: true });
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

  useEffect(() => {
    if (!backendOnline) return;
    const check = async () => {
      const r = await api('GET', '/api/server/components');
      if (r && !r.error && r.has_update) setSrvComp({ items: r.items || [] });
    };
    const id = setTimeout(check, 15000);
    return () => clearTimeout(id);
  }, [backendOnline]);

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

  // Кроссфейд вкладок: старая страница остаётся видимой (статичный кадр)
  // и гаснет поверх, пока новая проявляется — без кадра пустоты.
  const [leavingPage, setLeavingPage] = useState(null);
  const navigate = useCallback((p) => {
    setActivePage(prev => {
      if (p !== prev) setLeavingPage(prev);
      return p;
    });
  }, []);
  useEffect(() => {
    if (!leavingPage) return;
    const id = setTimeout(() => setLeavingPage(null), 300);
    return () => clearTimeout(id);
  }, [leavingPage]);

  const pageProps = { status, showToast, showProgressToast, updateProgressToast, completeProgressToast, onNavigate: navigate, onOpenLogs: () => setLogsOpen(true), onOpenDiagnostics: () => setDiagOpen(true), onOpenCheckerWithUrl: (url) => { setCheckerInitialUrl(url); setCheckerOpen(true); }, zapretLoading, onZapretToggle };

  const pageContent = backendOnline ? (
    <>
      {leavingPage && PAGES[leavingPage] ? (() => {
        const LeaveComp = PAGES[leavingPage];
        return (
          <div className="main-area page-leave" key={'leave-' + leavingPage}>
            <LeaveComp {...pageProps} />
          </div>
        );
      })() : null}
      <div className="main-area page-fade" key={activePage}>
        {PageComponent ? (
         <PageComponent {...pageProps} />
       ) : null}
      </div>
    </>
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
    {authState === 'needed' ? (
      <AuthGate
        onAuthed={() => setAuthState('ok')}
        onSkip={() => setAuthState('skipped')}
      />
    ) : (
    <div className="app stand-app">
      <div className={'stand-win' + (winMax ? '' : ' stand-win-restore')}>
        <TitleBar
          version={status?.mod?.version}
          arch="x64"
          theme={theme === 'custom' ? customBase : theme}
          updateInfo={
            zpuiCheck.state === 'available' && zpuiCheck.latest ? { component: 'ZPUI', current: zpuiCheck.current, latest: zpuiCheck.latest }
            : zapretCheck.state === 'available' && zapretCheck.latest ? { component: 'zapret', current: zapretCheck.current, latest: zapretCheck.latest }
            : null
          }
          onOpenUpdate={() => {
            const src = zpuiCheck.state === 'available' ? zpuiCheck : zapretCheck;
            setUpdateModal({ component: src === zpuiCheck ? 'ZPUI' : 'zapret', current: src.current, latest: src.latest });
          }}
          onToggleTheme={toggleTheme}
          maximized={winMax}
          onToggleMaximize={() => setWinMax(v => !v)}
          onClose={() => runClose(() => api('POST', '/api/app/close'), { successMsg: t('stand.closeToast'), errorMsg: t('stand.closeFailed') })}
        />
        <div className="stand-frame">
        <Sidebar
          activePage={activePage}
          onNavigate={navigate}
          onOpenChecker={() => setCheckerOpen(true)}
          onAutoSelect={() => { setAutoSelectAutoStart(true); setAutoSelectOpen(true); }}
          onOpenHealth={() => setHealthOpen(true)}
          onOpenHelp={() => setActivePage('docs')}
          healthWarn={healthWarn}
          status={status}
          showToast={showToast}
          onOpenLogs={() => setLogsOpen(true)}
          powerSaving={powerSaving}
          powerLevel={status?.power_saving_level || powerCfg.level}
        />
        <div className="stand-main">
          <div className="app-body stand-content">
            {pageContent}
            <ServerComponentsModal open={!!srvComp} items={srvComp?.items || []} onClose={() => setSrvComp(null)} />
            <BannerModal />
          </div>
          <Footer status={status} />
        </div>
        </div>
      <LogDrawer open={logsOpen} onClose={() => { setLogsOpen(false); setLogErrorCode(null); }} scrollToError={logErrorCode} onGenerateReport={() => { setLogsOpen(false); setActivePage('report'); }} showToast={showToast} />
      {checkerOpen && <ResourceChecker onClose={() => setCheckerOpen(false)} showToast={showToast} initialUrl={checkerInitialUrl} />}
      {healthOpen && <HealthCheckModal onClose={() => setHealthOpen(false)} />}
      <AutoSelectModal open={autoSelectOpen} onClose={() => setAutoSelectOpen(false)} showToast={showToast} autoStart={autoSelectAutoStart} />
      <DiagnosticsModal open={diagOpen} onClose={() => setDiagOpen(false)} showToast={showToast} />
      <UpdateAvailableModal
        data={updateModal}
        onClose={() => setUpdateModal(null)}
      />
      <UpdateProgressModal />
      <RecoveryModal
        open={recovery.open}
        phase={recovery.phase}
        steps={recovery.steps}
      />
      {recoveryReport && (
        <RecoveryFailedModal
          report={recoveryReport}
          secondFailure={!!recoveryReport.second_failure}
          version={status?.mod?.version}
          onClose={() => setRecoveryReport(null)}
          onOpenReport={() => { setRecoveryReport(null); setActivePage('report'); }}
          showToast={showToast}
        />
      )}
      <Toast toasts={toasts} onRemove={removeToast} version={status?.mod?.version} onOpenLogs={handleOpenLogs} />
      </div>
    </div>
    )}
    </ConfirmProvider>
  );
}
