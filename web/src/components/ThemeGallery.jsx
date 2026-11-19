import { useRef } from 'react';
import { useT } from '../i18n';
import { normalizeTheme } from '../themeCustom';
import { Check, Pencil, Download, Trash2, Plus, Upload } from 'lucide-react';

// ThemeCard — карточка сохранённой темы: мини-превью (цвета темы или картинка),
// имя и действия (Применить/Редактировать/Экспорт/Удалить). Используется и в
// модалке-галерее, и в секции «Сохранённые темы» под редактором.
export function ThemeCard({ slot, active, mode = 'apply', onApply, onEdit, onExport, onDelete }) {
  const { t } = useT();
  const n = normalizeTheme(slot.theme);
  const name = slot.name || t('themeGallery.unnamed');
  const swatches = [n.colors.accent, n.colors.success, n.colors.warning, n.colors.danger, n.colors.bgCard];

  return (
    <div className={'tg-card' + (active ? ' active' : '')}>
      <div className="tg-preview" style={{ background: n.colors.bgPage }}>
        {n.bgImage.url && <img src={n.bgImage.url} alt="" loading="lazy" />}
        <span className="tg-mini" style={{ background: n.colors.bgCard, borderColor: n.colors.border }}>
          <i style={{ background: n.colors.accent }} />
        </span>
        <span className="tg-swatches">
          {swatches.map((c, i) => (
            <span key={i} className="tg-swatch" style={{ background: c }} />
          ))}
        </span>
        {active && (
          <span className="tg-active-badge">
            <Check size={9} strokeWidth={3} />
            {t('themeGallery.active')}
          </span>
        )}
      </div>
      <span className="tg-name" title={name}>{name}</span>
      <div className="tg-actions">
        {mode === 'apply' && (active ? (
          <span className="tg-on">
            <Check size={12} strokeWidth={2.6} />
            {t('themeGallery.isActive')}
          </span>
        ) : (
          <button className="btn btn-accent btn-xs tg-apply" onClick={() => onApply(slot)} disabled={!slot.theme}>
            {t('themeGallery.apply')}
          </button>
        ))}
        {(onEdit) && (
          <button className="tg-ibtn" aria-label={t('themeGallery.edit')} onClick={() => onEdit(slot)}>
            <Pencil size={13} strokeWidth={2} />
          </button>
        )}
        {(onExport) && (
          <button
            className="tg-ibtn"
            aria-label={t('themeGallery.export')}
            onClick={() => onExport(slot)}
            disabled={!slot.theme}
          >
            <Download size={13} strokeWidth={2} />
          </button>
        )}
        {(onDelete) && (
          <button
            className="tg-ibtn tg-del"
            aria-label={t('themeGallery.delete') + ' ' + name}
            onClick={() => onDelete(slot)}
            disabled={active}
          >
            <Trash2 size={13} strokeWidth={2} />
          </button>
        )}
      </div>
    </div>
  );
}

// ThemeGallery — сетка карточек тем + слоты «Создать тему» и «Импорт».
// Данные локальные (без скелетонов); один и тот же компонент рендерится
// в модалке «Своя тема» и инлайн на под-вкладке «Тема».
export default function ThemeGallery({ themes, activeId, mode = 'apply', onApply, onEdit, onExport, onDelete, onCreate, onImportFile }) {
  const { t } = useT();
  const fileRef = useRef(null);

  return (
    <div className="tg-wrap">
      <div className="tg-grid">
        {(themes || []).map((s) => (
          <ThemeCard
            key={s.id}
            slot={s}
            mode={mode}
            active={s.id === activeId}
            onApply={onApply}
            onEdit={onEdit}
            onExport={onExport}
            onDelete={onDelete}
          />
        ))}
        {(onCreate) && (
          <button className="tg-card tg-card-add" onClick={onCreate}>
            <span className="tg-add-ic"><Plus size={18} strokeWidth={2} /></span>
            <span>{t('themeGallery.create')}</span>
          </button>
        )}
        {(onImportFile) && (
          <button className="tg-card tg-card-add" onClick={() => fileRef.current && fileRef.current.click()}>
            <span className="tg-add-ic"><Upload size={18} strokeWidth={2} /></span>
            <span>{t('themeGallery.import')}</span>
          </button>
        )}
      </div>
      {(!themes || themes.length === 0) && (
        <div className="tg-empty">
          {t(mode === 'apply' ? 'themeGallery.emptyPicker' : 'themeGallery.empty')}
        </div>
      )}
      <input
        ref={fileRef}
        type="file"
        accept=".zip,.json,application/zip,application/json"
        hidden
        onChange={(e) => {
          const f = e.target.files && e.target.files[0];
          e.target.value = '';
          if (f) onImportFile(f);
        }}
      />
    </div>
  );
}
