// Мини-пилюля состояния «точка + текст» (вкл/выкл, онлайн/оффлайн).
// variant — дополнительный цветовой класс (например warn).
// CSS: .syncd-pill в styles/misc-panels.css — класс исторический, общий.
export default function StatePill({ on, variant, children }) {
  return (
    <span className={'syncd-pill' + (on ? ' on' : '') + (variant ? ' ' + variant : '')}>
      <span className="syncd-pill-dot" />
      {children}
    </span>
  );
}
