import { useState, useEffect } from 'react';
import Modal from './ui/Modal';
import { api, openExternal } from '../api';
import { useT } from '../i18n';
import { Download, ArrowDownCircle, ArrowRight, ExternalLink } from 'lucide-react';
import { openUpdateModal } from '../hooks/useUpdateFlow';

const NOTES_ROUTES = {
  ZPUI: '/api/updates/release-notes',
  zapret: '/api/updates/zapret-release-notes',
};

function extractNotes(body) {
  if (!body) return [];
  const items = [];
  for (const raw of String(body).split('\n')) {
    const m = raw.trim().match(/^[-*]\s+(.{6,})$/);
    if (m) {
      items.push(m[1].replace(/\*\*|`/g, '').trim());
      if (items.length >= 4) break;
    }
  }
  return items;
}

// UpdateAvailableModal — «доступна новая версия» (стандарт 520px):
// иконка-контекст, крупная моно-строка «v1.9.1 → v2.0.0», блок «что нового»
// (если релиз отдал notes), футер «Позже» + «Установить». Кнопка «Установить»
// сразу открывает модалку скачивания (UpdateProgressModal) и начинает загрузку.
export default function UpdateAvailableModal({ data, onClose }) {
  const { t } = useT();
  const [notes, setNotes] = useState(null);

  useEffect(() => {
    setNotes(null);
    if (!data?.latest) return undefined;
    let alive = true;
    const route = NOTES_ROUTES[data.component] || NOTES_ROUTES.ZPUI;
    api('GET', route + '?version=' + encodeURIComponent(data.latest))
      .then((d) => {
        if (alive && d && !d.error) setNotes(d);
      })
      .catch(() => {});
    return () => { alive = false; };
  }, [data]);

  if (!data) return null;

  const fmtVer = (v) => (v ? (v.startsWith('v') ? v : 'v' + v) : '—');
  const componentName = data.component === 'zapret' ? t('nav.zapret') : 'ZPUI';
  const notesItems = extractNotes(notes?.body);

  const onUpdate = () => {
    onClose();
    openUpdateModal(data.component);
  };

  return (
    <Modal
      title={t('update.availableTitle')}
      icon={<ArrowDownCircle size={18} strokeWidth={2} />}
      tone="success"
      onClose={onClose}
      open={true}
      footer={
        <>
          <button className="btn btn-sm" onClick={onClose}>{t('update.later')}</button>
          <button className="btn btn-accent" onClick={onUpdate}>
            <Download size={14} strokeWidth={2.2} />
            {t('update.install')}
          </button>
        </>
      }
    >
      <div className="upd-modal-body">
        <div className="upd-modal-versions">
          <span className="upd-modal-comp">{componentName}</span>
          <span className="upd-modal-ver">{fmtVer(data.current)}</span>
          <span className="upd-modal-arrow"><ArrowRight size={18} strokeWidth={2.2} /></span>
          <span className="upd-modal-ver new">{fmtVer(data.latest)}</span>
        </div>
        <p className="upd-modal-text">{t('update.availableBody', { component: componentName, latest: fmtVer(data.latest) })}</p>
        {notesItems.length > 0 && (
          <div className="upd-notes">
            <span className="upd-notes-title">{t('update.whatsNew')}</span>
            <ul className="upd-notes-list">
              {notesItems.map((n, i) => <li key={i}>{n}</li>)}
            </ul>
            {notes.html_url && (
              <button className="upd-notes-more" onClick={() => openExternal(notes.html_url)}>
                <ExternalLink size={13} strokeWidth={2.2} />
                {t('update.whatsNewFull')}
              </button>
            )}
          </div>
        )}
      </div>
    </Modal>
  );
}
