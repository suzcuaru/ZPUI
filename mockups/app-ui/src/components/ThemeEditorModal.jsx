import { useState, useEffect, useRef } from 'react';
import Modal from './ui/Modal';
import Switch from './ui/Switch';
import { api } from '../api';
import { useT } from '../i18n';
import { rangePct } from '../utils';
import {
  defaultTheme, normalizeTheme, cloneTheme, themeEquals,
  applyCustomTheme, removeCustomTheme, normalizeHexInput,
} from '../themeCustom';
import { Palette, RotateCcw, FolderOpen, Save, Image as ImageIcon, X } from 'lucide-react';

// Группы цветов редактора: [ключ в theme.colors, ключ i18n, тултип или null].
const COLOR_GROUPS = [
  {
    title: 'themeEditor.secAccent',
    fields: [
      ['accent', 'themeEditor.accent', 'themeEditor.accentTip'],
      ['success', 'themeEditor.success', null],
      ['warning', 'themeEditor.warning', null],
      ['danger', 'themeEditor.danger', null],
    ],
  },
  {
    title: 'themeEditor.secBg',
    fields: [
      ['bgPage', 'themeEditor.bgPage', null],
      ['bgCard', 'themeEditor.bgCard', null],
      ['bgInset', 'themeEditor.bgInset', null],
    ],
  },
  {
    title: 'themeEditor.secText',
    fields: [
      ['textPrimary', 'themeEditor.textPrimary', null],
      ['textSecondary', 'themeEditor.textSecondary', null],
      ['textTertiary', 'themeEditor.textTertiary', null],
    ],
  },
  {
    title: 'themeEditor.secBorders',
    fields: [
      ['border', 'themeEditor.border', null],
      ['borderLight', 'themeEditor.borderLight', null],
    ],
  },
];

