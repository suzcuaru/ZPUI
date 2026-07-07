import { X } from 'lucide-react';

export default function Modal({ title, children, onClose, wide, open }) {
  if (!open) return null;

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className={'modal' + (wide ? ' modal-wide' : '')} onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <span className="modal-title">{title}</span>
          <button className="modal-close" onClick={onClose}><X size={16} strokeWidth={2.5} /></button>
        </div>
        <div className="modal-body">{children}</div>
      </div>
    </div>
  );
}
