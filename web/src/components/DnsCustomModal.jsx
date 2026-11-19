import { useState, useEffect } from 'react';
import Modal from './ui/Modal';
import Switch from './ui/Switch';
import { useT } from '../i18n';
import { isValidDnsIp, isIPv6Addr } from '../utils';
import { PenLine } from 'lucide-react';

const emptyForm = { name: '', primary: '', secondary: '', doh: '', encrypted: false };

// DnsCustomModal — создание/редактирование пользовательской DNS-записи:
// имя, основной и запасной адрес (только IPv4), опциональный DoH-шаблон.
export default function DnsCustomModal({ open, entry, busy, onSave, onClose }) {
  const { t } = useT();
  const [form, setForm] = useState(emptyForm);
  const [errs, setErrs] = useState({});

  useEffect(() => {
    if (!open) return;
    setErrs({});
    setForm(entry ? {
      name: entry.name || '',
      primary: entry.primary || '',
      secondary: entry.secondary || '',
      doh: entry.doh || '',
      encrypted: !!entry.doh,
    } : { ...emptyForm });
  }, [open, entry]);

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }));

  const submit = () => {
    const e = {};
    if (!form.name.trim()) e.name = true;
    if (!isValidDnsIp(form.primary) || isIPv6Addr(form.primary)) e.primary = true;
    if (form.secondary.trim() && (!isValidDnsIp(form.secondary) || isIPv6Addr(form.secondary))) e.secondary = true;
    if (form.encrypted && form.doh.trim() && !/^https:\/\/\S+\/?\S*$/i.test(form.doh.trim())) e.doh = true;
    setErrs(e);
    if (Object.keys(e).length > 0) return;
    onSave({
      id: entry?.id || '',
      name: form.name.trim(),
      primary: form.primary.trim(),
      secondary: form.secondary.trim(),
      doh: form.encrypted ? form.doh.trim() : '',
      enable: entry ? !!entry.enabled : false,
    });
  };

  return (
    <Modal
      open={open}
      title={t(entry ? 'dns.customEdit' : 'dns.customAdd')}
      icon={<PenLine size={16} strokeWidth={2} />}
      onClose={onClose}
      footer={
        <>
          <button className="btn btn-ghost" onClick={onClose}>{t('common.cancel')}</button>
          <button className="btn btn-accent" onClick={submit} disabled={busy}>{t('common.save')}</button>
        </>
      }
    >
      <div className="dns-form">
        <label className="dns-form-field">
          <span className="dns-form-label">{t('dns.nameLabel')}</span>
          <input
            className={'form-input' + (errs.name ? ' invalid' : '')}
            value={form.name}
            placeholder={t('dns.namePh')}
            maxLength={40}
            spellCheck={false}
            onChange={(e) => set('name', e.target.value)}
          />
        </label>
        <label className="dns-form-field">
          <span className="dns-form-label">{t('dns.primaryLabel')}</span>
          <input
            className={'form-input mono' + (errs.primary ? ' invalid' : '')}
            value={form.primary}
            placeholder={t('dns.primaryPh')}
            spellCheck={false}
            onChange={(e) => set('primary', e.target.value)}
          />
        </label>
        <label className="dns-form-field">
          <span className="dns-form-label">{t('dns.secondaryLabel')}</span>
          <input
            className={'form-input mono' + (errs.secondary ? ' invalid' : '')}
            value={form.secondary}
            placeholder={t('dns.secondaryPh')}
            spellCheck={false}
            onChange={(e) => set('secondary', e.target.value)}
          />
          <span className="dns-form-hint">{t('dns.addrHint')}</span>
        </label>
        <div className="dns-form-field dns-form-switch">
          <div>
            <div className="dns-form-label">{t('dns.encryptedLabel')}</div>
            <div className="dns-form-hint">{t('dns.encryptedHint')}</div>
          </div>
          <Switch size="sm" checked={form.encrypted} onChange={() => set('encrypted', !form.encrypted)} hideState />
        </div>
        {form.encrypted && (
          <label className="dns-form-field">
            <span className="dns-form-label">{t('dns.dohLabel')}</span>
            <input
              className={'form-input mono' + (errs.doh ? ' invalid' : '')}
              value={form.doh}
              placeholder={t('dns.dohPh')}
              spellCheck={false}
              onChange={(e) => set('doh', e.target.value)}
            />
          </label>
        )}
      </div>
    </Modal>
  );
}
