import { useState, useEffect, useCallback, useMemo, useRef, memo } from 'react';
import Sidebar from './components/layout/Sidebar';
import Footer from './components/layout/Footer';
import TitleBar from './components/layout/TitleBar';
import Toast from './components/feedback/Toast';
import RecoveryModal from './components/feedback/RecoveryModal';
import RecoveryFailedModal from './components/feedback/RecoveryFailedModal';
import LogDrawer from './components/navigation/LogDrawer';
import OfflineScreen from './components/feedback/OfflineScreen';
import ProfileDrawer from './components/navigation/ProfileDrawer';
import AuthSuggestModal from './components/feedback/AuthSuggestModal';
import SyncChoiceModal from './components/feedback/SyncChoiceModal';
import PromoRevealModal from './components/feedback/PromoRevealModal';
import StarPromoModal, { starPromoDue } from './components/StarPromoModal';
import EngineFailModal from './components/EngineFailModal';
import BugReportModal from './components/feedback/BugReportModal';
import SelectionStaleModal from './components/feedback/SelectionStaleModal';
import TooltipLayer from './components/ui/TooltipLayer';
import ResourceChecker from './components/ResourceChecker';
import HealthCheckModal from './components/HealthCheckModal';
import AutoSelectModal from './components/AutoSelectModal';
import StartupScreen from './components/StartupScreen';
import DiagnosticsModal from './components/DiagnosticsModal';
import CacheModal from './components/CacheModal';
import McpModal from './components/McpModal';
import ExtensionModal from './components/ExtensionModal';
import UpdateAvailableModal from './components/UpdateAvailableModal';
import UpdateProgressModal from './components/UpdateProgressModal';
import { initUpdateEvents } from './hooks/useUpdateFlow';
import { useActionFeedback } from './hooks/useActionFeedback';
import { useRecoveryEvents } from './hooks/useRecoveryEvents';
import { ConfirmProvider } from './components/ui/ConfirmDialog';
import DashboardPage from './pages/DashboardPage';
import ZapretPage from './pages/ZapretPage';
import SettingsPage from './pages/SettingsPage';
import MonitorPage from './pages/MonitorPage';
import ProxyPage from './pages/ProxyPage';
import DnsPage from './pages/DnsPage';
import DocsPage from './pages/DocsPage';
import { api, apiCall, apiWarm } from './api';
import { useT } from './i18n';
import { subscribeEvent } from './events';
import { useWailsEvent } from './hooks/useWailsEvent';
import { ToastProvider } from './context/ToastContext';
import { frontlog, logUiError } from './frontlog';
import { applyCustomTheme, removeCustomTheme, resolveThemeBgUrl, resyncCustomThemeMark } from './themeCustom';
import { sound } from './sound';
import { useEngineFail } from './engineFail';
import { isNetError } from './utils';
import { usePolling } from './hooks/usePolling';
import { PageVisibleProvider } from './hooks/usePageVisible';
import { setWindowShown, isInteractive, onWindowShownChange } from './winVisible';
import { setPowerSaving } from './powerFlag';
import { useUpdateCheck, checkZpuiUpdate, checkZapretUpdate, shouldNotifyZpui, shouldNotifyZapret, setZpuiCheck, setZapretCheck } from './hooks/useUpdateCheck';
import './styles/index.css';

// Стенд: модалку обновления открывает только чип в тайтлбаре, без авто-показа.
const standAutoUpdateModal = false;

const PAGES = {
  dashboard: DashboardPage,
  zapret: ZapretPage,
  proxy: ProxyPage,
  xboxdns: DnsPage, // алиас старых глубоких ссылок — вкладка теперь чистый DNS
  network: DnsPage,
  monitor: MonitorPage,
  settings: SettingsPage,
  docs: DocsPage,
};

// Клампинг подсказок: не выходить за левый/правый край ОКНА приложения
// (.stand-win с overflow:hidden), а не вьюпорта — в восстановленном режиме
// и для контента модалок вьюпорт шире окна, старый клампинг оставлял вылеты.
// Сдвиг задаётся переменной --tt-ml (её читает CSS подсказки), ширина текста
// измеряется canvas.
// Клампинг подсказок теперь внутри TooltipLayer (portal в body): компонент
// сам измеряет текст и флипает/клампит у краёв окна — здесь логики нет.

// Стабильные обёртки: компонент перерисовывается только при изменении СВОИХ
// пропсов, а не любого стейта App. Тик статуса (2с), тосты, открытие модалок
// и смена темы больше не гоняют перерендер всего дерева — при условии, что
// пропсы стабильны (useCallback/useMemo, никаких инлайн-стрелок в JSX).
const MTitleBar = memo(TitleBar);
const MSidebar = memo(Sidebar);
const MFooter = memo(Footer);
const MToast = memo(Toast);
const MTooltipLayer = memo(TooltipLayer);
const MLogDrawer = memo(LogDrawer);
const MProfileDrawer = memo(ProfileDrawer);
const MAuthSuggestModal = memo(AuthSuggestModal);
const MSyncChoiceModal = memo(SyncChoiceModal);
const MPromoRevealModal = memo(PromoRevealModal);
const MSelectionStaleModal = memo(SelectionStaleModal);
const MResourceChecker = memo(ResourceChecker);
const MHealthCheckModal = memo(HealthCheckModal);
const MAutoSelectModal = memo(AutoSelectModal);
const MDiagnosticsModal = memo(DiagnosticsModal);
const MCacheModal = memo(CacheModal);
const MMcpModal = memo(McpModal);
const MExtensionModal = memo(ExtensionModal);
const MUpdateAvailableModal = memo(UpdateAvailableModal);
const MUpdateProgressModal = memo(UpdateProgressModal);
const MRecoveryModal = memo(RecoveryModal);
const MRecoveryFailedModal = memo(RecoveryFailedModal);
const MEMO_PAGES = Object.fromEntries(
  Object.entries(PAGES).map(([k, C]) => [k, memo(C)]),
);

