import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import ErrorBoundary from './components/ErrorBoundary';
import { I18nProvider } from './i18n';
import { installDevMock } from './dev-mock';

// Dev-only: stub Wails backend so the UI is navigable without the Go backend.
if (import.meta.env.DEV && !(window.go && window.go.app && window.go.app.App)) {
  installDevMock();
}

// Применяем тему синхронно до первого paint, иначе на тёмной Windows
// приложение мелькает/грузится светлым (data-theme ставится только после status).
(function applyInitialTheme() {
  try {
    const saved = localStorage.getItem('zpui-theme');
    const sysDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    const theme = (saved === 'dark' || saved === 'light') ? saved : (sysDark ? 'dark' : 'light');
    document.documentElement.setAttribute('data-theme', theme);
  } catch {}
}());

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <I18nProvider>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </I18nProvider>
  </React.StrictMode>
);
