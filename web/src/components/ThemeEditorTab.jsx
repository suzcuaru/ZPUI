import { useState, useEffect, useRef } from 'react';
import Switch from './ui/Switch';
import Spinner from './ui/Spinner';
import { useConfirm } from './ui/ConfirmDialog';
import { api } from '../api';
import { useT } from '../i18n';
import { useToast } from '../context/ToastContext';
import { rangePct } from '../utils';
import { exportThemeZip, importThemeZip, downloadBlob, zipNameFor, looksLikeZip, blobToText } from '../themeZip';
import {
  defaultTheme, normalizeTheme, cloneTheme, themeEquals,
  applyCustomTheme, removeCustomTheme, resolveThemeBgUrl, normalizeHexInput, BG_FITS,
} from '../themeCustom';
import {
  Palette, RotateCcw, Upload, Download, Image as ImageIcon, X, Plus, Check, Paintbrush,
  PaintBucket, Type, Minus, Sparkles, Eye, BadgeCheck,
} from 'lucide-react';

// Группы цветов внутри карточки «Палитра»: [ключ, ключ i18n].
const COLOR_GROUPS = [
  {
    title: 'themeEditor.secAccent',
    icon: Palette,
    fields: [
      ['accent', 'themeEditor.accent'],
      ['success', 'themeEditor.success'],
      ['warning', 'themeEditor.warning'],
      ['danger', 'themeEditor.danger'],
    ],
  },
  {
    title: 'themeEditor.secBg',
    icon: PaintBucket,
    fields: [
      ['bgPage', 'themeEditor.bgPage'],
      ['bgCard', 'themeEditor.bgCard'],
      ['bgInset', 'themeEditor.bgInset'],
    ],
    lockedByImage: true,
  },
  {
    title: 'themeEditor.secText',
    icon: Type,
    fields: [
      ['textPrimary', 'themeEditor.textPrimary'],
      ['textSecondary', 'themeEditor.textSecondary'],
      ['textTertiary', 'themeEditor.textTertiary'],
    ],
  },
  {
    title: 'themeEditor.secBorders',
    icon: Minus,
    fields: [
      ['border', 'themeEditor.border'],
      ['borderLight', 'themeEditor.borderLight'],
    ],
  },
];

// BgRibbon — лента картинок из отдельной папки (в продукте — рядом с
// приложением). Список/загрузка/удаление — /api/bg/*; в ленте хранятся и
// выбираются ПУТИ (/bg/<file>), тема не содержит код картинки.
function BgRibbon({ selected, onSelect }) {
  const showToast = useToast();
  const { t } = useT();
  const confirm = useConfirm();
  const [files, setFiles] = useState(null);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef(null);

  const load = async () => {
    try {
      const r = await fetch('/api/bg/list');
      const d = await r.json();
      setFiles(Array.isArray(d.files) ? d.files : []);
    } catch {
      setFiles([]);
    }
  };

  useEffect(() => { load(); }, []);

  const upload = async (e) => {
    const file = e.target.files && e.target.files[0];
    e.target.value = '';
    if (!file) return;
    setBusy(true);
    try {
      const r = await fetch('/api/bg/upload', {
        method: 'POST',
        headers: { 'Content-Type': 'application/octet-stream', 'X-Filename': encodeURIComponent(file.name) },
        body: file,
      });
      const d = await r.json().catch(() => null);
      if (!d || d.error || !d.path) {
        showToast(t('themeEditor.bgUploadFail'), 'error');
        return;
      }
      await load();
      onSelect(d.path);
      showToast(t('themeEditor.bgUploadOk'), 'success');
    } catch {
      showToast(t('themeEditor.bgUploadFail'), 'error');
    } finally {
      setBusy(false);
    }
  };

  const remove = async (file) => {
    if (!await confirm({
      message: t('themeEditor.bgDeleteConfirm', { name: file.name }),
      variant: 'danger',
      confirmText: t('themeEditor.bgDelete'),
    })) return;
    try {
      const r = await fetch('/api/bg/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: file.name }),
      });
      const d = await r.json().catch(() => null);
      if (!d || d.error) {
        showToast(t('themeEditor.bgDeleteFail'), 'error');
        return;
      }
      if (selected === file.path) onSelect('');
      await load();
    } catch {
      showToast(t('themeEditor.bgDeleteFail'), 'error');
    }
  };

  return (
    <div className="bg-ribbon-wrap">
      <div className="bg-ribbon" role="listbox" aria-label={t('themeEditor.bgRibbon')}>
        <button
          className="bg-tile bg-tile-add"
          onClick={() => fileRef.current && fileRef.current.click()}
          disabled={busy}
          aria-label={t('themeEditor.bgUpload')}
        >
          {busy ? <Spinner size={14} /> : <Plus size={15} strokeWidth={2} />}
          <span>{t('themeEditor.bgUpload')}</span>
        </button>
        {(files || []).map((f) => (
          <div
            key={f.name}
            className={'bg-tile' + (f.path === selected ? ' selected' : '')}
            role="option"
            aria-selected={f.path === selected}
            aria-label={f.name}
            tabIndex={0}
            onClick={() => onSelect(f.path)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onSelect(f.path); }
            }}
          >
            <img src={f.path} alt="" loading="lazy" />
            <button
              className="bg-tile-del"
              aria-label={t('themeEditor.bgDelete') + ' ' + f.name}
              onClick={(e) => { e.stopPropagation(); remove(f); }}
            >
              <X size={11} strokeWidth={2.4} />
            </button>
          </div>
        ))}
      </div>
      {files && files.length === 0 && (
        <div className="bg-empty-note">{t('themeEditor.bgEmpty')}</div>
      )}
      <input ref={fileRef} type="file" accept="image/png,image/jpeg,image/webp,image/gif" hidden onChange={upload} />
    </div>
  );
}

