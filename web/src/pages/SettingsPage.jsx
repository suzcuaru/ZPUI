import { useState, useEffect, useCallback, useRef } from 'react';
import Switch from '../components/ui/Switch';
import ReleaseModal from '../components/ui/ReleaseModal';
import Modal from '../components/ui/Modal';
import ThemeGallery from '../components/ThemeGallery';
import { api, apiCall, apiCacheGet, apiCachePut } from '../api';
import { useT } from '../i18n';
import { ArrowRight, Trash2, RefreshCw, Download, AlertTriangle, Power, HardDriveDownload, Palette, Layers, Volume2, Database } from 'lucide-react';
import { usePolling } from '../hooks/usePolling';
import { useDebouncedSave } from '../hooks/useDebouncedSave';
import { useUpdateCheck } from '../hooks/useUpdateCheck';
import { openUpdateModal, showUpdateModal, startExtensionUpdate, useUpdateFlow } from '../hooks/useUpdateFlow';
import ThemeEditorTab from '../components/ThemeEditorTab';
import { useConfirm } from '../components/ui/ConfirmDialog';
import { rangePct } from '../utils';
import { normalizeTheme } from '../themeCustom';
import { sound } from '../sound';
import { exportThemeZip, importThemeZip, downloadBlob, zipNameFor, looksLikeZip, blobToText } from '../themeZip';
import Spinner from '../components/ui/Spinner';
import SyncDataTab from '../components/SyncDataTab';
import { useToast } from '../context/ToastContext';
import Select from '../components/ui/Select';

// Экономный режим: уровни степени экономии и пороги заряда батареи
// (0 = «всегда», -1 = «от АКБ», далее 50..10 % с шагом 5).
const POWER_LEVEL_KEYS = ['settings.powerLow', 'settings.powerMedium', 'settings.powerMax'];
const POWER_THRESHOLDS = [0, -1, 50, 45, 40, 35, 30, 25, 20, 15, 10];
const EXT_INFO_ROUTE = '/api/extension/info';

function thresholdLabel(threshold, t) {
  if (threshold === 0) return t('settings.powerThresholdAlways');
  if (threshold === -1) return t('settings.powerThresholdOnBattery');
  return threshold + '%';
}

