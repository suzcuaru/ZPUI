import { Check, Copy } from 'lucide-react';

// Иконочная кнопка «копировать»: пока скопировано — галочка.
// label — i18n-строка для тултипа и aria-label.
export default function CopyIconButton({ copied, onCopy, label, disabled, tooltipPos = 'bottom' }) {
  return (
    <button className="btn btn-icon" onClick={onCopy} disabled={disabled}
      data-tooltip={label} aria-label={label} data-tooltip-pos={tooltipPos}>
      {copied ? <Check size={12} strokeWidth={2.5} /> : <Copy size={12} strokeWidth={2} />}
    </button>
  );
}
