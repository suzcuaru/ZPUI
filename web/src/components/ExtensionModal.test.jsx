import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { I18nProvider } from '../i18n';
import { ToastProvider } from '../context/ToastContext';
import { ConfirmProvider } from './ui/ConfirmDialog';
import ExtensionModal from './ExtensionModal';
import { resetUpdateFlowForTests } from '../hooks/useUpdateFlow';
import { apiCacheInvalidate } from '../api';

// Модалка «Браузерный плагин»: список найденных браузеров, предвыбор
// установленных, установка по чекбоксам, снятие через confirm.

const BASE_INFO = {
  status: 'ok',
  id: 'zpui-extension',
  version: '1.2.0',
  available: true,
  browsers: [
    { id: 'chrome', name: 'Google Chrome', found: true, running: false, has_extension: true, installed_version: '1.2.0', selected: true },
    { id: 'edge', name: 'Microsoft Edge', found: true, running: false, has_extension: false, installed_version: '', selected: false },
    { id: 'opera', name: 'Opera', found: false, running: false, has_extension: false, installed_version: '', selected: false },
  ],
  latest: '',
  notes: '',
  update_ready: false,
  server_error: '',
};

function makeApp(overrides = {}) {
  return {
    GetExtensionInfo: vi.fn().mockResolvedValue(BASE_INFO),
    InstallExtension: vi.fn().mockResolvedValue({ status: 'ok' }),
    UninstallExtension: vi.fn().mockResolvedValue({ status: 'ok' }),
    OpenExtensionPage: vi.fn().mockResolvedValue({ status: 'ok' }),
    UpdateExtension: vi.fn().mockResolvedValue({ status: 'ok' }),
    ...overrides,
  };
}

function renderModal() {
  return render(
    <I18nProvider>
      <ToastProvider value={() => {}}>
        <ConfirmProvider>
          <ExtensionModal open onClose={() => {}} />
        </ConfirmProvider>
      </ToastProvider>
    </I18nProvider>
  );
}

async function waitInfo(app) {
  await waitFor(() => expect(app.GetExtensionInfo).toHaveBeenCalled());
  await waitFor(() => expect(document.querySelector('.ext-browser')).toBeTruthy());
}

let app;

beforeEach(() => {
  apiCacheInvalidate();
  app = makeApp();
  window.go = { app: { App: app } };
});

afterEach(() => {
  delete window.go;
  resetUpdateFlowForTests();
});

describe('ExtensionModal', () => {
  it('показывает только найденные браузеры', async () => {
    renderModal();
    await waitInfo(app);
    const rows = [...document.querySelectorAll('.ext-browser-name')].map((n) => n.textContent);
    expect(rows).toEqual(['Google Chrome', 'Microsoft Edge']);
    expect(document.body.textContent).not.toContain('Opera');
  });

  it('предвыбирает браузеры с установленным расширением, «Установить» выключен, пока всё актуально', async () => {
    renderModal();
    await waitInfo(app);
    const boxes = [...document.querySelectorAll('.ext-browser input[type="checkbox"]')];
    expect(boxes).toHaveLength(2);
    expect(boxes[0].checked).toBe(true);
    expect(boxes[1].checked).toBe(false);
    const installBtn = screen.getByRole('button', { name: 'Установить' });
    expect(installBtn.disabled).toBe(true);
  });

  it('установка: вызывает InstallExtension с id тех, где плагина нет, и показывает «Включить»', async () => {
    renderModal();
    await waitInfo(app);
    const boxes = [...document.querySelectorAll('.ext-browser input[type="checkbox"]')];
    fireEvent.click(boxes[1]);
    const installBtn = screen.getByRole('button', { name: 'Установить' });
    expect(installBtn.disabled).toBe(false);
    fireEvent.click(installBtn);
    await waitFor(() => expect(app.InstallExtension).toHaveBeenCalledTimes(1));
    expect(app.InstallExtension).toHaveBeenCalledWith(['edge']);
    const enableBtn = await waitFor(() => screen.getByRole('button', { name: 'Включить' }));
    fireEvent.click(enableBtn);
    await waitFor(() => expect(app.OpenExtensionPage).toHaveBeenCalledWith('chrome'));
  });

  it('пакета нет: список браузеров и выбор остались, «Скачать и установить» ведёт в модалку прогресса', async () => {
    app.GetExtensionInfo.mockResolvedValue({
      ...BASE_INFO,
      version: '',
      available: false,
      latest: '1.2.0',
      update_ready: true,
      browsers: BASE_INFO.browsers.map((b) => ({ ...b, has_extension: false, installed_version: '', selected: false })),
    });
    renderModal();
    await waitFor(() => expect(document.querySelector('.ext-browser')).toBeTruthy());
    expect(document.body.textContent).toContain('Пакет плагина не найден');
    const boxes = [...document.querySelectorAll('.ext-browser input[type="checkbox"]')];
    expect(boxes).toHaveLength(2);
    fireEvent.click(boxes[1]);
    const btn = screen.getByRole('button', { name: 'Скачать и установить' });
    expect(btn.disabled).toBe(false);
    fireEvent.click(btn);
    await waitFor(() => expect(app.UpdateExtension).toHaveBeenCalledTimes(1));
    expect(app.UpdateExtension).toHaveBeenCalledWith('edge');
    expect(app.InstallExtension).not.toHaveBeenCalled();
  });

  it('плагин есть в браузере, но выключен: виден как «выключен» с напоминанием включить', async () => {
    app.GetExtensionInfo.mockResolvedValue({
      ...BASE_INFO,
      browsers: BASE_INFO.browsers.map((b) => (b.id === 'chrome' ? { ...b, state: 'disabled' } : b)),
    });
    renderModal();
    await waitInfo(app);
    const row = document.querySelector('.ext-browser');
    expect(row.textContent).toContain('выключен');
    expect(document.body.textContent).toContain('Плагин установлен в: Google Chrome.');
    expect(screen.getByRole('button', { name: 'Включить' })).toBeTruthy();
  });

  it('удаление: только установленные из отмеченных, через confirm-диалог', async () => {
    renderModal();
    await waitInfo(app);
    fireEvent.click(screen.getByRole('button', { name: 'Удалить' }));
    const cfm = await waitFor(() => document.querySelector('.cfm-modal'));
    fireEvent.click(within(cfm).getByRole('button', { name: 'Удалить' }));
    await waitFor(() => expect(app.UninstallExtension).toHaveBeenCalledTimes(1));
    expect(app.UninstallExtension).toHaveBeenCalledWith(['chrome']);
  });
});