// ZsvcPanel — панель службы ZPUI над панелью ZAPRET: версия/uptime и
// автозапуск (поллинг раз в 30с). Плитки «Приложение» и «Краши» убраны
// (решение владельца 14.11). Одна и та же рамка при загрузке и с данными —
// вкладка не «прыгает».
function ZsvcPanel() {
  const { t } = useT();
  const [svc, setSvc] = useState(null);
  const [fails, setFails] = useState(0);
  const load = useCallback(async () => {
    const cached = apiCacheGet('/api/service/panel');
    if (cached) setSvc(cached);
    const r = await api('GET', '/api/service/panel');
    if (r && !r.error) {
      apiCachePut('/api/service/panel', r);
      setSvc(r);
      setFails(0);
    } else setFails((n) => n + 1);
  }, []);
  usePolling(load, 30000);

  const loading = !svc;
  const noData = loading && fails >= 3;
  const up = Number(svc?.uptime_sec) || 0;
  const uptime = up > 0
    ? (up >= 3600
        ? t('settings.svcUptimeHm', { h: Math.floor(up / 3600), m: Math.floor((up % 3600) / 60) })
        : t('settings.svcUptimeM', { m: Math.floor(up / 60) }))
    : '';
  const sub = (txt) => txt || '\u00A0';
  return (
    <div className="section zsvc-panel">
      <div className="zsvc-head">
        <span className="zsvc-title">{t('settings.svcPanelTitle')}</span>
        {loading
          ? <span className="zpset-pill off">{noData ? t('settings.svcNoData') : '—'}</span>
          : <span className={'zpset-pill ' + (svc.running ? 'on' : 'off')}>
              {svc.running ? t('settings.svcRunning') : t('settings.svcStopped')}
            </span>}
      </div>
      <div className="zsvc-tiles">
        <div className="zsvc-tile">
          <span className="zsvc-tile-k">{t('settings.svcRowVersion')}</span>
          <span className="zsvc-tile-v mono">{loading ? '—' : (svc.installed ? `v${svc.version || '—'}` : t('settings.svcNotInstalled'))}</span>
          <span className="zsvc-tile-sub">{sub(!loading && svc.running && uptime ? uptime : '')}</span>
        </div>
        <div className="zsvc-tile">
          <span className="zsvc-tile-k">{t('settings.svcRowAutostart')}</span>
          <span className={'zsvc-tile-v ' + (!loading && svc.autostart ? 'ok' : '')}>{loading ? '—' : (svc.autostart ? t('settings.svcAutostartYes') : t('settings.svcAutostartNo'))}</span>
          <span className="zsvc-tile-sub">{'\u00A0'}</span>
        </div>
      </div>
    </div>
  );
}
export default function SettingsPage({ status, onOpenLogs }) {
  const showToast = useToast();
  const { t, lang, changeLang } = useT();
  const confirm = useConfirm();
  const [config, setConfig] = useState(null);
  const [versions, setVersions] = useState(null);
  const [reinstalling, setReinstalling] = useState(false);
  const [fullReinstalling, setFullReinstalling] = useState(false);
  const [releaseVer, setReleaseVer] = useState(null);          // версия ZPUI, открытая в модалке «что нового»
  const [zapretReleaseVer, setZapretReleaseVer] = useState(null); // версия Zapret
  const [verPanel, setVerPanel] = useState(null);
  const [zapretVerPanel, setZapretVerPanel] = useState(null);
  const [syncInfo, setSyncInfo] = useState(null); // канал ZPUI Sync: recommended для карточки ZAPRET
  const [updSource, setUpdSource] = useState(null); // источник обновлений: auto/server/yandex/github/ghproxy
  const [extInfo, setExtInfo] = useState(null); // браузерный плагин: пакет, версии, обновление (/api/extension/info)
  // Оптимистичные оверлеи тумблера автозапуска и режима службы: сами значения
  // приходят из /api/status мгновенно (конфиг и кэш бэкенда), оверлей держится
  // до подтверждения очередным тиком статуса — скелетоны не нужны.
  const [autostartOpt, setAutostartOpt] = useState(null);
  const [autostartBusy, setAutostartBusy] = useState(false); // schtasks-запись — тумблер со спиннером
  const [svcOpt, setSvcOpt] = useState(null);
  // Автозапуск службы запрета (тип запуска в SCM): null = ещё не загружено.
  const [svcAuto, setSvcAuto] = useState(null);
  const [svcAutoBusy, setSvcAutoBusy] = useState(false);
  const [settingsTab, setSettingsTab] = useState('general'); // под-вкладки: «Общие» | «Тема»
  const [themes, setThemes] = useState([]);        // слоты сохранённых тем [{id,name,theme}]
  const [activeCustomId, setActiveCustomId] = useState(null);
  const [galleryOpen, setGalleryOpen] = useState(false); // модалка-галерея «Своя тема»
  const [teSlot, setTeSlot] = useState(null);      // слот в редакторе: {id, name} | null
  const teScrollRef = useRef(null);

  const { zpuiCheck, zapretCheck } = useUpdateCheck();
  const flow = useUpdateFlow();

  const loadConfig = async () => {
    const d = await api('GET', '/api/config');
    if (d) setConfig(d);
  };

  const loadVerPanel = async () => {
    const d = await api('GET', '/api/updates/versions-panel');
    if (d && !d.error) setVerPanel(d);
  };

  const loadZapretVerPanel = async () => {
    const d = await api('GET', '/api/updates/zapret-versions-panel');
    if (d && !d.error) setZapretVerPanel(d);
  };

  const loadVersions = async () => {
    const d = await api('GET', '/api/versions');
    if (d) setVersions(d);
  };

  const loadSyncInfo = async () => {
    const d = await api('GET', '/api/sync/info');
    if (d && !d.error) setSyncInfo(d);
  };

  const loadUpdSource = async () => {
    const d = await api('GET', '/api/updates/source');
    if (d && !d.error && d.source) setUpdSource(d.source);
  };

  const loadExtInfo = async () => {
    // SWR-кэш, как у панели службы: мгновенный отклик при возврате на вкладку
    const cached = apiCacheGet(EXT_INFO_ROUTE);
    if (cached) setExtInfo(cached);
    const r = await api('GET', EXT_INFO_ROUTE);
    if (r && !r.error) {
      apiCachePut(EXT_INFO_ROUTE, r);
      setExtInfo(r);
    }
  };

  useEffect(() => {
    loadConfig();
    loadVerPanel();
    loadZapretVerPanel();
    loadSyncInfo();
    loadUpdSource();
    loadExtInfo();
  }, []);
  usePolling(loadVersions, 60000);
  usePolling(loadSyncInfo, 60000);

  // Источники мгновенной отрисовки: автозапуск — конфиг (задачу создаёт
  // служба ZPUI, опрашивать Планировщик не нужно), режим службы — кэш бэкенда.
  const autostartOn = autostartOpt ?? status?.mod?.autostart === true;
  const svcInstalled = svcOpt ?? status?.zapret?.service_installed === true;

  useEffect(() => {
    if (autostartOpt !== null && status?.mod?.autostart === autostartOpt) setAutostartOpt(null);
    if (svcOpt !== null && status?.zapret?.service_installed === svcOpt) setSvcOpt(null);
  }, [status, autostartOpt, svcOpt]);
  usePolling(loadVerPanel, 30000);
  usePolling(loadZapretVerPanel, 30000);
  usePolling(loadExtInfo, 30000);

  // Состояние автозапуска службы запрета — из SCM, читаем один раз при входе
  // на вкладку (дальше им управляет тумблер локально).
  useEffect(() => {
    api('GET', '/api/zapret/service-autostart').then((r) => {
      if (r && !r.error) setSvcAuto(!!r.autostart);
    });
  }, []);

  // Слоты тем: загрузка при старте и по событию zpui:themes-changed (редактор
  // и галерея диспатчат его после сохранения/удаления).
  const loadThemes = useCallback(async () => {
    const r = await api('GET', '/api/custom-themes');
    if (r && Array.isArray(r.themes)) {
      setThemes(r.themes);
      setActiveCustomId(r.activeId || null);
    }
  }, []);

  useEffect(() => { loadThemes(); }, [loadThemes]);

  useEffect(() => {
    const onThemesChanged = () => { loadThemes(); };
    window.addEventListener('zpui:themes-changed', onThemesChanged);
    return () => window.removeEventListener('zpui:themes-changed', onThemesChanged);
  }, [loadThemes]);

  // Локальная синхронизация активной темы с App. Два события:
  //  • zpui:theme-changed — намерение пользователя (клик по кнопке ряда);
  //  • zpui:theme-applied — факт применения от единого применителя (App):
  //    покрывает тайтлбар/экономный режим и не даёт ряду «Тема» разъехаться
  //    (баг «Своя всегда активна»).
  useEffect(() => {
    const syncTheme = (v) => {
      if (v === 'system' || v === 'dark' || v === 'light' || v === 'custom') {
        setConfig((prev) => (prev ? { ...prev, theme: v } : prev));
      }
      // Обычная тема снимает активный слот на бэке — перечитываем, иначе
      // карточка в галерее остаётся «Активна» после ухода с кастомной.
      if (v === 'system' || v === 'dark' || v === 'light') {
        loadThemes();
      }
    };
    const onChanged = (e) => syncTheme(e.detail?.theme);
    const onApplied = (e) => syncTheme(e.detail?.theme);
    window.addEventListener('zpui:theme-changed', onChanged);
    window.addEventListener('zpui:theme-applied', onApplied);
    return () => {
      window.removeEventListener('zpui:theme-changed', onChanged);
      window.removeEventListener('zpui:theme-applied', onApplied);
    };
  }, [loadThemes]);

  useEffect(() => {
    const onZapretInstalled = () => { loadZapretVerPanel(); loadVersions(); };
    window.addEventListener('zpui:zapret-installed', onZapretInstalled);
    return () => window.removeEventListener('zpui:zapret-installed', onZapretInstalled);
  }, []);

  const saveConfig = useDebouncedSave('/api/config', 500, null);


  const update = useCallback((patch) => {
    setConfig(prev => {
      const next = { ...prev, ...patch };
      saveConfig(patch, prev);
      return next;
    });
    // App.jsx перечитывает конфиг на это событие (например, экономный режим
    // меняет интервалы опроса).
    window.dispatchEvent(new CustomEvent('zpui:config-changed'));
  }, [saveConfig]);

  const handleLanguage = (newLang) => {
    changeLang(newLang);
    update({ language: newLang });
  };

  // Звуки интерфейса: вложенный патч sounds.{enabled,volume} в конфиге.
  // sound.setConfig — сразу (отклик без ожидания debounced-сохранения).
  // zpui:config-changed НЕ диспатчим: App по событию перечитал бы конфиг
  // с бэка, куда debounced-POST уходит позже, — и затёр бы свежее состояние
  // движка звука старым значением (тумблер «срабатывал со второго раза»).
  const soundsCfg = config && config.sounds && typeof config.sounds === 'object'
    ? config.sounds
    : { enabled: false, volume: 60 };
  const soundsMuted = status?.power_saving === true && status?.power_saving_level === 3;
  const patchSounds = useCallback((patch) => {
    setConfig(prev => {
      const base = prev || {};
      const cur = base.sounds && typeof base.sounds === 'object' ? base.sounds : {};
      const next = { ...cur, ...patch };
      sound.setConfig(next);
      saveConfig({ sounds: next }, base);
      return { ...base, sounds: next };
    });
  }, [saveConfig]);

  // Тема применяется в App (единая точка): он же переключит иконку в тайтлбаре.
  // Здесь только локальный отклик кнопок — какая активна.
  const handleTheme = (theme) => {
    setConfig(prev => ({ ...prev, theme }));
    window.dispatchEvent(new CustomEvent('zpui:theme-changed', { detail: { theme } }));
  };

  // «Своя»: открывает модалку-галерею сохранённых тем (выбор/создание/импорт).
  const handleThemeCustom = () => {
    setGalleryOpen(true);
  };

  // Под-вкладка «Тема»: всегда сначала галерея; редактор открывается
  // явно — карандашом на карточке или «Создать тему».
  const openThemeTab = () => {
    setSettingsTab('theme');
    requestAnimationFrame(() => {
      const el = teScrollRef.current;
      if (el) el.scrollTop = 0;
    });
  };

  // Редактор конкретного слота на под-вкладке «Тема» (из карточки/создания).
  const editSlot = (s) => {
    setGalleryOpen(false);
    setSettingsTab('theme');
    setTeSlot({ id: s.id, name: s.name || '' });
    requestAnimationFrame(() => {
      const el = teScrollRef.current;
      if (el) el.scrollTop = 0;
    });
  };

  // Новый пустой слот → редактор.
  const createSlot = () => {
    editSlot({ id: null, name: t('themeGallery.newDefaultName') });
  };

  // Применить слот: активация в «БД» + событие App (перечитает и применит).
  const applySlot = async (s) => {
    const r = await api('POST', '/api/custom-themes/activate', { id: s.id });
    if (r && r.error) {
      showToast(r.error, 'error');
      return;
    }
    setConfig(prev => ({ ...prev, theme: 'custom' }));
    window.dispatchEvent(new CustomEvent('zpui:theme-changed', { detail: { theme: 'custom' } }));
    loadThemes();
    setGalleryOpen(false);
    showToast(t('themeGallery.applied'), 'success');
  };

  // Экспорт слота: тема + файлы звуковой схемы + фоновая картинка → ZIP
  // (zpui-theme-<имя>.zip). Легаси-импорт .json по-прежнему принимается.
  const exportSlot = async (s) => {
    try {
      const blob = await exportThemeZip(s);
      downloadBlob(blob, zipNameFor(s.name));
    } catch {
      showToast(t('themeEditor.exportFail'), 'error');
    }
  };

  // Удаление слота: активный блокируется на карточке, здесь — защита «БД».
  const deleteSlot = async (s) => {
    if (!await confirm({
      message: t('themeGallery.deleteConfirm', { name: s.name || t('themeGallery.unnamed') }),
      variant: 'danger',
      confirmText: t('themeGallery.delete'),
    })) return;
    const r = await api('POST', '/api/custom-themes/delete', { id: s.id });
    if (r && r.error) {
      showToast(t(r.error === 'theme_active' ? 'themeGallery.deleteActiveTip' : 'themeGallery.deleteFail'), 'error');
      return;
    }
    if (teSlot && teSlot.id === s.id) setTeSlot(null);
    loadThemes();
    showToast(t('themeGallery.deleted'), 'success');
  };

  // Импорт файла темы → новый слот (без активации). ZIP: theme.json + фоновая
  // картинка (заливается в /bg) + звуковая схема (сверяется с библиотекой);
  // одиночный .json — легаси-путь.
  const importSlotFile = async (file) => {
    try {
      if (await looksLikeZip(file)) {
        const { theme } = await importThemeZip(file);
        const name = file.name.replace(/\.zip$/i, '').replace(/^zpui-theme-?/i, '').slice(0, 40);
        const r = await api('POST', '/api/custom-themes', { name, theme, activate: false });
        if (r && r.error) {
          showToast(r.error, 'error');
          return;
        }
        loadThemes();
        showToast(t('themeGallery.imported'), 'success');
        return;
      }
      const parsed = JSON.parse(await blobToText(file));
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('bad');
      const theme = normalizeTheme(parsed);
      const name = file.name.replace(/\.json$/i, '').slice(0, 40);
      const r = await api('POST', '/api/custom-themes', { name, theme, activate: false });
      if (r && r.error) {
        showToast(r.error, 'error');
        return;
      }
      loadThemes();
      showToast(t('themeGallery.imported'), 'success');
    } catch {
      showToast(t('themeEditor.importFail'), 'error');
    }
  };

  // «Применить и закрыть» (и уход со вкладки): тема уже применена живым
  // предпросмотром и закоммичена в слот; перечитываем список тем.
  const closeThemeEditor = () => {
    setTeSlot(null);
    setSettingsTab('general');
    loadThemes();
  };

  // Редактор сохранил новый слот (первое «Сделать активной»/«Применить»):
  // перепривязываем редактор к уже сохранённому слоту — статус «Активна»
  // и кнопка «Сделать активной» считаются от его id.
  const handleSlotSaved = useCallback((id, savedName) => {
    setTeSlot((prev) => (prev ? { id, name: typeof savedName === 'string' ? savedName : prev.name } : prev));
  }, []);

  // Слот активен в системе: кастомная тема включена и это именно этот слот.
  const editorSlotActive = !!(config && config.theme === 'custom'
    && activeCustomId != null
    && teSlot != null
    && teSlot.id === activeCustomId);

  const handleAutostart = async (enabled) => {
    if (autostartBusy) return;
    setAutostartOpt(enabled);
    setAutostartBusy(true);
    const ok = await apiCall(() => api('POST', enabled ? '/api/autostart/enable' : '/api/autostart/disable'), null, showToast);
    setAutostartBusy(false);
    if (!ok) setAutostartOpt(null);
  };

  // Обновления: карточки ниже открывают модалку скачивания/установки.
  const startUpdate = useCallback((component) => {
    // Если этот компонент уже скачан — показываем модалку с кнопкой «Установить».
    if (flow.component === component && flow.phase === 'downloaded') {
      showUpdateModal();
      return;
    }
    openUpdateModal(component);
  }, [flow.component, flow.phase]);

  // Плагин браузера: кнопка ряда качает выпуск с сервера и ставит его тихо
  // (установщик запускается в фоне); после ok перечитываем инфо — там
  // подъедет свежая версия пакета.
  // Плагин браузера: та же модалка прогресса, что у ZPUI/ZAPRET (скачивание →
  // установка в фоновых событиях); итог — событие zpui:extension-installed.
  const extUpdating = !!extInfo?.updating
    || (flow.open && flow.component === 'extension'
      && (flow.phase === 'downloading' || flow.phase === 'installing'));
  useEffect(() => {
    const onInstalled = () => loadExtInfo();
    window.addEventListener('zpui:extension-installed', onInstalled);
    return () => window.removeEventListener('zpui:extension-installed', onInstalled);
  }, []);
  const updateExtension = useCallback(() => {
    if (extUpdating) return;
    startExtensionUpdate([]);
  }, [extUpdating]);

  // Смена источника обновлений: оптимистично, откат при ошибке сохранения.
  const changeUpdSource = async (next) => {
    const prev = updSource || 'auto';
    setUpdSource(next);
    const r = await api('POST', '/api/updates/source', { source: next });
    if (!r || r.error) {
      setUpdSource(prev);
      showToast(r?.error || t('toast.requestFailed'), 'error');
    } else {
      showToast(t('settings.updSourceSaved'), 'success');
    }
  };


  // Конфиг ещё грузится — скелетон в той же сетке (как на XboxDns/User),
  // пустой возврат давал «прыжок» пустой вкладки.
  if (!config) {
    return (
      <div className="settings-page">
      <div className="set-columns stagger">
        <div className="section set-main-section skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '70%' }} /></div>
        <div className="set-side-col">
          <div className="section skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '60%' }} /></div>
          <div className="section skeleton skeleton-pulse"><div className="skeleton-line" style={{ width: '60%' }} /></div>
        </div>
      </div>
      </div>
    );
  }

  return (
    <div className="settings-page">
      <div className="set-subtabs" role="tablist" aria-label={t('settings.subtabs')}>
        <button
          role="tab"
          aria-selected={settingsTab === 'general'}
          className={'btn btn-sm' + (settingsTab === 'general' ? ' active' : '')}
          onClick={() => setSettingsTab('general')}
        >
          {t('settings.tabGeneral')}
        </button>
        <button
          role="tab"
          aria-selected={settingsTab === 'data'}
          className={'btn btn-sm' + (settingsTab === 'data' ? ' active' : '')}
          onClick={() => setSettingsTab('data')}
        >
          <Database size={12} strokeWidth={2} />
          {t('settings.tabData')}
        </button>
        <button
          role="tab"
          aria-selected={settingsTab === 'theme'}
          className={'btn btn-sm' + (settingsTab === 'theme' ? ' active' : '')}
          onClick={openThemeTab}
        >
          <Palette size={12} strokeWidth={2} />
          {t('settings.tabTheme')}
        </button>
      </div>

      {settingsTab === 'general' && (
      <div className="set-columns stagger tab-fade">

        <div className="section set-main-section">
          <div className="section-title">{t('settings.appearanceSettings')}</div>
          <div className="set-row">
            <div className="set-row-info"><span className="set-row-title">{t('settings.theme')}</span></div>
            <div className="set-theme-row">
              <button className={'btn btn-sm' + (config.theme === 'system' ? ' active' : '')} onClick={() => handleTheme('system')}>{t('settings.systemTheme')}</button>
              <button className={'btn btn-sm' + (config.theme === 'light' ? ' active' : '')} onClick={() => handleTheme('light')}>{t('settings.lightTheme')}</button>
              <button className={'btn btn-sm' + (config.theme === 'dark' ? ' active' : '')} onClick={() => handleTheme('dark')}>{t('settings.darkTheme')}</button>
              <button className={'btn btn-sm' + (config.theme === 'custom' ? ' active' : '')} onClick={handleThemeCustom}>{t('settings.themeCustom')}</button>
            </div>
          </div>
          <div className="set-row">
            <div className="set-row-info"><span className="set-row-title">{t('settings.language')}</span></div>
            <div className="set-lang-row">
              <button className={'btn btn-sm' + (lang === 'ru' ? ' active' : '')} onClick={() => handleLanguage('ru')}>RU</button>
              <button className={'btn btn-sm' + (lang === 'en' ? ' active' : '')} onClick={() => handleLanguage('en')}>EN</button>
            </div>
          </div>
          <div className="set-row set-sounds">
            <div className="set-row-info">
              <span className="set-row-title">{t('settings.sounds')}</span>
              {soundsMuted && <span className="set-row-desc">{t('settings.soundsDescPower')}</span>}
            </div>
            <div className="set-sounds-ctl">
              <Volume2 size={14} strokeWidth={2} className="set-sounds-ic" aria-hidden="true" />
              <input
                type="range" min="0" max="100" step="5"
                className="set-sounds-vol slider-ui"
                disabled={soundsMuted || soundsCfg.enabled === false}
                value={soundsCfg.volume != null ? soundsCfg.volume : 60}
                aria-label={t('settings.soundsVolume')}
                style={{ '--val': rangePct(soundsCfg.volume != null ? soundsCfg.volume : 60, 0, 100) + '%' }}
                onChange={(e) => patchSounds({ volume: +e.target.value })} />
              <Switch checked={soundsCfg.enabled === false ? false : true} onChange={() => patchSounds({ enabled: !(soundsCfg.enabled === false ? false : true) })} />
            </div>
          </div>
          <MiniRow label={t('settings.autoStartWindows')}>
            <Switch checked={autostartOn} onChange={() => handleAutostart(!autostartOn)} loading={autostartBusy} />
          </MiniRow>
          <MiniRow label={t('settings.startMinimized')}><Switch checked={config.start_minimized || false} onChange={() => update({ start_minimized: !config.start_minimized })} /></MiniRow>
          <MiniRow label={t('settings.closeToTray')}><Switch checked={config.close_to_tray !== false} onChange={() => update({ close_to_tray: !config.close_to_tray })} /></MiniRow>
          <MiniRow label={t('settings.updateCheck')}><Switch checked={config.auto_update_check !== false} onChange={() => update({ auto_update_check: !config.auto_update_check })} /></MiniRow>
          {/* Автозапуск SOCKS5 = тот же флаг, что и тумблер на вкладке прокси;
              живой сервер поднимает фоновый синхронизатор в течение 10 секунд */}
          <MiniRow label={t('settings.autoStartSocksProxy')}><Switch checked={config.proxy_enabled === true} onChange={() => update({ proxy_enabled: config.proxy_enabled !== true })} /></MiniRow>
          <MiniRow label={t('settings.autoStartTgProxy')}><Switch checked={!!config.auto_start_proxy} onChange={() => update({ auto_start_proxy: !config.auto_start_proxy })} /></MiniRow>

          <div className="set-notif-sub">
            <span className="section-title">{t('settings.powerSaving')}</span>
            <MiniRow label={t('settings.powerSavingEnable')}>
              <Switch checked={config.power_saving || false} onChange={() => update({ power_saving: !config.power_saving })} />
            </MiniRow>
            {/* Ползунки показываем только при включённом режиме —
                выключенное состояние «серым» вводит в заблуждение */}
            {config.power_saving && (
              <div className="pw-slider-row">
                <span className="pw-slider-label">{t('settings.powerLevel')}</span>
                <input type="range" min="1" max="3" step="1"
                  value={config.power_level || 1}
                  onChange={(e) => update({ power_level: +e.target.value })}
                  className="pw-slider slider-ui"
                  style={{ '--val': rangePct(config.power_level || 1, 1, 3) + '%' }} />
                <span className="pw-slider-val">{t(POWER_LEVEL_KEYS[(config.power_level || 1) - 1])}</span>
              </div>
            )}
            {config.power_saving && status?.power_battery?.has && (
              <div className="pw-slider-row">
                <span className="pw-slider-label">{t('settings.powerThreshold')}</span>
                <input type="range" min="0" max={POWER_THRESHOLDS.length - 1} step="1"
                  value={POWER_THRESHOLDS.indexOf(config.power_threshold ?? 0)}
                  onChange={(e) => update({ power_threshold: POWER_THRESHOLDS[+e.target.value] })}
                  className="pw-slider slider-ui"
                  style={{ '--val': rangePct(POWER_THRESHOLDS.indexOf(config.power_threshold ?? 0), 0, POWER_THRESHOLDS.length - 1) + '%' }} />
                <span className="pw-slider-val">{thresholdLabel(config.power_threshold ?? 0, t)}</span>
              </div>
            )}
          </div>

          <div className="set-row set-row-bottom" style={{ padding: '3px 0', marginTop: 'auto' }}>
            <div className="set-row-info">
              <span className="set-row-title">{t('settings.checkInterval')}</span>
              <span className="set-row-desc">{t('settings.checkIntervalDesc')}</span>
            </div>
            <div className="set-interval-row">
              {[5, 10, 15, 30].map(m => (
                <button key={m} className={'btn btn-sm' + ((config.resource_check_interval || 10) === m ? ' active' : '')}
                  onClick={() => update({ resource_check_interval: m })}>{m}m</button>
              ))}
            </div>
          </div>
        </div>

        <div className="set-side-col">
        <div className="section">
          <div className="section-title set-section-title">
            <span>{t('settings.updates')}</span>
            <span className="inline-row" style={{ gap: 8 }}>
              <span className="ms muted">{t('settings.updSource')}</span>
              <Select
                className="upd-source-select"
                align="right"
                items={['auto', 'server', 'yandex', 'github', 'ghproxy'].map((id) => ({ id, label: t('settings.updSrc.' + id) }))}
                value={updSource || 'auto'}
                onChange={changeUpdSource}
                ariaLabel={t('settings.updSource')}
              />
            </span>
          </div>

          <SimpleUpdateCard
            name="ZPUI"
            current={verPanel?.current || versions?.zpui}
            latest={verPanel?.latest || zpuiCheck.latest}
            checkState={zpuiCheck.state}
            updateNeeded={zpuiCheck.state === 'available' || verPanel?.avail === true}
            downloaded={flow.component === 'ZPUI' && flow.phase === 'downloaded'}
            onStartUpdate={() => startUpdate('ZPUI')}
            onShowRelease={(ver) => setReleaseVer(ver)}
            t={t}
          />

          <SimpleUpdateCard
            name="ZAPRET"
            current={zapretVerPanel?.current || status?.zapret?.version || zapretCheck.current}
            latest={zapretVerPanel?.latest || zapretCheck.latest}
            checkState={zapretCheck.state}
            updateNeeded={zapretCheck.state === 'available' || zapretVerPanel?.avail === true}
            downloaded={flow.component === 'zapret' && flow.phase === 'downloaded'}
            onStartUpdate={() => startUpdate('zapret')}
            onShowRelease={(ver) => setZapretReleaseVer(ver)}
            t={t}
          />

          <ExtensionUpdateCard
            info={extInfo}
            busy={extUpdating}
            onStart={updateExtension}
            t={t}
          />

        </div>

        <ZsvcPanel />

        <div className="section zpset-section">
          <div className="zpset-head">
            <span className="zpset-head-title">ZAPRET</span>
            <span className={'zpset-pill ' + (status?.zapret?.status === 'running' && !config.zapret_skipped ? 'on' : 'off')}>
              {config.zapret_skipped ? t('zapret.skippedStatus') : status?.zapret?.status === 'running' ? t('settings.svcRunning') : t('status.stopped')}
              {status?.zapret?.version && (
                <span className="zpset-pill-ver">v{status.zapret.version}</span>
              )}
            </span>
          </div>

          <div className="zpset-svc-row">
            <div className="zpset-svc-label">
              <Power size={13} strokeWidth={2} />
              <span>{svcInstalled ? t('settings.serviceMode') : t('settings.processMode')}</span>
            </div>
            {svcInstalled && (
              <label className="zpset-svc-auto" onClick={(e) => e.preventDefault()}>
                <span className="zpset-toggle-label">{t('settings.svcAutostart')}</span>
                <Switch
                  checked={svcAuto === true}
                  loading={svcAutoBusy}
                  onChange={async () => {
                    if (svcAutoBusy) return;
                    const next = !(svcAuto === true);
                    setSvcAuto(next);
                    setSvcAutoBusy(true);
                    const r = await api('POST', '/api/zapret/service-autostart', { enabled: next });
                    setSvcAutoBusy(false);
                    if (!r || r.error) {
                      setSvcAuto(!next);
                      showToast(r?.error || t('settings.svcAutostartFailed'), 'error');
                    } else {
                      showToast(next ? t('settings.svcAutostartOn') : t('settings.svcAutostartOff'), 'success');
                    }
                  }}
                />
              </label>
            )}
          </div>

          <div className="zpset-actions">
            {svcInstalled ? (
              <button className="zpset-action danger" disabled={reinstalling} onClick={async () => {
                if (!await confirm({ message: t('settings.removeServiceConfirm'), variant: 'danger', confirmText: t('settings.removeServiceBtn') })) return;
                setReinstalling(true);
                const ok = await apiCall(async () => api('POST', '/api/zapret/service/remove'), t('settings.serviceRemoved'), showToast);
                if (ok) setSvcOpt(false);
                setReinstalling(false);
              }}>
                {reinstalling ? <Spinner size={13} /> : <Trash2 size={13} strokeWidth={2} />}
                <span>{t('settings.removeServiceBtn')}</span>
              </button>
            ) : (
              <button className="zpset-action accent" disabled={reinstalling} onClick={async () => {
                setReinstalling(true);
                const strategy = config.current_strategy || '';
                const result = await api('POST', '/api/zapret/service/install', { strategy });
                if (result?.error) {
                  showToast(result.error, 'error');
                } else {
                  showToast(t('settings.serviceInstalled'), 'success');
                  setSvcOpt(true);
                }
                setReinstalling(false);
              }}>
                {reinstalling ? <Spinner size={13} /> : <Download size={13} strokeWidth={2} />}
                <span>{t('settings.installServiceBtn')}</span>
              </button>
            )}
            <button className="zpset-action" disabled={reinstalling} onClick={async () => {
              if (!await confirm({ message: t('settings.reinstallServiceConfirm'), variant: 'danger', confirmText: t('settings.reinstallServiceBtn') })) return;
              setReinstalling(true);
              await apiCall(async () => api('POST', '/api/zapret/stop'), null, showToast);
              await apiCall(async () => api('POST', '/api/zapret/service/remove'), null, showToast);
              await new Promise(r => setTimeout(r, 1000));
              const result = await api('POST', '/api/zapret/start');
              if (result?.error) {
                showToast(result.error, 'error');
              } else {
                showToast(t('settings.serviceReinstalled'), 'success');
              }
              setReinstalling(false);
              setSvcOpt(false);
            }}>
              {reinstalling ? <Spinner size={13} /> : <RefreshCw size={13} strokeWidth={2} />}
              <span>{t('settings.reinstallServiceBtn')}</span>
            </button>
            <button className="zpset-action" disabled={fullReinstalling}
              onClick={async () => {
                if (!await confirm({ message: t('settings.fullReinstallConfirm'), variant: 'danger', confirmText: t('settings.fullReinstallBtn') })) return;
                setFullReinstalling(true);
                showToast(t('settings.fullReinstallStarted'), 'info');
                const result = await api('POST', '/api/zapret/full-reinstall');
                setFullReinstalling(false);
                if (result?.error) {
                  showToast(result.error, 'error');
                } else {
                  showToast(t('settings.fullReinstallComplete'), 'success');
                }
                loadVerPanel();
                loadZapretVerPanel();
                loadVersions();
              }}>
              {fullReinstalling ? <Spinner size={13} /> : <AlertTriangle size={13} strokeWidth={2} />}
              <span>{t('settings.fullReinstallBtn')}</span>
            </button>
          </div>

          {config.zapret_skipped && (
            <div className="zpset-install-prompt">
              <div className="zpset-install-info">
                <Download size={14} strokeWidth={2.2} className="zpset-install-icon" />
                <div>
                  <span className="zpset-install-title">{t('settings.installZapret')}</span>
                  <span className="zpset-install-desc">{t('settings.installZapretDesc')}</span>
                </div>
              </div>
              <button className="btn btn-accent btn-sm" onClick={async () => {
                if (await confirm({ message: t('settings.installZapretConfirm'), confirmText: t('settings.installZapretBtn') })) {
                  api('POST', '/api/app/restart');
                }
              }}>
                {t('settings.installZapretBtn')}
              </button>
            </div>
          )}
        </div>
        </div>
      </div>
      )}

      {settingsTab === 'data' && <SyncDataTab />}

      <div className={'te-tab-scroll' + (settingsTab === 'theme' ? '' : ' off')} ref={teScrollRef}>
        {settingsTab === 'theme' && (
        <>
          {teSlot ? (
            <ThemeEditorTab
              active={true}
              slot={teSlot}
              isActive={editorSlotActive}
              onDone={closeThemeEditor}
              onBack={() => setTeSlot(null)}
              onSlotSaved={handleSlotSaved}
            />
          ) : (
            <div className="section te-tab-section tg-section">
              <div className="section-title te-tab-title">
                <Layers size={14} strokeWidth={2} />
                {t('themeGallery.pickToEdit')}
              </div>
              <ThemeGallery
                mode="edit"
                themes={themes}
                activeId={activeCustomId}
                onApply={applySlot}
                onEdit={editSlot}
                onExport={exportSlot}
                onDelete={deleteSlot}
                onCreate={createSlot}
                onImportFile={importSlotFile}
              />
            </div>
          )}
        </>
        )}
      </div>

      <Modal
        open={galleryOpen}
        onClose={() => setGalleryOpen(false)}
        title={t('themeGallery.title')}
        icon={<Palette size={18} strokeWidth={2} />}
        tone="accent"
        className="tg-modal"
      >
        {/* Модалка «Своя» — только ВЫБОР темы: управление (создание/импорт/
            редактирование/удаление) живёт на под-вкладке «Тема» */}
        <ThemeGallery
          mode="apply"
          themes={themes}
          activeId={activeCustomId}
          onApply={applySlot}
        />
      </Modal>

      <ReleaseModal
        open={releaseVer !== null}
        onClose={() => setReleaseVer(null)}
        currentVersion={zpuiCheck.current || versions?.zpui}
        latestVersion={zpuiCheck.latest}
        version={releaseVer || ''}
        target="zpui"
      />
      <ReleaseModal
        open={zapretReleaseVer !== null}
        onClose={() => setZapretReleaseVer(null)}
        currentVersion={zapretVerPanel?.current || zapretCheck.current}
        latestVersion={zapretVerPanel?.latest || zapretCheck.latest}
        version={zapretReleaseVer || ''}
        target="zapret"
      />
    </div>
  );
}

