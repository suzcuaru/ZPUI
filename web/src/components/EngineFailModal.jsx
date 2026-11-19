import { useState, useEffect, useRef } from 'react';
import { ShieldAlert, Wrench, MessageSquareWarning, Download } from 'lucide-react';
import Modal from './ui/Modal';
import FeedbackModal from './feedback/FeedbackModal';
import { api } from '../api';
import { useT } from '../i18n';
import { useEngineFail, clearEngineFail } from '../engineFail';
import { subscribeEvent } from '../events';

// EngineFailModal — движок запрета не запустился (решение владельца 14.11):
// модалка при входе в интерфейс с ошибкой и рекомендацией. «Переустановить
// запрет» заново скачивает и ставит движок (прогресс — engine:progress,
// тот же upd-flow, что в обновлениях); если систему блокирует HVCI — кнопка
// исправления; кнопка «Сообщить о проблеме» открывает форму с
// предзаполненным текстом ошибки.
const ENGINE_LABEL = 'ZAPRET';

export default function EngineFailModal({ onClose }) {
  const { t } = useT();
  const info = useEngineFail();
  const [reinstalling, setReinstalling] = useState(false);
  const [progress, setProgress] = useState(null);
  const [reinstallError, setReinstallError] = useState('');
  const [report, setReport] = useState(null);
  const aliveRef = useRef(true);

  useEffect(() => {
    aliveRef.current = true;
    return () => { aliveRef.current = false; };
  }, []);

  // Прогресс переустановки: каналы engine:progress/engine:done (component
  // 'zapret'), интерфейс — тот же upd-flow, что в модалке обновлений.
  useEffect(() => {
    if (!reinstalling) return undefined;
    const offProgress = subscribeEvent('engine:progress', (d) => {
      if (!d || d.component !== 'zapret') return;
      setProgress({
        status: d.status || 'downloading',
        pct: d.percent || 0,
        dl: d.downloaded || 0,
        total: d.total || 0,
        step: d.step || '',
      });
    });
    const offDone = subscribeEvent('engine:done', (d) => {
      if (!d || d.component !== 'zapret') return;
      if (d.ok) {
        // Движок установлен — пытаемся запустить, модалка закрывается.
        api('POST', '/api/zapret/start').catch(() => {});
        setReinstalling(false);
        setProgress(null);
        clearEngineFail();
        onClose && onClose();
        return;
      }
      setReinstalling(false);
      setProgress(null);
      setReinstallError(d.error || t('update.errorBody'));
    });
    return () => { offProgress(); offDone(); };
  }, [reinstalling, t, onClose]);

  if (!info) return null;

  const close = () => {
    clearEngineFail();
    onClose && onClose();
  };

  const startReinstall = async () => {
    setReinstallError('');
    setProgress({ status: 'downloading', pct: 0, dl: 0, total: 0, step: '' });
    setReinstalling(true);
    const r = await api('POST', '/api/zapret/install-engine');
    if (r && r.error) {
      setReinstalling(false);
      setProgress(null);
      setReinstallError(r.error);
    }
  };

  const fmtMb = (n) => (n > 0 ? (n / 1024 / 1024).toFixed(1) + ' ' + t('update.mb') : '');
  const progressMeta = progress && (progress.status === 'downloading'
    ? (progress.total > 0 ? `${fmtMb(progress.dl)} / ${fmtMb(progress.total)}` : `${progress.pct}%`)
    : progress.step);

  const prefill = [
    `${t('engineFail.reportTitle')}: ${ENGINE_LABEL}${info.version ? ' (v' + info.version + ')' : ''}`,
    `${t('engineFail.hvciLine')}: ${info.hvci ? t('common.on') : t('common.off')}`,
    info.error ? `${t('update.errorTitle')}: ${info.error}` : '',
    t('engineFail.reportNote'),
  ].filter(Boolean).join('\n');

  return (
    <>
      <Modal
        title={reinstalling ? t('engineFail.reinstalling') : t('engineFail.title')}
        icon={<ShieldAlert size={15} strokeWidth={2} />}
        tone="danger"
        open={true}
        closable={!reinstalling}
        onClose={close}
        footer={(
          <>
            <button className="btn btn-sm" onClick={() => setReport(prefill)}>
              <MessageSquareWarning size={13} strokeWidth={2.2} />
              {t('engineFail.report')}
            </button>
            <button className="btn btn-accent" onClick={startReinstall} disabled={reinstalling}>
              <Download size={14} strokeWidth={2.2} />
              {t('engineFail.reinstall')}
            </button>
          </>
        )}
      >
        <div className="efail-body">
          {reinstalling && progress ? (
            <div className="upd-flow-body">
              <Download size={30} strokeWidth={1.8} className="upd-flow-icon" />
              <span className="upd-flow-pct">{progress.pct}%</span>
              <div className="upd-flow-bar">
                <div className="upd-flow-fill" style={{ width: `${progress.pct}%` }} />
              </div>
              {progressMeta && <span className="upd-flow-meta">{progressMeta}</span>}
            </div>
          ) : (
            <>
              <p className="efail-text">
                {t('engineFail.lead')}
              </p>
              {info.hvci && (
                <div className="efail-hvci">
                  <Wrench size={14} strokeWidth={2.2} />
                  <span>{t('engineFail.hvciAdvice')}</span>
                </div>
              )}
              {info.error && <p className="efail-error mono">{info.error}</p>}
              {reinstallError && <p className="efail-error mono">{reinstallError}</p>}
              <p className="efail-note">{t('engineFail.note')}</p>
            </>
          )}
        </div>
      </Modal>

      {report && (
        <FeedbackModal
          open={true}
          onClose={() => setReport(null)}
          initialTopic="engine"
          initialText={report}
        />
      )}
    </>
  );
}