// ThemeEditorModal — редактор пользовательской темы стенда.
// Живой предпросмотр: каждый черновик сразу применяется к стенду
// (style-тег + data-theme="custom"). Закрытие без сохранения возвращает
// состояние на момент входа; «Сброс» — значения на момент входа.
export default function ThemeEditorModal({ open, onClose, showToast }) {
  const { t } = useT();
  const [draft, setDraft] = useState(null);
  const [hasSaved, setHasSaved] = useState(false);
  const [saving, setSaving] = useState(false);
  const entryRef = useRef(null);
  const savedRef = useRef({ applied: false, obj: null });
  const fileRef = useRef(null);

  // Открытие: снимок состояния (активна ли custom, что сохранено в «БД»)
  // и стартовый черновик. Все хуки — до условных возвратов; guard по draft
  // переживает double-invoke эффектов в StrictMode.
  useEffect(() => {
    if (!open || draft) return;
    let alive = true;
    const attr = document.documentElement.getAttribute('data-theme');
    (async () => {
      const r = await api('GET', '/api/custom-theme');
      if (!alive) return;
      const saved = r && r.theme ? normalizeTheme(r.theme) : null;
      const base = saved ? saved.base : (attr === 'light' ? 'light' : 'dark');
      const initial = saved || defaultTheme(base);
      entryRef.current = { theme: cloneTheme(initial), attr: attr === 'custom' ? 'dark' : attr };
      savedRef.current = { applied: attr === 'custom', obj: saved ? cloneTheme(saved) : null };
      setHasSaved(!!saved);
      setDraft(initial);
    })();
    return () => { alive = false; };
  }, [open, draft]);

  // Живой предпросмотр: черновик мгновенно красит стенд.
  useEffect(() => {
    if (!open || !draft) return;
    applyCustomTheme(draft);
    document.documentElement.setAttribute('data-theme', 'custom');
  }, [open, draft]);

  // Закрытие модалки (крестик/Esc/overlay/«Готово»): откат к состоянию входа
  // либо к последнему сохранённому варианту.
  useEffect(() => {
    if (open || !entryRef.current) return;
    const st = savedRef.current;
    if (st.applied && st.obj) {
      applyCustomTheme(st.obj);
      document.documentElement.setAttribute('data-theme', 'custom');
    } else {
      removeCustomTheme();
      document.documentElement.setAttribute('data-theme', entryRef.current.attr || 'dark');
    }
    setDraft(null);
    entryRef.current = null;
  }, [open]);

  if (!open) return null;

  const setColor = (key, hex) => setDraft((d) => ({ ...d, colors: { ...d.colors, [key]: hex } }));
  const patch = (fn) => setDraft((d) => fn(d));

  const handleBase = (base) => {
    if (!draft || draft.base === base) return;
    patch((d) => ({ ...defaultTheme(base), bgImage: d.bgImage, effects: d.effects }));
  };

  const handleImageUrl = (url) => {
    patch((d) => ({ ...d, bgImage: { ...d.bgImage, url } }));
  };

  const handleFile = (e) => {
    const file = e.target.files && e.target.files[0];
    e.target.value = '';
    if (!file || !draft) return;
    if (draft.bgImage.url && draft.bgImage.url.startsWith('blob:')) {
      try { URL.revokeObjectURL(draft.bgImage.url); } catch {}
    }
    const url = URL.createObjectURL(file);
    // Первая картинка — разумные стартовые значения стекла, иначе изменений не видно.
    const fresh = !draft.bgImage.url;
    patch((d) => ({
      ...d,
      bgImage: {
        ...d.bgImage,
        url,
        panelOpacity: fresh ? Math.min(d.bgImage.panelOpacity, 85) : d.bgImage.panelOpacity,
        blur: fresh ? Math.max(d.bgImage.blur, 6) : d.bgImage.blur,
        dim: fresh ? Math.max(d.bgImage.dim, 25) : d.bgImage.dim,
      },
    }));
  };

  const clearImage = () => {
    if (draft.bgImage.url && draft.bgImage.url.startsWith('blob:')) {
      try { URL.revokeObjectURL(draft.bgImage.url); } catch {}
    }
    patch((d) => ({ ...d, bgImage: { ...d.bgImage, url: '' } }));
  };

  const handleReset = () => {
    if (!entryRef.current) return;
    setDraft(cloneTheme(entryRef.current.theme));
  };

  const handleLoad = async () => {
    const r = await api('GET', '/api/custom-theme');
    if (r && r.theme) {
      setDraft(normalizeTheme(r.theme));
      showToast(t('themeEditor.loadToast'), 'info');
    } else {
      showToast(t('themeEditor.loadNone'), 'warning');
    }
  };

  const handleSave = async () => {
    if (!draft || saving) return;
    setSaving(true);
    const r = await api('POST', '/api/custom-theme', { theme: draft });
    setSaving(false);
    if (r && r.error) { showToast(r.error, 'error'); return; }
    savedRef.current = { applied: true, obj: cloneTheme(draft) };
    setHasSaved(true);
    showToast(t('themeEditor.savedToast'), 'success');
    window.dispatchEvent(new CustomEvent('zpui:theme-changed', { detail: { theme: 'custom' } }));
  };

  const hasImage = !!draft?.bgImage?.url;

  const colorGroup = (group) => (
    <div className="te-group" key={group.title}>
      <div className="te-sec-title">{t(group.title)}</div>
      {group.fields.map(([key, label, tip]) => (
        <ColorField
          key={key}
          label={t(label)}
          tip={tip ? t(tip) : null}
          value={draft.colors[key]}
          onChange={(hex) => setColor(key, hex)}
        />
      ))}
    </div>
  );

  return (
    <Modal
      wide
      className="te-modal"
      open={open}
      onClose={onClose}
      title={t('themeEditor.title')}
      icon={<Palette size={18} strokeWidth={1.8} />}
      tone="accent"
      footer={
        <>
          <button className="btn btn-sm te-foot-reset" onClick={handleReset} data-tooltip={t('themeEditor.resetTip')}>
            <RotateCcw size={12} strokeWidth={2} />
            {t('themeEditor.reset')}
          </button>
          <button className="btn btn-sm" onClick={handleLoad} disabled={!hasSaved}>
            <FolderOpen size={12} strokeWidth={2} />
            {t('themeEditor.load')}
          </button>
          <button className="btn btn-accent btn-sm" onClick={handleSave} disabled={saving || (draft && !!savedRef.current.obj && themeEquals(draft, savedRef.current.obj))}>
            <Save size={12} strokeWidth={2} />
            {t('themeEditor.save')}
          </button>
          <button className="btn btn-sm" onClick={onClose}>{t('themeEditor.done')}</button>
        </>
      }
    >
      {!draft ? (
        <div className="modal-loading"><span className="mini-spin" />{t('common.loading')}</div>
      ) : (
        <div className="te-editor">
          <div className="te-grid te-grid-top">
            <div className="te-col">
              <div className="te-group">
                <div className="te-sec-title">{t('themeEditor.base')}</div>
                <div className="te-base-row" data-tooltip={t('themeEditor.baseTip')}>
                  <button className={'btn btn-sm' + (draft.base === 'light' ? ' active' : '')} onClick={() => handleBase('light')}>{t('settings.lightTheme')}</button>
                  <button className={'btn btn-sm' + (draft.base === 'dark' ? ' active' : '')} onClick={() => handleBase('dark')}>{t('settings.darkTheme')}</button>
                </div>
              </div>
              {colorGroup(COLOR_GROUPS[0])}
            </div>

            <div className="te-col">
              {colorGroup(COLOR_GROUPS[1])}
              {colorGroup(COLOR_GROUPS[2])}
              {colorGroup(COLOR_GROUPS[3])}
            </div>

            <div className="te-col">
              <div className="te-group">
                <div className="te-sec-title">{t('themeEditor.secShape')}</div>
                <SliderRow
                  label={t('themeEditor.radiusMd')} min={4} max={16} step={1}
                  value={draft.radius.md}
                  onChange={(v) => patch((d) => ({ ...d, radius: { ...d.radius, md: v } }))}
                  format={(v) => v + 'px'}
                />
                <SliderRow
                  label={t('themeEditor.radiusLg')} min={4} max={16} step={1}
                  value={draft.radius.lg}
                  onChange={(v) => patch((d) => ({ ...d, radius: { ...d.radius, lg: v } }))}
                  format={(v) => v + 'px'}
                />
                <SliderRow
                  label={t('themeEditor.contrast')} min={50} max={150} step={5}
                  value={draft.contrast}
                  onChange={(v) => patch((d) => ({ ...d, contrast: v }))}
                  format={(v) => v + '%'}
                />
              </div>
            </div>
          </div>

          <div className="te-grid te-grid-bottom">
            <div className="te-col">
              <div className="te-group">
                <div className="te-sec-title">{t('themeEditor.secImage')}</div>
                <div className="te-url-row">
                  <input
                    className="form-input te-url"
                    value={draft.bgImage.url}
                    placeholder={t('themeEditor.bgUrlPh')}
                    spellCheck="false"
                    aria-label={t('themeEditor.bgUrl')}
                    onChange={(e) => handleImageUrl(e.target.value)}
                  />
                  <button className="btn btn-sm" onClick={() => fileRef.current && fileRef.current.click()} data-tooltip={t('themeEditor.bgFileTip')}>
                    <ImageIcon size={12} strokeWidth={2} />
                    {t('themeEditor.bgFile')}
                  </button>
                  {hasImage && (
                    <button className="btn btn-sm btn-ghost" onClick={clearImage} aria-label={t('themeEditor.bgClear')}>
                      <X size={13} strokeWidth={2.2} />
                    </button>
                  )}
                  <input ref={fileRef} type="file" accept="image/*" hidden onChange={handleFile} />
                </div>
                <SliderRow
                  label={t('themeEditor.panelOpacity')} min={0} max={100} step={1}
                  value={draft.bgImage.panelOpacity} disabled={!hasImage}
                  onChange={(v) => patch((d) => ({ ...d, bgImage: { ...d.bgImage, panelOpacity: v } }))}
                  format={(v) => v + '%'}
                />
                <SliderRow
                  label={t('themeEditor.blur')} min={0} max={20} step={1}
                  value={draft.bgImage.blur} disabled={!hasImage}
                  onChange={(v) => patch((d) => ({ ...d, bgImage: { ...d.bgImage, blur: v } }))}
                  format={(v) => v + 'px'}
                />
                <SliderRow
                  label={t('themeEditor.dim')} min={0} max={90} step={1}
                  value={draft.bgImage.dim} disabled={!hasImage}
                  onChange={(v) => patch((d) => ({ ...d, bgImage: { ...d.bgImage, dim: v } }))}
                  format={(v) => v + '%'}
                />
              </div>
            </div>

            <div className="te-col">
              <div className="te-group">
                <div className="te-sec-title">{t('themeEditor.secEffects')}</div>
                <div className="te-switch-row">
                  <span className="te-slider-label">{t('themeEditor.glow')}</span>
                  <Switch checked={draft.effects.glow} hideState onChange={() => patch((d) => ({ ...d, effects: { ...d.effects, glow: !d.effects.glow } }))} />
                </div>
                <div className="te-switch-row">
                  <span className="te-slider-label">{t('themeEditor.translucent')}</span>
                  <Switch checked={draft.effects.translucent} hideState onChange={() => patch((d) => ({ ...d, effects: { ...d.effects, translucent: !d.effects.translucent } }))} />
                </div>
                <SliderRow
                  label={t('themeEditor.strength')} min={0} max={100} step={5}
                  value={draft.effects.strength} disabled={!draft.effects.glow && !draft.effects.translucent && !hasImage}
                  onChange={(v) => patch((d) => ({ ...d, effects: { ...d.effects, strength: v } }))}
                  format={(v) => v + '%'}
                />
              </div>
              <div className="te-footnote">{t('themeEditor.previewNote')}</div>
            </div>
          </div>
        </div>
      )}
    </Modal>
  );
}

function ColorField({ label, tip, value, onChange }) {
  const [text, setText] = useState(null);
  const valid = text == null || normalizeHexInput(text);
  return (
    <div className="te-color-row">
      <label className="te-swatch" data-tooltip={tip || undefined}>
        <input
          type="color"
          value={value}
          aria-label={label}
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