function MiniRow({ label, children }) {
  return (
    <div className="set-row" style={{ padding: '3px 0' }}>
      <div className="set-row-info"><span className="set-row-title">{label}</span></div>
      {children}
    </div>
  );
}

// SimpleUpdateCard — карточка версии: клик по версии = «что нового»,
// при наличии обновления — кнопка «Скачать» (открывает модалку обновления),
// если архив уже скачан — кнопка «Установить».
// recommendedVer — версия, которую рекомендует сервер (канал ZPUI Sync).
function SimpleUpdateCard({ name, current, latest, checkState, updateNeeded, downloaded, busy, busyLabel, onStartUpdate, onShowRelease, recommendedVer, t }) {
  const fmtVer = (v) => v ? (v.startsWith('v') ? v : 'v' + v) : '—';
  const hasLatest = latest && latest !== current;
  const verChip = (className, ver) => onShowRelease ? (
    <button type="button" className={`upd-ver-btn ${className}`} data-tooltip={t('settings.whatsNewTip')} data-tooltip-pos="bottom" onClick={() => onShowRelease(ver)}>{fmtVer(ver)}</button>
  ) : (
    <span className={className}>{fmtVer(ver)}</span>
  );
  const isChecking = checkState === 'checking';
  const rec = recommendedVer && recommendedVer !== current ? fmtVer(recommendedVer) : '';

  return (
    <div className="upd-card">
      <div className="upd-info">
        <div className="upd-simple-row">
          <span className="upd-name">{name}</span>
          {verChip('upd-cur-ver', current)}
          {hasLatest && updateNeeded && (
            <>
              <ArrowRight size={11} strokeWidth={2.5} style={{ color: 'var(--text-tertiary)', flexShrink: 0 }} />
              {verChip('upd-new-ver', latest)}
            </>
          )}
        </div>
        {rec && <span className="upd-rec-ver">{t('settings.serverRecommends', { version: rec })}</span>}
      </div>
      <div className="upd-actions">
        {isChecking ? (
          <span className="upd-status checking">
            <Spinner size={10} className="spin-inline" />
            {t('settings.checkingUpdates')}
          </span>
        ) : busy ? (
          <span className="upd-status checking">
            <Spinner size={10} className="spin-inline" />
            {busyLabel}
          </span>
        ) : downloaded ? (
          <button className="btn btn-sm upd-btn-check" onClick={onStartUpdate}>
            <HardDriveDownload size={11} strokeWidth={2.2} />
            {t('update.install')}
          </button>
        ) : updateNeeded ? (
          <button className="btn btn-sm upd-btn-check" onClick={onStartUpdate}>
            <Download size={11} strokeWidth={2.2} />
            {t('common.download')}
          </button>
        ) : (
          <span className="upd-status latest">{t('status.upToDate')}</span>
        )}
      </div>
    </div>
  );
}

