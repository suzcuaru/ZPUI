import React from 'react';
import { api } from '../api';
import ru from '../locales/ru.json';
import en from '../locales/en.json';

// Краш рендера может случиться до того, как эффект I18nProvider успеет
// зарегистрировать t() — читаем язык и словарь напрямую.
const crashText = (key) => {
  const lang = localStorage.getItem('zpui-lang') === 'en' ? en : ru;
  return lang?.crash?.[key] || ru.crash?.[key] || key;
};

export default class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { error: null, info: null };
  }
  static getDerivedStateFromError(error) {
    return { error };
  }
  componentDidCatch(error, info) {
    this.setState({ info });
    console.error('[ErrorBoundary]', error, info);
    try {
      api('POST', '/api/logs/error-code', { code: 'FATAL', category: 'render', message: String(error?.stack || error) }).catch(() => {});
    } catch (_) {}
  }
  render() {
    if (this.state.error) {
      const stack = (this.state.error && this.state.error.stack) || String(this.state.error);
      return (
        <div style={{
          padding: 24, height: '100vh', overflow: 'auto',
          background: 'var(--bg-page)', color: 'var(--danger)',
          fontFamily: 'var(--font-mono)', fontSize: 13, whiteSpace: 'pre-wrap',
        }}>
          <h2 style={{ color: 'var(--text-primary)', marginBottom: 12 }}>{crashText('title')}</h2>
          <div>{stack}</div>
          {this.state.info && this.state.info.componentStack && (
            <div style={{ opacity: 0.6, marginTop: 16 }}>{this.state.info.componentStack}</div>
          )}
          <button
            onClick={() => window.location.reload()}
            style={{ marginTop: 16, padding: '6px 12px', background: 'var(--accent)', color: 'var(--accent-text)', border: 'none', borderRadius: 6, cursor: 'pointer' }}
          >{crashText('reload')}</button>
        </div>
      );
    }
    return this.props.children;
  }
}
