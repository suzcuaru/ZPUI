import { useState } from 'react';
import Modal from '../ui/Modal';
import { api } from '../../api';
import { useT } from '../../i18n';
import { FileCheck2, Send, FolderOpen } from 'lucide-react';

// ReportReadyModal — отчёт сохранён: открыть файл или отправить в Телеграм
// (файл кладётся в буфер обмена, открывается группа поддержки — Ctrl+V
// в чате прикрепляет файл). Small-вариант общего каркаса с моно-путём.
export default function ReportReadyModal({ open, path, onClose, showToast }) {
  const { t } = useT();
  const [sending, setSending] = useState(false);

  const openFile = async () => {
    const r = await api('POST', '/api/report/open', { path });
    if (r?.error) showToast(r.error, 'error');
  };

  const sendToTelegram = async () => {
    setSending(true);
    const copy = await api('POST', '/api/report/copy-file', { path });
    if (copy?.error) {
      setSending(false);
      showToast(copy.error, 'error');
      return;
    }
    await api('POST', '/api/telegram/open', { domain: 'zpui_chat' });
    setSending(false);
    showToast(t('report.sentTelegram'), 'success');
    onClose();
  };

  return (
    <Modal
      title={t('report.readyTitle')}
      onClose={onClose}
      open={open}
      size="sm"
      icon={<FileCheck2 size={18} strokeWidth={2} />}
      tone="success"
      footer={
        <>
          <button className="btn btn-sm" onClick={openFile}>
            <FolderOpen size={13} strokeWidth={2.2} />
            {t('report.openFile')}
          </button>
          <button className="btn btn-accent btn-sm" onClick={sendToTelegram} disabled={sending}>
            {sending ? <span className="mini-spin" /> : <Send size={13} strokeWidth={2.2} />}
            {t('report.sendTelegram')}
          </button>
          <button className="btn btn-sm btn-ghost" onClick={onClose}>{t('common.close')}</button>
        </>
      }
    >
      <div className="rptr">
        <span className="rptr-title">{t('report.readyDesc')}</span>
        <code className="rptr-path mono" data-tooltip={path}>{path}</code>
        <div className="rptr-note">{t('report.telegramHint')}</div>
      </div>
    </Modal>
  );
}
