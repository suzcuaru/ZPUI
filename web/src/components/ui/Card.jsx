export default function Card({ title, subtitle, action, children, className = '' }) {
  return (
    <div className={'card ' + className}>
      {(title || subtitle || action) && (
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
