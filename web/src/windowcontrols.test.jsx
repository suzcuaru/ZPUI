import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { I18nProvider } from './i18n';
import { sound } from './sound';
import Modal from './components/ui/Modal';
import TitleBar from './components/layout/TitleBar';

// Кнопки управления окном и крестики модалок: клик БЕЗ звука (data-sound="off"
// исключает их из глобального делегата sound.js), hover/active меняют только
// цвет глифа. Тесты проверяют сам механизм делегата на реальном sound.js.

const onClose = vi.fn();
const onMinimize = vi.fn();
const onToggleMaximize = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(sound, 'play').mockReturnValue(false);
  sound.install();
  render(
    <I18nProvider>
      <Modal open onClose={onClose} title="Тестовая модалка">
        <div>тело</div>
      </Modal>
      <TitleBar
        version="9.9.9"
        arch="x64"
        theme="dark"
        themeSetting="dark"
        onToggleTheme={() => {}}
        maximized={false}
        onToggleMaximize={onToggleMaximize}
        onMinimize={onMinimize}
        onClose={onClose}
      />
    </I18nProvider>
  );
});

describe('window controls & modal close', () => {
  it('крестик модалки: закрывает и БЕЗЗВУКНО, глиф 16px/stroke 1.5', () => {
    const x = document.querySelector('.modal-close');
    expect(x).toBeTruthy();
    expect(x.getAttribute('data-sound')).toBe('off');
    fireEvent.click(x);
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(sound.play).not.toHaveBeenCalled();
    const svg = x.querySelector('svg');
    expect(svg.getAttribute('width')).toBe('16');
    expect(svg.getAttribute('stroke-width')).toBe('1.5');
    expect(svg.getAttribute('stroke-linecap')).toBe('round');
  });

  it('кнопки окна (свернуть/развернуть/закрыть): клики без звука', () => {
    fireEvent.click(screen.getByRole('button', { name: 'Свернуть' }));
    fireEvent.click(screen.getByRole('button', { name: 'Развернуть на весь экран' }));
    fireEvent.click(document.querySelector('.tbar-cap-close'));
    expect(onMinimize).toHaveBeenCalledTimes(1);
    expect(onToggleMaximize).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(sound.play).not.toHaveBeenCalled();
  });

  it('глифы кнопок окна: 16px, stroke 1.5, скруглённые концы', () => {
    document.querySelectorAll('.tbar-cap').forEach((btn) => {
      expect(btn.getAttribute('data-sound')).toBe('off');
      const svg = btn.querySelector('svg');
      expect(svg).toBeTruthy();
      expect(svg.getAttribute('width')).toBe('16');
      expect(svg.getAttribute('stroke-width')).toBe('1.5');
      expect(svg.getAttribute('stroke-linecap')).toBe('round');
    });
    expect(document.querySelectorAll('.tbar-cap').length).toBe(3);
  });

  it('контроль делегата: обычная кнопка по-прежнему щёлкает (click)', () => {
    const el = document.createElement('button');
    el.className = 'btn';
    el.textContent = 'обычная';
    document.body.appendChild(el);
    fireEvent.click(el);
    expect(sound.play).toHaveBeenCalledWith('click');
    el.remove();
  });

  it('контроль исключения Switch: switch-track не щёлкает делегатом', () => {
    const el = document.createElement('button');
    el.className = 'switch-track';
    document.body.appendChild(el);
    fireEvent.click(el);
    expect(sound.play).not.toHaveBeenCalledWith('click');
    el.remove();
  });
});