// ThemePreview — живая панель-пример: собрана из реальных контролов стенда и
// красится исключительно CSS-переменными текущего черновика (они применяются
// к :root[data-theme="custom"] тем же движком, что и весь стенд) — любое
// изменение в секциях слева видно здесь мгновенно. Кнопки/тумблер/ползунок
// интерактивны (состояние локальное), data-sound="off" не спамит кликами.
function ThemePreview() {
  const { t } = useT();
  const [on, setOn] = useState(true);
  const [val, setVal] = useState(60);
  const [txt, setTxt] = useState('');
  return (
    <div className="te-pv" role="group" aria-label={t('themeEditor.pvTitle')}>
      <div className="te-pv-bar" aria-hidden="true">
        <span className="te-pv-dot" style={{ background: 'var(--accent)' }} />
        <span className="te-pv-dot" style={{ background: 'var(--success)' }} />
        <span className="te-pv-dot" style={{ background: 'var(--danger)' }} />
      </div>
      <div className="te-pv-panel">
        <div className="te-pv-heading">{t('themeEditor.pvHeading')}</div>
        <div className="te-pv-line">{t('themeEditor.pvLine')}</div>
        <div className="te-pv-line t2">{t('themeEditor.pvLine2')}</div>
        <div className="te-pv-line t3">{t('themeEditor.pvLine3')}</div>
        <div className="te-pv-row">
          <button type="button" className="btn btn-sm" data-sound="off">{t('themeEditor.pvBtn')}</button>
          <button type="button" className="btn btn-accent btn-sm" data-sound="off">{t('themeEditor.pvBtnAccent')}</button>
        </div>
        <div className="te-pv-row">
          <Switch checked={on} hideState onChange={() => setOn((v) => !v)} />
          <input
            type="range" min="0" max="100" step="5"
            className="te-slider slider-ui te-pv-range"
            value={val}
            aria-label={t('themeEditor.pvSlider')}
            style={{ '--val': rangePct(val, 0, 100) + '%' }}
            onChange={(e) => setVal(+e.target.value)}
          />
        </div>
        <input
          className="form-input te-pv-input"
          value={txt}
          placeholder={t('themeEditor.pvInputPh')}
          aria-label={t('themeEditor.pvInputPh')}
          spellCheck="false"
          onChange={(e) => setTxt(e.target.value)}
        />
        <div className="te-pv-row">
          <span className="badge badge-success">{t('themeEditor.pvOk')}</span>
          <span className="badge badge-danger">{t('themeEditor.pvErr')}</span>
        </div>
      </div>
      <div className="te-pv-swrow" aria-hidden="true">
        <span className="te-pv-sw" style={{ background: 'var(--bg-card)' }} />
        <span className="te-pv-sw" style={{ background: 'var(--bg-inset)' }} />
        <span className="te-pv-sw te-pv-sw-rad" />
        <span className="te-pv-sw-cap">{t('themeEditor.pvSurfaces')}</span>
      </div>
    </div>
  );
}