export default function App() {
  const { t } = useT();
  const { zpuiCheck, zapretCheck } = useUpdateCheck();
  // Стартовый экран возвращён: мок проходит все шаги (служба/локальная копия
  // установлены, автоподбор есть — перебор не запускается).
  const [startupDone, setStartupDone] = useState(false);
  // «Движок запрета не запустился» (ставит стартовый экран): гард авто-модалки
  // подбора — пользователь занят модалкой ошибки.
  const engineFail = useEngineFail();
  // Авторизация = иконка профиля в тайтлбаре. Состояния:
  // 'ok' — токен валиден (зелёная), 'offline' — токен есть, сервер недоступен
  // (оранжевая), 'none' — не авторизован (красная). Без блокирующего гейта:
  // локальные функции работают всегда.
  const [authState, setAuthState] = useState('none');
  const [authEnabled, setAuthEnabled] = useState(false);
  // Снимок account/me: подписка, план, устройства.
  const [meInfo, setMeInfo] = useState(null);
  // Боковая панель профиля (иконка в тайтлбаре): вход/аккаунт/подписка.
  const [profileOpen, setProfileOpen] = useState(false);
  // «Сервер ZPUI Live недоступен»: анти-ложняки — офлайн только если
  // НЕУСПЕШНЫ ОБА зонда (server/ping И account/me) 3 подряд с шагом ~5с.
  // HTTP-ошибки (401/500) офлайном не считаются — только сетевые сбои
  // (isNetError). Восстановление → скрыть мгновенно. Повторный показ —
  // не чаще раза в 60с.
  const [srvOffline, setSrvOffline] = useState(false);
  const srvHiddenRef = useRef(false);
  const srvLastShowRef = useRef(0);
  const srvBusyRef = useRef(false);

  // События обновлений (update:download/install/open-modal) — один раз на сессию.
  useEffect(() => { initUpdateEvents(); }, []);

  // Глобальные сбои окна (необработанные исключения и reject'ы) — во
  // фронтенд-лог: невидимые ошибки видны владельцу в журнале Диагностики.
  useEffect(() => {
    const onError = (e) => {
      frontlog('window.error', {
        message: String((e && e.message) || e || 'unknown'),
        source: e && e.filename ? `${e.filename}:${e.lineno || 0}` : '',
      });
    };
    const onRejection = (e) => {
      const reason = e && e.reason;
      frontlog('window.unhandledrejection', {
        message: String((reason && (reason.stack || reason.message)) || reason || 'unknown'),
      });
    };
    window.addEventListener('error', onError);
    window.addEventListener('unhandledrejection', onRejection);
    return () => {
      window.removeEventListener('error', onError);
      window.removeEventListener('unhandledrejection', onRejection);
    };
  }, []);

  // Авторизация при старте и по расписанию: /api/server/status даёт
  // account_set (есть ли токен) и available (жив ли сервер), /api/account/me
  // валидирует токен. Гейта нет: локальные функции работают всегда, состояние
  // видно по окантовке иконки профиля.
  const refreshAuth = useCallback(async () => {
    if (!isInteractive()) return;
    const [srv, me] = await Promise.all([
      api('GET', '/api/server/status'),
      api('GET', '/api/account/me'),
    ]);
    const tokenSet = !!srv && srv.account_set === true;
    setAuthEnabled(tokenSet || !!(me && !isNetError(me) && !me.error));
    if (me && !me.error) {
      setMeInfo(me);
      setAuthState('ok');
      return;
    }
    setMeInfo(null);
    if (isNetError(me)) {
      setAuthState(tokenSet ? 'offline' : 'none');
      return;
    }
    setAuthState('none');
  }, []);

  useEffect(() => {
    if (!startupDone) return;
    api('GET', '/api/config').then((c) => {
      setAuthEnabled(c ? c.authorization_enabled === true : false);
    });
    refreshAuth();
  }, [startupDone, refreshAuth]);

  usePolling(refreshAuth, 30000);

  // Предложение авторизоваться: один раз за запуск, только когда авторизация
  // включена в config.json (authorization_enabled), учётной записи нет и
  // пользователь не нажимал «Больше не показывать»
  // (server.auth_prompt_dismissed).
  const authSuggestShownRef = useRef(false);
  const [authSuggest, setAuthSuggest] = useState(false);
  // Синхронизация устройств (перенос после входа) и промо-карточка регистрации.
  const [syncChoiceOpen, setSyncChoiceOpen] = useState(false);
  const [regPromoOpen, setRegPromoOpen] = useState(false);
  const [regPromo, setRegPromo] = useState(null);
  // Зеркало profileOpen для колбэка вне эффекта: предложение не должно
  // всплывать, если панель профиля уже открыта (таймер может сработать позже).
  const profileOpenRef = useRef(false);
  useEffect(() => {
    if (!startupDone || authSuggestShownRef.current) return;
    if (authState !== 'none') return;
    authSuggestShownRef.current = true;
    let alive = true;
    Promise.all([
      api('GET', '/api/config'),
      api('GET', '/api/server/status'),
    ]).then(([c, s]) => {
      if (!alive) return;
      const enabled = !!(c && c.authorization_enabled === true);
      const dismissed = !!(s && !s.error && s.auth_prompt_dismissed === true);
      if (enabled && !dismissed) {
        setTimeout(() => { if (alive && !profileOpenRef.current) setAuthSuggest(true); }, 1200);
      }
    }).catch(() => {});
    return () => { alive = false; };
  }, [startupDone, authState]);

  // Просьба о звезде на GitHub: не чаще раза в неделю, после открытия окна.
  const [starPromo, setStarPromo] = useState(false);
  useEffect(() => {
    if (!startupDone) return;
    const tm = setTimeout(() => { if (starPromoDue()) setStarPromo(true); }, 2500);
    return () => clearTimeout(tm);
  }, [startupDone]);

  // Панель профиля открыли — предложение больше не нужно (в т.ч. если юзер
  // успел зайти в профиль раньше, чем всплыло предложение).
  useEffect(() => {
    profileOpenRef.current = profileOpen;
    if (profileOpen) setAuthSuggest(false);
  }, [profileOpen]);

  // Ручное обновление meInfo (после авторизации/выхода).
  const refreshMe = useCallback(async () => {
    const m = await api('GET', '/api/account/me');
    if (m && !m.error) { setMeInfo(m); setAuthState('ok'); }
    else { setMeInfo(null); setAuthState('none'); }
    return m;
  }, []);

  // Подписка/устройства в поповере профиля должны стареть: истечение срока,
  // покупка на другом устройстве. Досматриваем /api/account/me раз в минуту
  // (скрытое окно — пауза встроена в usePolling).
  usePolling(refreshMe, 60000, [], startupDone && authEnabled);

  // Оба зонда: офлайн засчитывается только когда неуспешны ОБА (isNetError:
  // упавший биндинг/таймаут или явный сетевой код; HTTP-коды — не офлайн).
  const probeServer = useCallback(async () => {
    const [p, m] = await Promise.all([
      api('GET', '/api/server/ping'),
      api('GET', '/api/account/me'),
    ]);
    return isNetError(p) && isNetError(m);
  }, []);

  // Быстрый ретрай из модалки: одиночный проход обоих зондов, при успехе —
  // мгновенное скрытие. Возвращает true, если связь восстановлена.
  const retryServer = useCallback(async () => {
    if (await probeServer()) {
      srvHiddenRef.current = false;
      setSrvOffline(false);
      return true;
    }
    return false;
  }, [probeServer]);

  const [winMax, setWinMax] = useState(true);

  // Состояние кнопки «развернуть» синхронизируется с реальным окном Wails,
  // иначе UI и ОС расходятся после сворачивания в трей/восстановления.
  useWailsEvent('window:maximised', () => setWinMax(true));
  useWailsEvent('window:unmaximised', () => setWinMax(false));

  useEffect(() => {
    api('POST', '/api/app/is-maximised').then((r) => {
      if (r && typeof r.maximised === 'boolean') setWinMax(r.maximised);
    }).catch(() => {});
  }, []);


  const [activePage, setActivePage] = useState('dashboard');
  // Keep-mounted вкладки (спека 2026-11-03-render-responsiveness, R5/R6):
  // посещённая страница монтируется при первом визите и остаётся смонтированной
  // (скрытые — display:none, опросы и тики гейтит PageVisibleProvider в хуках).
  // Данные при возврате мгновенны (SWR + живое состояние), пересборки DOM нет.
  // Переход между вкладками мгновенный, без анимаций: кроссфейд мерцал
  // (фидбек 03.11), fade-in читался как «пропадает → появляется» (фидбек 04.11).
  const [visitedPages, setVisitedPages] = useState(['dashboard']);
  const activePageRef = useRef('dashboard');
  const navigate = useCallback((p) => {
    if (p === activePageRef.current) return;
    activePageRef.current = p;
    setVisitedPages((list) => (list.includes(p) ? list : [...list, p]));
    setActivePage(p);
  }, []);

  // При показе окна (из трея / повторном открытии) — всегда главная страница.
  // Скрытие (трей/close-to-tray/--hidden) глушит тики и анимации: невидимое
  // окно не должно рисовать кадры и грузить GPU (winVisible.js).
  useWailsEvent('window:shown', () => { setWindowShown(true); navigate('dashboard'); });
  useWailsEvent('window:hidden', () => setWindowShown(false));

  // Фактическое состояние окна у бэка: событие window:shown могло прийти до
  // подписки или быть пропущено — переспрашиваем и правим гейт.
  const probeWindowShown = useCallback(() => {
    api('POST', '/api/app/is-shown').then((r) => {
      if (r && typeof r.shown === 'boolean') setWindowShown(r.shown);
    }).catch(() => {});
  }, []);

  useEffect(() => {
    // Старт с окном в трее (--hidden): событие window:hidden могло прийти до
    // подписки — спрашиваем фактическое состояние окна у бэка.
    probeWindowShown();
  }, [probeWindowShown]);
  const [status, setStatus] = useState(null);
  const [toasts, setToasts] = useState([]);

  const [logsOpen, setLogsOpen] = useState(false);
  const [logErrorCode, setLogErrorCode] = useState(null);
  const [checkerOpen, setCheckerOpen] = useState(false);
  const [checkerInitialUrl, setCheckerInitialUrl] = useState('');
  const [autoSelectOpen, setAutoSelectOpen] = useState(false);
  // Автостарт умного подбора для авто-открытой модалки (чистая база, первый
  // запуск): выставляется один раз в handleStartupComplete, сбрасывается при
  // закрытии — ручное открытие всегда показывает интро без запуска.
  const [autoSelectAutoStart, setAutoSelectAutoStart] = useState(false);
  // «Пора пройти подбор» (selection_last_at старше 7 дней): показывается
  // один раз за сессию — см. эффект ниже.
  const [selectionStaleOpen, setSelectionStaleOpen] = useState(false);
  const [backendOnline, setBackendOnline] = useState(true);
  // Причина офлайна: 'bridge' — биндинги Wails недоступны (фронт отсоединён
  // от бэка: обычно запущена старая копия приложения), 'api' — бэкенд-процесс
  // не отвечает на /api/status.
  const [offlineReason, setOfflineReason] = useState('');
  const [theme, setTheme] = useState('dark');
  const [themeMode, setThemeMode] = useState('system');
  // Сырое значение из конфига: system/light/dark/custom — позиция в цикле тайтлбара.
  const [themeSetting, setThemeSetting] = useState('system');
  const [customBase, setCustomBase] = useState('dark');
  const [healthOpen, setHealthOpen] = useState(false);
  const [healthWarn, setHealthWarn] = useState(null);
  const [diagOpen, setDiagOpen] = useState(false);
  const [cacheOpen, setCacheOpen] = useState(false);
  const [mcpOpen, setMcpOpen] = useState(false);
  const [extensionOpen, setExtensionOpen] = useState(false);
  const [zapretLoading, setZapretLoading] = useState(false);
  const [recovery, setRecovery] = useState({ open: false, steps: [], phase: 'progress' });
  const [recoveryReport, setRecoveryReport] = useState(null);
  // Баг-репорт «как в 1С»: из тоста ошибки — модалка с кодом/текстом ошибки
  // и отправкой JSON-диагностики на сервер.
  const [bugReport, setBugReport] = useState({ open: false, code: '', error: '' });
  const openBugReport = useCallback((payload) => {
    setBugReport({ open: true, code: payload?.code || '', error: payload?.error || '' });
  }, []);
  // Экономный режим: настройки из конфига + работа от батареи (для автоматики).
  // Источник истины — /api/status от бэкенда (он переживает переоткрытие окна);
  // локальные настройки нужны лишь до первого опроса статуса.
  const [powerCfg, setPowerCfg] = useState({ enabled: false, level: 2, threshold: 0 });
  const [onBattery, setOnBattery] = useState(false);
  const powerSaving = status?.power_saving === true
    || (!status && powerCfg.enabled
      && (powerCfg.threshold === 0 || (powerCfg.threshold === -1 && onBattery)));
  const powerLevel = status?.power_saving_level || powerCfg.level || 2;
  // Уровень 3 («максимальная экономия»): глушит анимации и кастомную тему.
  const powerSaveMax = powerSaving && powerLevel === 3;
  const [autoUpdateCheck, setAutoUpdateCheck] = useState(true);
  const [updateModal, setUpdateModal] = useState(null);
  const failCountRef = useRef(0);
  // Сериализованный прошлый /api/status: новый объект кладём в state только
  // при фактическом изменении — иначе все memo-страницы перерисовывались бы
  // на каждый 2-с тик даже без изменений данных.
  const statusStrRef = useRef(null);
  // Зеркало startupDone для колбеков вне эффекта (pollStatus читает реф,
  // поэтому не зависит от устаревшего замыкания).
  const startupDoneRef = useRef(false);
  const themeInitRef = useRef(false);
  // Зеркало «уровень 3 экономии» и признак подавленной кастомной темы:
  // читаются в эффекте инициализации темы (см. ниже).
  const powerMaxRef = useRef(false);
  const customSuppressedRef = useRef(false);

  const showToast = useCallback((msg, type, opts) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, msg, type, ...(opts || {}) }]);
    // Ошибка, показанная пользователю, обязана пережить окно: журнал, отчёт,
    // сервер (LogUiError). Все error-тосты приложения идут через эту точку.
    if (type === 'error') logUiError(msg, opts);
  }, []);

  const { run: runClose } = useActionFeedback(showToast);

  const removeToast = useCallback((id) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  // Иконка профиля: статус-окантовка (ok/offline/none), детали — в панели.
  const userBadge = useMemo(() => ({ state: authState }), [authState]);
  const onLogout = useCallback(async () => {
    await api('POST', '/api/auth/logout');
    setMeInfo(null);
    setAuthState('none');
    showToast(t('profile.logoutDone'), 'success');
  }, [showToast, t]);

  // Стабильная идентичность: инлайн-стрелка в onAuthed перезапускала эффект
  // модалки авторизации на каждом рендере App (опрос статуса каждые 2с) —
  // код авторизации перевыпускался каждые 2 секунды.
  const onAuthed = useCallback(async (res, opts) => {
    await refreshMe();
    if (!(opts && opts.quiet)) showToast(t('profile.authDoneToast'), 'success');
    // Промо-карточка при первой регистрации (CRM → Настройки → «Регистрация»):
    // сервер вкладывает registration_promo только новой учётке на новом
    // устройстве — здесь просто показываем то, что пришло.
    if (opts && opts.registered && res && res.registration_promo && res.registration_promo.code) {
      setRegPromo(res.registration_promo);
      setRegPromoOpen(true);
    }
    // Синхронизация устройств: если у других устройств аккаунта есть снапшот
    // (настройки/темы/данные) — предложение перенести (можно пропустить).
    try {
      const r = await api('GET', '/api/account/sync/devices');
      if (r && !r.error && Array.isArray(r.devices) && r.devices.some((d) => !d.current)) {
        setSyncChoiceOpen(true);
      }
    } catch { /* нет сети — не мешаем входу */ }
  }, [refreshMe, showToast, t]);

  // Панель открыли — данные аккаунта досвежаются (срок подписки, устройства).
  useEffect(() => {
    if (profileOpen) refreshMe();
  }, [profileOpen, refreshMe]);

  // Темы, применённые синхронизацией устройств: бэкенд пишет themes.json и
  // шлёт событие — перечитываем кастомную тему и применяем (как при старте).
  useEffect(() => subscribeEvent('theme:sync', () => {
    try {
      const raw = localStorage.getItem('zpui-theme');
      if (raw) applyTheme(raw);
    } catch { /* нет хранилища */ }
  }), []);


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

  // Правые панели взаимоисключающие: открытие журнала закрывает профиль
  // и наоборот — иначе панели стекуются друг на друга.
  const handleOpenLogs = useCallback((errorCode) => {
    setLogErrorCode(errorCode || null);
    setProfileOpen(false);
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

  // Единая точка сохранения темы в конфиг бэкенда. Ретрай: темы обязаны
  // переживати перезагрузку, одноразовый отказ команды недопустим.
  const saveThemeConfig = useCallback(async (themeValue) => {
    const r = await api('POST', '/api/config', { theme: themeValue });
    if (r && r.error) await api('POST', '/api/config', { theme: themeValue });
  }, []);

  // Рассылка факта применения: все точки (ряд «Тема» в настройках, тайтлбар,
  // подавление экономным режимом) читают ОДИН источник — это событие.
  const announceTheme = useCallback((themeValue) => {
    window.dispatchEvent(new CustomEvent('zpui:theme-applied', { detail: { theme: themeValue } }));
  }, []);

  // Единственный применитель темы: тайтлбар вызывает напрямую, карточки
  // настроек присылают событие zpui:theme-changed — оба контрола синхронны.
  // Обычные темы обязаны полностью снимать кастомные переопределения (style-тег).
  const applyTheme = useCallback((themeValue) => {
    if (themeValue === 'custom') return;
    setThemeSetting(themeValue);
    removeCustomTheme();
    // Обычная тема: перекрытие звуками темы снимается — работают настройки из конфига.
    sound.clearThemeSounds();
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
    announceTheme(themeValue);
  }, [saveThemeConfig, announceTheme]);

  // Кастомная тема: style-тег с :root[data-theme="custom"] + режим 'custom'
  // в конфиге. База запоминается для иконки/переключателя тайтлбара.
  // Легаси-путь картинки сверяется с папкой /api/bg/list перед применением.
  const activateCustomTheme = useCallback(async (themeData) => {
    if (!themeData || typeof themeData !== 'object') return;
    const resolved = await resolveThemeBgUrl(themeData);
    applyCustomTheme(resolved);
    // Кастомная тема применяет и свои звуки (scheme/volume перекрывают конфиг).
    sound.setThemeSounds(resolved.sounds);
    setCustomBase(resolved.base === 'light' ? 'light' : 'dark');
    try { localStorage.setItem('zpui-theme', 'custom'); } catch {}
    setThemeMode('manual');
    setThemeSetting('custom');
    setTheme('custom');
    document.documentElement.setAttribute('data-theme', 'custom');
    saveThemeConfig('custom');
    announceTheme('custom');
  }, [saveThemeConfig, announceTheme]);

  const toggleTheme = useCallback(() => {
    // Цикл из трёх режимов: системная → светлая → тёмная → системная.
    // Активная кастомная тема: первый клик возвращает на её базу.
    if (theme === 'custom') { applyTheme(customBase); return; }
    const next = themeSetting === 'system' ? 'light' : themeSetting === 'light' ? 'dark' : 'system';
    applyTheme(next);
  }, [theme, themeSetting, customBase, applyTheme]);

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
      if (powerMaxRef.current) {
        // Уровень 3 экономии: сохранённую custom при старте не применяем —
        // помечаем подавленной, при снижении уровня эффект выше её вернёт.
        customSuppressedRef.current = true;
        setThemeSetting('custom');
        api('GET', '/api/system-theme').then(sys => {
          const resolved = sys === 'light' ? 'light' : 'dark';
          setTheme(resolved);
          setThemeMode('system');
          document.documentElement.setAttribute('data-theme', resolved);
        }).catch(() => {});
        return;
      }
      // В конфиге кастомная тема: применяем сохранённую, без неё — системную.
      api('GET', '/api/custom-theme').then(r => {
        if (r && r.theme) activateCustomTheme(r.theme);
        else applyTheme('system');
      });
    } else if (savedTheme === 'system') {
      try { localStorage.removeItem('zpui-theme'); } catch {}
      setThemeSetting('system');
      api('GET', '/api/system-theme').then(sysTheme => {
        if (sysTheme) {
          const resolved = sysTheme === 'dark' ? 'dark' : 'light';
          setTheme(resolved);
          document.documentElement.setAttribute('data-theme', resolved);
        }
      });
      announceTheme('system');
    } else {
      try { localStorage.setItem('zpui-theme', savedTheme); } catch {}
      setThemeSetting(savedTheme === 'dark' ? 'dark' : 'light');
      setTheme(savedTheme === 'dark' ? 'dark' : 'light');
      setThemeMode(savedTheme);
      document.documentElement.setAttribute('data-theme', savedTheme);
      announceTheme(savedTheme);
    }
  }, [startupDone, status, applyTheme, activateCustomTheme, announceTheme]);

  // Status polling — с самого монтирования (включая стартовый экран), иначе
  // дашборд после загрузки всегда застаёт status=null: пилюли сервисов пустые,
  // пока не пройдёт первый тик после startupDone. Окно скрыто (трей/свернуто) —
  // опрос приостанавливаем: WebView2 не гарантирует троттлинг таймеров, а каждый
  // запрос будит CPU.
  const pollStatus = async () => {
    if (!isInteractive()) return;
    const bridgeOk = !!(window.go && window.go.app && window.go.app.App);
    const data = bridgeOk ? await api('GET', '/api/status', undefined, 8000) : null;
    if (data && !data.error) {
      if (failCountRef.current > 0) console.log('[App] Backend reconnected');
      failCountRef.current = 0;
      setBackendOnline(true);
      setOfflineReason('');
      const dataStr = JSON.stringify(data);
      if (dataStr !== statusStrRef.current) {
        statusStrRef.current = dataStr;
        // Экономный режим для фоновых таймеров (powerFlag → usePolling/useNow):
        // до первого статуса флаг false, дальше тянется за бэкендом.
        setPowerSaving(data.power_saving === true);
        setStatus(data);
      }
      // Связь восстановлена — счётчики автоперезагрузок больше не нужны.
      try { sessionStorage.removeItem('zpui_reload_bridge'); sessionStorage.removeItem('zpui_reload_api'); } catch {}
    } else if (startupDoneRef.current) {
      // До конца стартового экрана биндинги могли ещё не подняться — такие
      // попытки не считаем сбоями (иначе ложный офлайн-экран на старте).
      // Ответ с полем error (исключение биндинга) — тоже сбой, а не успех.
      failCountRef.current++;
      if (failCountRef.current === 3) {
        setOfflineReason(bridgeOk ? 'api' : 'bridge');
        setBackendOnline(false);
      }
    }
  };
  usePolling(pollStatus, powerSaving ? 5000 : 2000);

  // Прогрев ключей вкладок на простое после стартового экрана: повторные
  // открытия DNS/Монитора/Прокси/Дашборда показывают данные мгновенно из кэша.
  useEffect(() => {
    if (!startupDone) return;
    const t = setTimeout(() => {
      apiWarm([
        '/api/dns/config',
        '/api/dns/current',
        '/api/resource-status',
        '/api/tgproxy/status',
        '/api/tgproxy/config',
        '/api/socks/status',
        '/api/monitor/devices',
        '/api/monitor/traffic-history',
        '/api/availability/history?hours=1&type=standard',
        '/api/availability/history?hours=1&type=user',
        '/api/availability/history?hours=1&type=key',
        '/api/zapret/strategy-changes',
      ]);
    }, 2500);
    return () => clearTimeout(t);
  }, [startupDone]);

  // Мёртвый бридж или молчащий бэкенд сами не восстанавливаются — страница
  // перезагружается через asset server и получает рантайм Wails заново;
  // обычно это снимает «ожидание бэкенда» без перезапуска приложения.
  // Счётчик в sessionStorage ограничивает цикл, если перезагрузка не помогла.
  useEffect(() => {
    if (backendOnline || !offlineReason) return;
    const delay = offlineReason === 'bridge' ? 15000 : 45000;
    const key = 'zpui_reload_' + offlineReason;
    const t = setTimeout(() => {
      let n = 0;
      try { n = parseInt(sessionStorage.getItem(key) || '0', 10); } catch {}
      if (n >= 2) return;
      try { sessionStorage.setItem(key, String(n + 1)); } catch {}
      window.location.reload();
    }, delay);
    return () => clearTimeout(t);
  }, [backendOnline, offlineReason]);

  // Окно снова видно (открытие из трея) — не ждём ближайшего тика: статус
  // обновляется сразу, пилюли/индикаторы не висят устаревшими.
  useEffect(() => {
    const onVisible = () => { if (isInteractive()) pollStatus(); };
    document.addEventListener('visibilitychange', onVisible);
    const offWin = onWindowShownChange(onVisible);
    return () => {
      document.removeEventListener('visibilitychange', onVisible);
      offWin();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Настройки экономного режима: из конфига (при старте и при изменении).
  // Там же — звуки интерфейса (sounds.{enabled,volume,scheme}).
  useEffect(() => {
    if (!startupDone) return;
    const load = () => api('GET', '/api/config').then(c => {
      if (c) {
        setPowerCfg({
          enabled: c.power_saving === true,
          level: c.power_level || 2,
          threshold: c.power_threshold || 0,
        });
        if (c.sounds && typeof c.sounds === 'object') sound.setConfig(c.sounds);
      }
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

  // Экономный режим уровня 3 («максимальная экономия») глушит анимации:
  // и декоративные бесконечные (пульсы, shimmer), и конечные UI-анимации,
  // а также ПРИНУДИТЕЛЬНО звуки интерфейса. Уровни 1–2 их не трогают.
  useEffect(() => {
    document.body.classList.toggle('power-save', powerSaveMax);
    sound.setSuppressed(powerSaveMax);
  }, [powerSaveMax]);

  // Уровень 3 отключает кастомную тему (откат на системную): стиль и маркер
  // снимаются, но выбор в конфиге НЕ перезаписывается — при снижении уровня
  // активная сохранённая тема возвращается. На старте сохранённая custom при
  // максимальном уровне не применяется (powerMaxRef — зеркало powerSaveMax),
  // но помечается подавленной, чтобы «вернуть» при снижении.
  useEffect(() => { powerMaxRef.current = powerSaveMax; }, [powerSaveMax]);
  useEffect(() => {
    if (!startupDone) return;
    if (powerSaveMax) {
      const isCustom = theme === 'custom'
        || document.documentElement.getAttribute('data-theme') === 'custom';
      if (!isCustom) return;
      customSuppressedRef.current = true;
      removeCustomTheme();
      api('GET', '/api/system-theme').then(sys => {
        const resolved = sys === 'light' ? 'light' : 'dark';
        setTheme(resolved);
        setThemeMode('system');
        document.documentElement.setAttribute('data-theme', resolved);
      }).catch(() => {});
      return;
    }
    if (!customSuppressedRef.current) return;
    customSuppressedRef.current = false;
    api('GET', '/api/config').then(c => {
      if (c && c.theme === 'custom') {
        api('GET', '/api/custom-theme').then(r => {
          if (r && r.theme) activateCustomTheme(r.theme);
        }).catch(() => {});
      }
    }).catch(() => {});
  }, [powerSaveMax, theme, startupDone, activateCustomTheme]);

  // Окно (.stand-win) могло смонтироваться позже применения кастомной темы
  // (AuthGate/стартап поверх): после его появления переустанавливаем маркер
  // фоновой картинки, иначе ::before сцены не красился.
  useEffect(() => {
    if (!startupDone) return;
    resyncCustomThemeMark();
  }, [startupDone, authState]);

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
      // События бэкенда (нужен подбор) открывают модалку БЕЗ автостарта —
      // запуск только кнопкой «Запуск».
      setAutoSelectOpen(true);
    };
    const zapretNeedsAutoSelectHandler = () => {
      setAutoSelectOpen(true);
    };
    const dnsAutoDisabledHandler = (data) => {
      showToast(t('xboxdns.autoDisabled', { reason: data?.reason || '' }), 'warning');
    };
    const offs = [
      subscribeEvent('update:available', handler),
      subscribeEvent('zapret:files-missing', filesHandler),
      subscribeEvent('zapret:recovering', recoveryHandler),
      subscribeEvent('zapret:recovery_complete', recoveryCompleteHandler),
      subscribeEvent('zapret:recovery_failed', recoveryFailedHandler),
      subscribeEvent('operator:auto_select_needed', autoSelectNeededHandler),
      subscribeEvent('zapret:needs_autoselect', zapretNeedsAutoSelectHandler),
      subscribeEvent('xboxdns:auto_disabled', dnsAutoDisabledHandler),
    ];
    return () => offs.forEach((off) => off());
  }, [startupDone, showToast, t]);

  // Модалка «пора пройти подбор»: прошёл 7+ дней с последнего ручного прогона
  // (selection_last_at в /api/status, пишет бэкенд). Один раз за сессию
  // (реф-флаг, не localStorage); гейты: стартовый экран завершён, автоподбор
  // не идёт (auto_test_running), модалка подбора ещё не открыта.
  const selectionStaleShownRef = useRef(false);
  useEffect(() => {
    if (!startupDone || selectionStaleShownRef.current) return;
    if (autoSelectOpen || status?.zapret?.auto_test_running === true) return;
    const last = Number(status?.selection_last_at) || 0;
    if (last <= 0 || Date.now() / 1000 - last <= 7 * 24 * 3600) return;
    selectionStaleShownRef.current = true;
    setSelectionStaleOpen(true);
  }, [startupDone, status, autoSelectOpen]);

  useRecoveryEvents({ startupDone, setRecovery, setZapretLoading, setRecoveryReport });

  const handleStartupComplete = useCallback((info) => {
    startupDoneRef.current = true;
    setStartupDone(true);
    // Самолечение гейта видимости: стартовый экран просил показ через бэкенд,
    // событие window:shown могло не дойти — переспрашиваем фактическое
    // состояние окна, иначе вкладки остаются заглушенными до переоткрытия.
    probeWindowShown();
    if (!localStorage.getItem('zpui-manual-seen')) {
      localStorage.setItem('zpui-manual-seen', '1');
      navigate('docs');
    }
    // Чистая база (первый запуск): сразу открываем «Умный подбор» — модалка
    // сама стартует прогон (стартовый перебор к этому моменту уже завершён).
    // Гард: движок не поднялся (engineFail) — не отвлекаем пользователя
    // от модалки ошибки.
    if (info && info.firstRun && !engineFail) {
      setAutoSelectAutoStart(true);
      setAutoSelectOpen(true);
    }
  }, [navigate, probeWindowShown, engineFail]);

  // Стабильные обработчики для мемоизированного хрома и модалок: каждая
  // инлайн-стрелка в JSX ломала memo и возвращала перерендер всего дерева.
  const openProfile = useCallback(() => setProfileOpen(true), []);
  const openLogs = useCallback(() => setLogsOpen(true), []);
  const openDiagnostics = useCallback(() => setDiagOpen(true), []);
  const openChecker = useCallback(() => setCheckerOpen(true), []);
  const closeChecker = useCallback(() => setCheckerOpen(false), []);
  const openHelp = useCallback(() => navigate('docs'), [navigate]);
  const openCheckerWithUrl = useCallback((url) => {
    setCheckerInitialUrl(url);
    setCheckerOpen(true);
  }, []);
  const openCache = useCallback(() => setCacheOpen(true), []);
  const openMcp = useCallback(() => setMcpOpen(true), []);
  const closeMcp = useCallback(() => setMcpOpen(false), []);
  const openExtension = useCallback(() => setExtensionOpen(true), []);
  const closeExtension = useCallback(() => setExtensionOpen(false), []);
  const openHealth = useCallback(() => setHealthOpen(true), []);
  const openAutoSelect = useCallback(() => setAutoSelectOpen(true), []);
  const closeAutoSelect = useCallback(() => {
    setAutoSelectOpen(false);
    setAutoSelectAutoStart(false);
  }, []);
  const closeDiagnostics = useCallback(() => setDiagOpen(false), []);
  const closeCache = useCallback(() => setCacheOpen(false), []);
  const closeHealth = useCallback(() => setHealthOpen(false), []);
  const closeProfile = useCallback(() => setProfileOpen(false), []);
  const closeAuthSuggest = useCallback(() => setAuthSuggest(false), []);
  const closeSelectionStale = useCallback(() => setSelectionStaleOpen(false), []);
  // «Запустить подбор» из модалки «данные устарели» — тот же механизм, что
  // у кнопки «Автоподбор» в сайдбаре (openAutoSelect → autoSelectOpen).
  const runSelectionFromStale = useCallback(() => {
    setSelectionStaleOpen(false);
    openAutoSelect();
  }, [openAutoSelect]);
  const authSuggestGoAuth = useCallback(() => {
    setAuthSuggest(false);
    setProfileOpen(true);
  }, []);
  const closeLogs = useCallback(() => {
    setLogsOpen(false);
    setLogErrorCode(null);
  }, []);
  const closeUpdateModal = useCallback(() => setUpdateModal(null), []);
  const closeRecoveryReport = useCallback(() => setRecoveryReport(null), []);
  const closeTitleBarUser = useCallback(() => {
    setLogsOpen(false);
    setProfileOpen(true);
  }, []);
  const toggleMaximize = useCallback(() => {
    setWinMax(v => !v);
    api('POST', '/api/app/maximize');
  }, []);
  const minimizeWindow = useCallback(() => { api('POST', '/api/app/minimize'); }, []);
  const closeWindow = useCallback(() => {
    runClose(() => api('POST', '/api/app/close'), { errorMsg: t('stand.closeFailed') });
  }, [runClose, t]);
  const openUpdateFromChip = useCallback(() => {
    const src = zpuiCheck.state === 'available' ? zpuiCheck : zapretCheck;
    setUpdateModal({ component: src === zpuiCheck ? 'ZPUI' : 'zapret', current: src.current, latest: src.latest });
  }, [zpuiCheck, zapretCheck]);

  const updateInfo = useMemo(() => (
    zpuiCheck.state === 'available' && zpuiCheck.latest ? { component: 'ZPUI', current: zpuiCheck.current, latest: zpuiCheck.latest }
      : zapretCheck.state === 'available' && zapretCheck.latest ? { component: 'zapret', current: zapretCheck.current, latest: zapretCheck.latest }
        : null
  ), [zpuiCheck, zapretCheck]);

  // pageProps: стабильная идентичность обязательна для memo-страниц —
  // пересоздаётся только при смене статуса/загрузки запрета.
  const pageProps = useMemo(() => ({
    status,
    onOpenAuth: openProfile,
    showProgressToast,
    updateProgressToast,
    completeProgressToast,
    onNavigate: navigate,
    onOpenLogs: openLogs,
    onOpenDiagnostics: openDiagnostics,
    onOpenCheckerWithUrl: openCheckerWithUrl,
    zapretLoading,
    onZapretToggle,
  }), [status, openProfile, showProgressToast, updateProgressToast, completeProgressToast, navigate, openLogs, openDiagnostics, openCheckerWithUrl, zapretLoading, onZapretToggle]);

  // Show startup screen when not done
  if (!startupDone) {
    return (
      <>
        <StartupScreen onComplete={handleStartupComplete} />
        <MTooltipLayer />
        <MToast toasts={toasts} onRemove={removeToast} version={status?.mod?.version} />
      </>
    );
  }

  const pageContent = backendOnline ? (
    <>
      <div className="main-area">
        {visitedPages.map((p) => {
          const PageComp = MEMO_PAGES[p];
          if (!PageComp) return null;
          const active = p === activePage;
          return (
            <PageVisibleProvider key={p} value={active}>
              <div
                className={'page-host' + (active ? ' page-fade' : ' page-host-hidden')}
                aria-hidden={!active}
              >
                <PageComp {...pageProps} />
              </div>
            </PageVisibleProvider>
          );
        })}
      </div>
    </>
  ) : (
    <div className="main-area">
      <OfflineScreen onRetry={async (silent) => {
        const d = await api('GET', '/api/status', undefined, 8000);
        if (d && !d.error) {
          setBackendOnline(true);
          setOfflineReason('');
          failCountRef.current = 0;
          setStatus(d);
          try { sessionStorage.removeItem('zpui_reload_bridge'); sessionStorage.removeItem('zpui_reload_api'); } catch {}
        } else if (!silent) {
          showToast(t('offline.backendUnavailable'), 'error');
        }
      }} reason={offlineReason} />
    </div>
  );

  return (
    <ToastProvider value={showToast}>
    <ConfirmProvider>
    <div className="app stand-app">
      <div className="stand-win">
        <MTitleBar
          version={status?.mod?.version}
          arch={status?.mod?.arch}
          theme={theme === 'custom' ? customBase : theme}
          updateInfo={updateInfo}
          onOpenUpdate={openUpdateFromChip}
          onToggleTheme={toggleTheme}
          themeSetting={themeSetting}
          maximized={winMax}
          onToggleMaximize={toggleMaximize}
          onMinimize={minimizeWindow}
          onClose={closeWindow}
          user={userBadge}
          onUserClick={closeTitleBarUser}
        />
        <div className="stand-frame">
        <MSidebar
          activePage={activePage}
          onNavigate={navigate}
          onOpenChecker={openChecker}
          onAutoSelect={openAutoSelect}
          onOpenHealth={openHealth}
          onOpenHelp={openHelp}
          healthWarn={healthWarn}
          status={status}
          authEnabled={authEnabled}
         
          onOpenLogs={openLogs}
          onOpenCache={openCache}
          onOpenMcp={openMcp}
          onOpenExtension={openExtension}
          powerSaving={powerSaving}
          powerLevel={powerLevel}
        />
        <div className="stand-main">
          <div className="app-body stand-content">
            {pageContent}
          </div>
          <MFooter status={status} authEnabled={authEnabled} />
        </div>
        </div>
      <MLogDrawer open={logsOpen} onClose={closeLogs} scrollToError={logErrorCode} />
      {checkerOpen && <MResourceChecker onClose={closeChecker} initialUrl={checkerInitialUrl} />}
      {healthOpen && (
        <MHealthCheckModal
          onClose={closeHealth}
         
          onNavigate={navigate}
          onOpenDiagnostics={openDiagnostics}
        />
      )}
      <MAutoSelectModal open={autoSelectOpen} onClose={closeAutoSelect} autoStart={autoSelectAutoStart} />
      <MDiagnosticsModal open={diagOpen} onClose={closeDiagnostics} />
      <MCacheModal open={cacheOpen} onClose={closeCache} />
      <MMcpModal open={mcpOpen} onClose={closeMcp} />
      <MExtensionModal open={extensionOpen} onClose={closeExtension} />
      <MUpdateAvailableModal
        data={updateModal}
        onClose={closeUpdateModal}
      />
      <MUpdateProgressModal />
      <MRecoveryModal
        open={recovery.open}
        phase={recovery.phase}
        steps={recovery.steps}
      />
      {recoveryReport && (
        <MRecoveryFailedModal
          report={recoveryReport}
          secondFailure={!!recoveryReport.second_failure}
          version={status?.mod?.version}
          onClose={closeRecoveryReport}
         
        />
      )}
      <MToast toasts={toasts} onRemove={removeToast} version={status?.mod?.version} onOpenLogs={handleOpenLogs} onReportBug={openBugReport} />
      <MProfileDrawer
        open={profileOpen}
        onClose={closeProfile}
        me={meInfo}
        arch={status?.mod?.arch}
        onAuthed={onAuthed}
        onLogout={onLogout}
       
      />
      <MAuthSuggestModal
        open={authSuggest}
        onClose={closeAuthSuggest}
        onAuth={authSuggestGoAuth}

      />
      <MSyncChoiceModal
        open={syncChoiceOpen}
        onClose={() => setSyncChoiceOpen(false)}
        onRestored={() => { /* применённые настройки/тема подхватятся по событиям */ }}
      />
      <MPromoRevealModal
        open={regPromoOpen}
        promo={regPromo}
        onClose={() => { setRegPromoOpen(false); setRegPromo(null); }}
        onActivated={refreshMe}
      />
      <MSelectionStaleModal
        open={selectionStaleOpen}
        onClose={closeSelectionStale}
        onRun={runSelectionFromStale}
      />
      {starPromo && (
        <StarPromoModal onClose={() => setStarPromo(false)} />
      )}
      {startupDone && <EngineFailModal />}
      <BugReportModal
        open={bugReport.open}
        code={bugReport.code}
        error={bugReport.error}
        onClose={() => setBugReport((b) => ({ ...b, open: false }))}
      />
      </div>
    </div>
    <MTooltipLayer />
    </ConfirmProvider>
    </ToastProvider>
  );
}
