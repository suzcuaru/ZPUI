import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import ErrorBoundary from './components/ErrorBoundary';
import { I18nProvider } from './i18n';
import { sound } from './sound';
import { applyRenderTier } from './renderTier';
import './styles/stand.css';

// Звуки интерфейса: глобальные слушатели (первый жест разблокирует
// AudioContext, клик-делегат по кнопкам) ставятся до рендера.
sound.install();

// Тема до первого paint.
(function applyInitialTheme() {
  try {
    const saved = localStorage.getItem('zpui-theme');
    const theme = (saved === 'dark' || saved === 'light') ? saved : 'dark';
    document.documentElement.setAttribute('data-theme', theme);
  } catch {}
}());

// Tier рендера до первого paint: на программном рендере (SwiftShader/WARP)
// body.render-lite убирает живые blur и глушит декоративные анимации.
applyRenderTier();

// Масштаб интерфейса — фиксированный (+5%), применяется CSS-ом через
// zoom оболочки .stand-app (stand.css, --ui-k); JS-применение не нужно.

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <I18nProvider>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </I18nProvider>
  </React.StrictMode>
);
