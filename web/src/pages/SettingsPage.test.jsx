import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, fireEvent, waitFor, within } from '@testing-library/react';
import SettingsPage from './SettingsPage';
import { I18nProvider } from '../i18n';
import { ToastProvider } from '../context/ToastContext';
import { ConfirmProvider } from '../components/ui/ConfirmDialog';
import { apiCacheInvalidate } from '../api';
import { resetUpdateFlowForTests } from '../hooks/useUpdateFlow';

// Ряд «Плагин браузера» в панели обновлений настроек: версия пакета,
// «актуально»/«Обновить»/«Установить» (POST /api/extension/update),
// приглушённая подсказка при недоступном сервере.

const INFO = (over = {}) => ({
  status: 'ok',
  id: 'zpui-extension',
  version: '1.2.0',
  available: true,
  browsers: [],
  latest: '',
  notes: '',
  update_ready: false,
  update_via: 'none',
  server_error: '',
  ...over,
});

function makeApp(info) {
  return {
    GetConfig: vi.fn().mockResolvedValue({ theme: 'dark', language: 'ru' }),
    CheckVersionsPanel: vi.fn().mockResolvedValue(null),
    CheckZapretVersionsPanel: vi.fn().mockResolvedValue(null),
    GetSyncInfo: vi.fn().mockResolvedValue(null),
    GetUpdatesSource: vi.fn().mockResolvedValue(null),
    GetVersions: vi.fn().mockResolvedValue(null),
    GetZsvcPanel: vi.fn().mockResolvedValue(null),
    GetZapretServiceAutostart: vi.fn().mockResolvedValue(null),
    GetCustomThemes: vi.fn().mockResolvedValue(null),
    GetExtensionInfo: vi.fn().mockResolvedValue(info),
    UpdateExtension: vi.fn().mockResolvedValue({ status: 'ok', via: 'setup', version: '1.3.0', browsers: [] }),
  };
}

function renderPage() {
  return render(
    <I18nProvider>
      <ToastProvider value={() => {}}>
        <ConfirmProvider>
          <SettingsPage status={{}} onOpenLogs={() => {}} />
        </ConfirmProvider>
      </ToastProvider>
    </I18nProvider>
  );
}

function extCard() {
  return [...document.querySelectorAll('.upd-card')]
    .find((c) => c.textContent.includes('Браузерный плагин'));
}

beforeEach(() => {
  apiCacheInvalidate();
  resetUpdateFlowForTests();
});

afterEach(() => {
  delete window.go;
  resetUpdateFlowForTests();
});

describe('SettingsPage: ряд «Плагин браузера»', () => {
  it('update_ready: версии и кнопка «Обновить», клик открывает модалку прогресса (UpdateExtension), итог перечитывает инфо', async () => {
    const app = makeApp(INFO({ latest: '1.3.0', update_ready: true, update_via: 'setup' }));
    window.go = { app: { App: app } };
    renderPage();
    const btn = await waitFor(() => {
      const card = extCard();
      expect(card).toBeTruthy();
      return within(card).getByRole('button', { name: 'Обновить' });
    });
    const card = extCard();
    expect(card.textContent).toContain('v1.2.0');
    expect(card.textContent).toContain('v1.3.0');
    const callsBefore = app.GetExtensionInfo.mock.calls.length;
    fireEvent.click(btn);
    await waitFor(() => expect(app.UpdateExtension).toHaveBeenCalledTimes(1));
    expect(app.UpdateExtension).toHaveBeenCalledWith();
    // Пока идёт установка — ряда не трогаем повторно: кнопка «Обновить» уступает спиннеру.
    await waitFor(() => expect(card.querySelector('button')).toBeNull());
    window.dispatchEvent(new CustomEvent('zpui:extension-installed'));
    await waitFor(() => expect(app.GetExtensionInfo.mock.calls.length).toBeGreaterThan(callsBefore));
  });

  it('пакет актуален: пилюля «Актуально», кнопки в ряду нет', async () => {
    const app = makeApp(INFO());
    window.go = { app: { App: app } };
    renderPage();
    const card = await waitFor(() => {
      const c = extCard();
      expect(c).toBeTruthy();
      expect(c.querySelector('.upd-status.latest')).toBeTruthy();
      return c;
    });
    expect(card.textContent).toContain('Актуально');
    expect(card.querySelector('button')).toBeNull();
  });

  it('пакета нет, на сервере есть выпуск: кнопка «Установить» вызывает UpdateExtension', async () => {
    const app = makeApp(INFO({ version: '', available: false, latest: '1.3.0', update_ready: true, update_via: 'crx' }));
    window.go = { app: { App: app } };
    renderPage();
    const btn = await waitFor(() => {
      const card = extCard();
      expect(card).toBeTruthy();
      return within(card).getByRole('button', { name: 'Установить' });
    });
    fireEvent.click(btn);
    await waitFor(() => expect(app.UpdateExtension).toHaveBeenCalledTimes(1));
  });

  it('сервер недоступен и пакета нет: приглушённая подсказка, без кнопки', async () => {
    const app = makeApp(INFO({ version: '', available: false, server_error: 'сервер: http 503' }));
    window.go = { app: { App: app } };
    renderPage();
    const card = await waitFor(() => {
      const c = extCard();
      expect(c).toBeTruthy();
      expect(c.querySelector('.upd-status.none')).toBeTruthy();
      return c;
    });
    expect(card.textContent).toContain('Сервер недоступен');
    expect(card.querySelector('button')).toBeNull();
  });
});