// ThemeEditorTab — редактор КОНКРЕТНОГО слота пользовательской темы (id+name),
// под-вкладка Настроек. Живой предпросмотр: черновик сразу применяется к
// стенду (style-тег + data-theme="custom"), панель «Предпросмотр» справа
// показывает все изменения на реальных контролах. Уход со вкладки / смена
// слота: изменённый черновик коммитится в свой слот и активируется,
// неизменённый — тихо возвращает входное состояние.
// Вёрстка: одна колонка карточек — «Палитра» (база+контраст+цвета),
// «Фоновая картинка», «Эффекты»; справа sticky-предпросмотр.
// slot: {id, name}; id === null — новый слот (создаётся при первом сохранении).
// isActive — слот сейчас активен в системе; onSlotSaved(id, name) — после
// первого сохранения нового слота (родитель перепривяжет редактор к слоту).
export default function ThemeEditorTab({ active, slot, isActive, onDone, onBack, onSlotSaved }) {
  const showToast = useToast();
  const { t } = useT();
  const [draft, setDraft] = useState(null);
  const [name, setName] = useState('');
  const entryRef = useRef(null);
  const savedRef = useRef({ applied: false, obj: null });
  const importRef = useRef(null);
  const draftRef = useRef(null);
  const nameRef = useRef('');
  const loadedIdRef = useRef(undefined); // слот, чей черновик загружен
  const committedIdRef = useRef(null);  // id слота после первого сохранения нового

  const slotId = slot ? slot.id : null;
  const effSlotId = () => (committedIdRef.current != null ? committedIdRef.current : slotId);

  useEffect(() => { draftRef.current = draft; nameRef.current = name; }, [draft, name]);

  // Открытие вкладки / смена слота: снимок состояния (активна ли custom, что
  // сохранено в «БД») и стартовый черновик слота. Легаси-путь картинки
  // резолвится по /api/bg/list ещё ДО предпросмотра — картинка видна сразу.
  useEffect(() => {
    if (!active) return;
    if (committedIdRef.current != null && slotId === committedIdRef.current) {
      loadedIdRef.current = slotId;
      return;
    }
    if (draft && loadedIdRef.current === slotId) return;
    let alive = true;
    loadedIdRef.current = slotId;
    setDraft(null);
    (async () => {
      const r = await api('GET', '/api/custom-themes');
      if (!alive) return;
      const list = r && Array.isArray(r.themes) ? r.themes : [];
      const found = slotId != null ? list.find((s) => s.id === slotId) : null;
      const attr = document.documentElement.getAttribute('data-theme');
      const saved = found && found.theme && typeof found.theme === 'object' ? normalizeTheme(found.theme) : null;
      const base = saved ? saved.base : (attr === 'light' ? 'light' : 'dark');
      const initial = await resolveThemeBgUrl(saved || defaultTheme(base));
      if (!alive) return;
      const initName = found && typeof found.name === 'string'
        ? found.name
        : (slot && typeof slot.name === 'string' ? slot.name : '');
      entryRef.current = { theme: cloneTheme(initial), name: initName, attr: attr === 'custom' ? 'dark' : attr };
      const activeSlotTheme = r && r.activeId && list.find((s) => s.id === r.activeId);
      savedRef.current = {
        applied: attr === 'custom',
        obj: activeSlotTheme && activeSlotTheme.theme ? normalizeTheme(activeSlotTheme.theme) : null,
      };
      committedIdRef.current = null;
      setDraft(initial);
      setName(initName);
    })();
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, slotId]);

  // Живой предпросмотр: черновик мгновенно красит стенд.
  useEffect(() => {
    if (!active || !draft) return;
    applyCustomTheme(draft);
    document.documentElement.setAttribute('data-theme', 'custom');
  }, [active, draft]);

  // Уход со вкладки / смена слота: применённый черновик остаётся, изменённый
  // коммитится в свой слот (и активируется), неизменённый — тихий возврат
  // входного состояния.
  useEffect(() => {
    if (!active) return;
    return () => {
      const entry = entryRef.current;
      entryRef.current = null;
      const d = draftRef.current;
      const n = nameRef.current;
      setDraft(null);
      setName('');
      if (!entry || !d) return;
      // Новый слот коммитится и при уходе без «Применить» — иначе «Создать
      // тему» терялась при простом переключении вкладки.
      if (effSlotId() != null && themeEquals(d, entry.theme) && (n || '') === (entry.name || '')) {
        const st = savedRef.current;
        if (st.applied && st.obj) {
          applyCustomTheme(st.obj);
          document.documentElement.setAttribute('data-theme', 'custom');
        } else {
          removeCustomTheme();
          document.documentElement.setAttribute('data-theme', entry.attr || 'dark');
        }
        return;
      }
      (async () => {
        const resolved = await resolveThemeBgUrl(d);
        const r = await api('POST', '/api/custom-themes', { id: effSlotId() || undefined, name: n, theme: resolved, activate: true });
        if (r && r.error) return;
        window.dispatchEvent(new CustomEvent('zpui:theme-changed', { detail: { theme: 'custom' } }));
        window.dispatchEvent(new CustomEvent('zpui:themes-changed'));
      })();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, slotId]);

  if (!active) return null;

  const hasImage = !!draft?.bgImage?.url;
  const setColor = (key, hex) => setDraft((d) => ({ ...d, colors: { ...d.colors, [key]: hex } }));
  const patchBg = (fields) => setDraft((d) => ({ ...d, bgImage: { ...d.bgImage, ...fields } }));
  const patchFx = (fields) => setDraft((d) => ({ ...d, effects: { ...d.effects, ...fields } }));
  const patch = (fn) => setDraft((d) => fn(d));

  const handleBase = (base) => {
    if (!draft || draft.base === base) return;
    patch((d) => ({ ...defaultTheme(base), bgImage: d.bgImage, effects: d.effects, typography: d.typography }));
  };

  // Выбор картинки (клик по ленте / после загрузки / очистка): в тему пишется
  // путь; при первой картинке — мягкие дефолты прозрачности/затемнения.
  const handleSelectImage = (path) => {
    setDraft((d) => {
      if (!path) return { ...d, bgImage: { ...d.bgImage, url: '' } };
      const fresh = !d.bgImage.url;
      return {
        ...d,
        bgImage: {
          ...d.bgImage,
          url: path,
          ...(fresh ? {
            panelOpacity: Math.min(d.bgImage.panelOpacity, 85),
            shellOpacity: Math.min(d.bgImage.shellOpacity, 85),
            blur: Math.max(d.bgImage.blur, 6),
            dim: Math.max(d.bgImage.dim, 25),
          } : {}),
        },
      };
    });
  };

  const handleReset = () => {
    if (!entryRef.current) return;
    setDraft(cloneTheme(entryRef.current.theme));
    setName(entryRef.current.name);
  };

  // Сохранение изменённого черновика в слот с активацией. Возвращает id слота
  // или null (не изменился/ошибка). Обновляет снимок входного состояния.
  const commitChangedDraft = async () => {
    const entry = entryRef.current;
    const d = draftRef.current;
    const n = nameRef.current;
    if (!d || !entry) return null;
    // Новый слот (id ещё нет) сохраняем всегда: «Создать тему» обязана
    // оставить тему в галерее, даже если пользователь ничего не трогал.
    if (effSlotId() != null && themeEquals(d, entry.theme) && (n || '') === (entry.name || '')) return null;
    const resolved = await resolveThemeBgUrl(d);
    const r = await api('POST', '/api/custom-themes', { id: effSlotId() || undefined, name: n, theme: resolved, activate: true });
    if (r && r.error) return null;
    if (r && r.id) committedIdRef.current = r.id;
    entryRef.current = { theme: cloneTheme(resolved), name: n, attr: 'custom' };
    savedRef.current = { applied: true, obj: cloneTheme(resolved) };
    window.dispatchEvent(new CustomEvent('zpui:theme-changed', { detail: { theme: 'custom' } }));
    window.dispatchEvent(new CustomEvent('zpui:themes-changed'));
    return committedIdRef.current;
  };

  // «Сделать активной»: сохранить черновик (если менялся) и включить тему
  // сейчас, не закрывая редактор.
  const handleMakeActive = async () => {
    const saved = await commitChangedDraft();
    if (saved == null) {
      const id = effSlotId();
      if (id == null) return;
      const r = await api('POST', '/api/custom-themes/activate', { id });
      if (r && r.error) return;
      window.dispatchEvent(new CustomEvent('zpui:theme-changed', { detail: { theme: 'custom' } }));
      window.dispatchEvent(new CustomEvent('zpui:themes-changed'));
    }
    if (onSlotSaved && committedIdRef.current != null) onSlotSaved(committedIdRef.current, nameRef.current);
  };

  // «Применить и закрыть»: изменённый черновик коммитится сразу (не через
  // эффект ухода) в СВОЙ слот с активацией, чтобы родитель после onDone
  // перечитал уже новое состояние.
  const handleApply = async () => {
    await commitChangedDraft();
    if (onDone) onDone();
  };

  // Экспорт: черновик + файлы звуковой схемы + фоновая картинка → ZIP.
  const handleExport = async () => {
    if (!draft) return;
    try {
      const blob = await exportThemeZip({ name, theme: draft });
      downloadBlob(blob, zipNameFor(name));
      showToast(t('themeEditor.exportOk'), 'success');
    } catch {
      showToast(t('themeEditor.exportFail'), 'error');
    }
  };

  // Импорт: ZIP (theme.json + фон + схема звуков) или легаси .json → draft
  // (live-предпросмотр подхватит сам). Битый файл — тост ошибки.
  const handleImportFile = async (e) => {
    const file = e.target.files && e.target.files[0];
    e.target.value = '';
    if (!file) return;
    try {
      if (await looksLikeZip(file)) {
        const { theme } = await importThemeZip(file);
        setDraft(theme);
        showToast(t('themeEditor.importOk'), 'success');
        return;
      }
      const parsed = JSON.parse(await blobToText(file));
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('bad');
      const resolved = await resolveThemeBgUrl(normalizeTheme(parsed));
      setDraft(resolved);
      showToast(t('themeEditor.importOk'), 'success');
    } catch {
      showToast(t('themeEditor.importFail'), 'error');
    }
  };

  const colorGroup = (group) => {
    const locked = !!group.lockedByImage && hasImage;
    const GIcon = group.icon;
    return (
      <div className="te-group" key={group.title}>
        <div className="te-group-title">
          {GIcon && <GIcon size={11} strokeWidth={2} />}
          {t(group.title)}
          {locked && (
            <span className="te-lock">
              <ImageIcon size={11} strokeWidth={2} />
            </span>
          )}
        </div>
        {group.fields.map(([key, label]) => (
          <ColorField
            key={key}
            label={t(label)}
            locked={locked}
            value={draft.colors[key]}
            onChange={(hex) => setColor(key, hex)}
          />
        ))}
      </div>
    );
  };

  return (
    <div className="te-tab">
      <div className="section te-tab-section">
        <div className="te-head">
          <div className="te-head-id">
            <span className="te-head-label">
              {onBack && (
                <button className="te-back" onClick={onBack} aria-label={t('themeEditor.backToGallery')}>
                  <X size={14} strokeWidth={2.2} />
                </button>
              )}
              <Palette size={13} strokeWidth={2} />
              {t('themeEditor.title')}
            </span>
            <div className="te-head-row">
              <input
                className="form-input te-name"
                value={name}
                placeholder={t('themeEditor.namePh')}
                spellCheck="false"
                maxLength={40}
                aria-label={t('themeEditor.name')}
                disabled={!draft}
                onChange={(e) => setName(e.target.value)}
              />
              <span className={'te-status' + (isActive ? ' on' : '')}>
                {isActive && <Check size={10} strokeWidth={3} />}
                {isActive ? t('themeEditor.statusActive') : t('themeEditor.statusDraft')}
              </span>
            </div>
          </div>
          <div className="te-tab-actions">
            <button className="btn btn-sm" onClick={handleReset} disabled={!draft} aria-label={t('themeEditor.reset')}>
              <RotateCcw size={12} strokeWidth={2} />
              {t('themeEditor.reset')}
            </button>
            <button className="btn btn-sm" onClick={() => importRef.current && importRef.current.click()} disabled={!draft}>
              <Upload size={12} strokeWidth={2} />
              {t('themeEditor.import')}
            </button>
            <button className="btn btn-sm" onClick={handleExport} disabled={!draft}>
              <Download size={12} strokeWidth={2} />
              {t('themeEditor.export')}
            </button>
            {!isActive && (
              <button className="btn btn-sm te-activate" onClick={handleMakeActive} disabled={!draft}>
                <BadgeCheck size={12} strokeWidth={2} />
                {t('themeEditor.makeActive')}
              </button>
            )}
            <button className="btn btn-accent btn-sm" onClick={handleApply} disabled={!draft}>
              <Check size={12} strokeWidth={2.4} />
              {t('themeEditor.apply')}
            </button>
            <input ref={importRef} type="file" accept=".zip,.json,application/zip,application/json" hidden onChange={handleImportFile} />
          </div>
        </div>

        {!draft ? (
          <div className="modal-loading"><Spinner size={14} />{t('common.loading')}</div>
        ) : (
        <div className="te-tab-grid">
          <div className="te-tab-main">

            {/* ── Палитра: база + контраст + все цвета одной карточкой ── */}
            <div className="te-card">
              <div className="te-sec-title">
                <Paintbrush size={12} strokeWidth={2} />
                {t('themeEditor.palette')}
              </div>
              <div className="te-base-row">
                <span className="te-slider-label">{t('themeEditor.base')}</span>
                <div className="te-fit-btns">
                  <button className={'btn btn-sm te-fit-btn' + (draft.base === 'light' ? ' active' : '')} onClick={() => handleBase('light')}>{t('settings.lightTheme')}</button>
                  <button className={'btn btn-sm te-fit-btn' + (draft.base === 'dark' ? ' active' : '')} onClick={() => handleBase('dark')}>{t('settings.darkTheme')}</button>
                </div>
                <input
                  type="range" min="50" max="150" step="5"
                  className="te-slider slider-ui"
                  value={draft.contrast}
                  aria-label={t('themeEditor.contrast')}
                  style={{ '--val': rangePct(draft.contrast, 50, 150) + '%' }}
                  onChange={(e) => patch((d) => ({ ...d, contrast: +e.target.value }))}
                />
                <span className="te-slider-val">{draft.contrast}%</span>
              </div>
              {COLOR_GROUPS.map(colorGroup)}
            </div>

            {/* ── Фоновая картинка ── */}
            <div className="te-card">
              <div className="te-sec-title">
                <ImageIcon size={12} strokeWidth={2} />
                {t('themeEditor.secImage')}
                {hasImage && (
                  <button className="btn btn-sm btn-ghost te-bg-clear" onClick={() => handleSelectImage('')}>
                    <X size={12} strokeWidth={2.2} />
                    {t('themeEditor.bgClear')}
                  </button>
                )}
              </div>
              <BgRibbon
                selected={hasImage && draft.bgImage.url.startsWith('/bg/') ? draft.bgImage.url : ''}
                onSelect={handleSelectImage}
              />
              <div className="te-img-grid">
                <SliderRow
                  label={t('themeEditor.panelOpacity')} min={0} max={100} step={1}
                  value={draft.bgImage.panelOpacity} disabled={!hasImage}
                  onChange={(v) => patchBg({ panelOpacity: v })}
                  format={(v) => v + '%'}
                />
                <SliderRow
                  label={t('themeEditor.shellOpacity')} min={0} max={100} step={1}
                  value={draft.bgImage.shellOpacity} disabled={!hasImage}
                  onChange={(v) => patchBg({ shellOpacity: v })}
                  format={(v) => v + '%'}
                />
                <SliderRow
                  label={t('themeEditor.dim')} min={0} max={90} step={1}
                  value={draft.bgImage.dim} disabled={!hasImage}
                  onChange={(v) => patchBg({ dim: v })}
                  format={(v) => v + '%'}
                />
                <div className={'te-fit-row' + (!hasImage ? ' disabled' : '')}>
                  <span className="te-slider-label">{t('themeEditor.fitMode')}</span>
                  <div className="te-fit-btns" role="group" aria-label={t('themeEditor.fitMode')}>
                    {BG_FITS.map((m) => (
                      <button
                        key={m}
                        className={'btn btn-sm te-fit-btn' + (draft.bgImage.fit === m ? ' active' : '')}
                        disabled={!hasImage}
                        aria-pressed={draft.bgImage.fit === m}
                        onClick={() => patchBg({ fit: m })}
                      >
                        {t('themeEditor.fit_' + m)}
                      </button>
                    ))}
                  </div>
                </div>
                <SliderRow
                  label={t('themeEditor.posX')} min={0} max={100} step={1}
                  value={draft.bgImage.posX} disabled={!hasImage}
                  onChange={(v) => patchBg({ posX: v })}
                  format={(v) => v + '%'}
                />
                <SliderRow
                  label={t('themeEditor.posY')} min={0} max={100} step={1}
                  value={draft.bgImage.posY} disabled={!hasImage}
                  onChange={(v) => patchBg({ posY: v })}
                  format={(v) => v + '%'}
                />
                <SliderRow
                  label={t('themeEditor.satur')} min={0} max={200} step={5}
                  value={draft.bgImage.satur} disabled={!hasImage}
                  onChange={(v) => patchBg({ satur: v })}
                  format={(v) => v + '%'}
                />
                <SliderRow
                  label={t('themeEditor.bright')} min={50} max={150} step={5}
                  value={draft.bgImage.bright} disabled={!hasImage}
                  onChange={(v) => patchBg({ bright: v })}
                  format={(v) => v + '%'}
                />
              </div>
            </div>

            {/* ── Эффекты ── */}
            <div className="te-card">
              <div className="te-sec-title"><Sparkles size={12} strokeWidth={2} />{t('themeEditor.secEffects')}</div>
              <div className="te-switch-row">
                <span className="te-slider-label">{t('themeEditor.glow')}</span>
                <Switch checked={draft.effects.glow} hideState onChange={() => patchFx({ glow: !draft.effects.glow })} />
              </div>
              <ColorField
                label={t('themeEditor.glowColor')}
                locked={!draft.effects.glow}
                value={draft.effects.glowColor || draft.colors.accent}
                onChange={(hex) => patchFx({ glowColor: hex })}
              />
              <div className="te-switch-row">
                <span className="te-slider-label">{t('themeEditor.translucent')}</span>
                <Switch checked={draft.effects.translucent} hideState onChange={() => patchFx({ translucent: !draft.effects.translucent })} />
              </div>
              <SliderRow
                label={t('themeEditor.strength')} min={0} max={100} step={5}
                value={draft.effects.strength} disabled={!draft.effects.glow && !draft.effects.translucent && !hasImage}
                onChange={(v) => patchFx({ strength: v })}
                format={(v) => v + '%'}
              />
              <SliderRow
                label={t('themeEditor.modalOpacity')} min={40} max={100} step={5}
                value={draft.effects.modalOpacity}
                onChange={(v) => patchFx({ modalOpacity: v })}
                format={(v) => v + '%'}
              />
            </div>
          </div>

          <div className="te-tab-side">
            <div className="te-card te-pv-card">
              <div className="te-sec-title">
                <Eye size={12} strokeWidth={2} />
                <span className="te-pv-tip">{t('themeEditor.pvTitle')}</span>
              </div>
              <ThemePreview />
            </div>
            <div className="te-footnote">{t('themeEditor.previewNote')}</div>
          </div>
        </div>
        )}
      </div>
    </div>
  );
}

function ColorField({ label, value, onChange, locked }) {
  const [text, setText] = useState(null);
  const valid = text == null || normalizeHexInput(text);
  return (
    <div className={'te-color-row' + (locked ? ' locked' : '')}>
      <label className="te-swatch">
        <input
          type="color"
          value={value}
          aria-label={label}
          disabled={locked}
          onChange={(e) => { setText(null); onChange(e.target.value); }}
        />
      </label>
      <span className="te-color-label">{label}</span>
      <input
        className={'te-hex form-input' + (valid ? '' : ' input-error')}
        value={text != null ? text : value}
        maxLength={7}
        spellCheck="false"
        aria-label={label}
        disabled={locked}
        onChange={(e) => {
          const v = e.target.value;
          setText(v);
          const hex = normalizeHexInput(v);
          if (hex) onChange(hex);
        }}
        onBlur={() => { setText(null); }}
      />
    </div>
  );
}

function SliderRow({ label, min, max, step, value, onChange, format, disabled }) {
  return (
    <div className={'te-slider-row' + (disabled ? ' disabled' : '')}>
      <span className="te-slider-label">{label}</span>
      <input
        type="range"
        className="te-slider slider-ui"
        min={min}
        max={max}
        step={step}
        value={value}
        disabled={disabled}
        aria-label={label}
        style={{ '--val': rangePct(value, min, max) + '%' }}
        onChange={(e) => onChange(+e.target.value)}
      />
      <span className="te-slider-val">{format(value)}</span>
    </div>
  );
}
