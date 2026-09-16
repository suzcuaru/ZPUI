import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import ErrorBoundary from './components/ErrorBoundary';
import { I18nProvider } from './i18n';
import { installStandMock } from './stand-mock';
import './styles/stand.css';

// Стенд: мок бэкенда ставится всегда (в браузере нет window.go).
installStandMock();

// Тема до первого paint.
(function applyInitialTheme() {
  try {
    const saved = localStorage.getItem('zpui-theme');
    const theme = (saved === 'dark' || saved === 'light') ? saved : 'dark';
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
