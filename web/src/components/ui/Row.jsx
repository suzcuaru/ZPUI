/**
 * Row — переиспользуемый компонент строки настроек.
 * Заменяет дублированную разметку:
 *   <div className="set-row">
 *     <div className="set-row-info">
 *       <span className="set-row-title">...</span>
 *       <span className="set-row-desc">...</span>
 *     </div>
 *     {children}
 *   </div>
 */
export default function Row({ title, desc, children, style }) {
  return (
    <div className="set-row" style={style}>
      <div className="set-row-info">
        <span className="set-row-title">{title}</span>
        {desc && <span className="set-row-desc">{desc}</span>}
      </div>
      {children}
    </div>
  );
}