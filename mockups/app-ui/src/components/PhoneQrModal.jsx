import { useEffect, useRef } from 'react';
import QRCode from 'qrcode';
import Modal from './ui/Modal';
import { useT } from '../i18n';
import { Wifi, WifiOff, QrCode } from 'lucide-react';

// PhoneQrModal — QR-код с адресом веб-страницы моста: телефон сканирует код
// камерой, открывается страница с кнопкой «Подключить в Telegram» —
// Telegram запускается и добавляет MTProto-прокси с этого ПК.
export default function PhoneQrModal({ open, onClose, webUrl, lanOpen, enablingLan, onEnableLan }) {
  const { t } = useT();
  const canvasRef = useRef(null);

  useEffect(() => {
    if (!open || !webUrl || !canvasRef.current) return;
    QRCode.toCanvas(canvasRef.current, webUrl, {
      width: 212,
      margin: 1,
      color: { dark: '#000000', light: '#ffffff' },
    }).catch(() => {});
  }, [open, webUrl]);

  return (
    <Modal
      title={t('tgproxy.phoneQrTitle')}
      onClose={onClose}
      open={open}
      wide
      icon={<QrCode size={18} strokeWidth={2} />}
      tone="accent"
    >
      <div className="phqr">
        <div className="phqr-qr-wrap"><canvas ref={canvasRef} /></div>
        <div className="phqr-hint">{t('tgproxy.phoneQrHint')}</div>
        {webUrl && <code className="phqr-url">{webUrl}</code>}
        <div className={'phqr-lan ' + (lanOpen ? 'ok' : 'off')}>
          {lanOpen ? <Wifi size={12} strokeWidth={2.2} /> : <WifiOff size={12} strokeWidth={2.2} />}
          <span>{lanOpen ? t('tgproxy.phoneQrLanOk') : t('tgproxy.phoneQrLanOff')}</span>
          {!lanOpen && (
            <button className="btn btn-accent btn-sm" onClick={onEnableLan} disabled={enablingLan}>
              {enablingLan && <span className="mini-spin" />}
              {t('tgproxy.phoneQrEnableLan')}
            </button>
          )}
        </div>
        <div className="phqr-note">{t('tgproxy.phoneQrSameNet')}</div>
      </div>
    </Modal>
  );
}
