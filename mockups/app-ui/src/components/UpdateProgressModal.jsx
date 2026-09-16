import Modal from './ui/Modal';
import { useT } from '../i18n';
import { useUpdateFlow, applyDownloadedUpdate, closeUpdateModal, retryDownload } from '../hooks/useUpdateFlow';
import { Download, HardDriveDownload, CheckCircle2, AlertTriangle } from 'lucide-react';

// UpdateProgressModal — скачивание и установка обновлений ZPUI/zapret.
// Во время скачивания и установки закрыть модалку нельзя (без крестика,
// без клика по фону). Прогресс — крупный бар 8px + моно-процент.
// После скачивания появляется кнопка «Установить» в футере.
export default function UpdateProgressModal() {
  const { t } = useT();
  const flow = useUpdateFlow();
  if (!flow.open || flow.phase === 'idle') return null;

  const componentName = flow.component === 'zapret' ? t('nav.zapret') : 'ZPUI';
  const fmtVer = (v) => (v ? (v.startsWith('v') ? v : 'v' + v) : '');
  const busy = flow.phase === 'downloading' || flow.phase === 'installing' || flow.phase === 'launching';
  const closable = !busy;

  const titleFor = () => {
    switch (flow.phase) {
      case 'downloading': return t('update.downloadTitle', { component: componentName });
      case 'downloaded': return t('update.downloadComplete');
      case 'installing': return t('update.installingTitle', { component: componentName });
      case 'launching': return t('update.launchingTitle');
      case 'done': return t('update.doneTitle', { component: componentName, version: fmtVer(flow.version) });
      case 'error': return t('update.errorTitle');
      default: return '';
    }
  };

  const pct = flow.total > 0 ? flow.percent : Math.min(flow.percent, 99);
  const mb = (n) => (n > 0 ? (n / 1024 / 1024).toFixed(1) + ' ' + t('update.mb') : '');

  const sourceName = flow.source === 'github' ? 'GitHub' : t('update.sourceYandex');

  let body = null;
  let footer = null;
  if (flow.phase === 'downloading') {
    body = (
      <div className="upd-flow-body">
        <Download size={30} strokeWidth={1.8} className="upd-flow-icon" />
        <span className="upd-flow-pct">{pct}%</span>
        <div className="upd-flow-bar">
          <div className="upd-flow-fill" style={{ width: `${pct}%` }} />
        </div>
        <span className="upd-flow-meta">
          {flow.total > 0
            ? `${mb(flow.downloaded)} / ${mb(flow.total)}`
            : (mb(flow.downloaded) || `${flow.percent}%`)}
        </span>
        {flow.component === 'zapret' && flow.source && (
          <span className="upd-flow-source">{t('update.fromSource', { source: sourceName })}</span>
        )}
      </div>
    );
  } else if (flow.phase === 'downloaded') {
    body = (
      <div className="upd-flow-body">
        <HardDriveDownload size={30} strokeWidth={1.8} className="upd-flow-icon ok" />
        <p className="upd-flow-text">
          {flow.component === 'ZPUI'
            ? t('update.downloadedSetupHint', { version: fmtVer(flow.version) })
            : t('update.downloadedZipHint', { version: fmtVer(flow.version) })}
        </p>
      </div>
    );
    footer = (
      <>
        <button className="btn btn-sm" onClick={closeUpdateModal}>{t('update.later')}</button>
        <button className="btn btn-accent" onClick={applyDownloadedUpdate}>
          <HardDriveDownload size={14} strokeWidth={2.2} />
          {t('update.install')}
        </button>
      </>
    );
  } else if (flow.phase === 'installing') {
    body = (
      <div className="upd-flow-body">
        <Download size={30} strokeWidth={1.8} className="upd-flow-icon" />
        <span className="upd-flow-pct">{flow.percent}%</span>
        <div className="upd-flow-bar">
          <div className="upd-flow-fill" style={{ width: `${flow.percent}%` }} />
        </div>
        {flow.step && <span className="upd-flow-meta">{flow.step}</span>}
      </div>
    );
  } else if (flow.phase === 'launching') {
    body = (
      <div className="upd-flow-body">
        <span className="mini-spin upd-flow-spin" />
        <p className="upd-flow-text">{t('update.launchingInstaller')}</p>
      </div>
    );
  } else if (flow.phase === 'done') {
    body = (
      <div className="upd-flow-body">
        <CheckCircle2 size={30} strokeWidth={1.8} className="upd-flow-icon ok" />
        <p className="upd-flow-text">{t('update.doneBody', { version: fmtVer(flow.version) })}</p>
      </div>
    );
    footer = (
      <button className="btn btn-accent" onClick={closeUpdateModal}>{t('common.completed')}</button>
    );
  } else if (flow.phase === 'error') {
    body = (
      <div className="upd-flow-body">
        <AlertTriangle size={30} strokeWidth={1.8} className="upd-flow-icon err" />
        <p className="upd-flow-text err" role="alert">{flow.error || t('update.errorBody')}</p>
      </div>
    );
    footer = (
      <>
        <button className="btn btn-sm" onClick={closeUpdateModal}>{t('update.close')}</button>
        <button className="btn btn-accent" onClick={retryDownload}>{t('update.retry')}</button>
      </>
    );
  }

  return (
    <Modal
      title={titleFor()}
      onClose={closeUpdateModal}
      open={true}
      closable={closable}
      footer={footer}
    >
      {body}
    </Modal>
  );
}