// ExtensionUpdateCard — ряд «Плагин браузера» в панели обновлений: версия
// пакета, пилюля «актуально», кнопка «Обновить»/«Установить» (POST
// /api/extension/update — установщик с сервера ставится тихо в фоне),
// приглушённая подсказка при недоступном сервере. Раскладка и стили —
// те же, что у карточек ZPUI/ZAPRET выше (upd-card).
function ExtensionUpdateCard({ info, busy, onStart, t }) {
  const fmtVer = (v) => (v ? (v.startsWith('v') ? v : 'v' + v) : '—');
  const installed = !!(info && info.version);
  const updateReady = !!(info && info.update_ready);
  const serverOffline = !!(info && info.server_error && !info.latest);

  return (
    <div className="upd-card">
      <div className="upd-info">
        <div className="upd-simple-row">
          <span className="upd-name">{t('extension.title')}</span>
          {installed && <span className="upd-cur-ver">{fmtVer(info.version)}</span>}
          {updateReady && installed && info.latest && (
            <>
              <ArrowRight size={11} strokeWidth={2.5} style={{ color: 'var(--text-tertiary)', flexShrink: 0 }} />
              <span className="upd-new-ver">{fmtVer(info.latest)}</span>
            </>
          )}
        </div>
      </div>
      <div className="upd-actions">
        {!info ? (
          <span className="upd-status checking">
            <Spinner size={10} className="spin-inline" />
            {t('settings.checkingUpdates')}
          </span>
        ) : busy ? (
          <span className="upd-status checking">
            <Spinner size={10} className="spin-inline" />
            {t('extension.updating')}
          </span>
        ) : updateReady ? (
          <button type="button" className="btn btn-sm upd-btn-check" onClick={onStart}>
            {installed
              ? <Download size={11} strokeWidth={2.2} />
              : <HardDriveDownload size={11} strokeWidth={2.2} />}
            {installed ? t('extension.update') : t('extension.install')}
          </button>
        ) : installed ? (
          <span className="upd-status latest">{t('status.upToDate')}</span>
        ) : (
          <span className="upd-status none">
            {serverOffline ? t('settings.ext.serverOffline') : t('settings.ext.notInstalled')}
          </span>
        )}
      </div>
    </div>
  );
}

