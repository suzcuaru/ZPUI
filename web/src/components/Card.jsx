export default function Card({ title, subtitle, action, compact, className, children }) {
  const cls = ['card'];
  if (compact) cls.push('card-compact');
  if (className) cls.push(className);

  return (
    <div className={cls.join(' ')}>
      {(title || action) && (
        <div className="card-header">
          <div className="card-titles">
            {title && <div className="card-title">{title}</div>}
            {subtitle && <div className="card-subtitle">{subtitle}</div>}
          </div>
          {action && <div className="card-action">{action}</div>}
        </div>
      )}
      <div className="card-body">{children}</div>
    </div>
  );
}
