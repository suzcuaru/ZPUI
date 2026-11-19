// sound.js — звуковой движок интерфейса стенда (мини-прототип для продукта).
//
// Библиотека звуков: Kenney «Interface Sounds» (CC0 1.0, public domain),
// источник: https://kenney.nl/assets/interface-sounds (зеркало:
// github.com/Calinou/kenney-interface-sounds). Файлы — public/sounds/*.wav,
// лицензия — public/sounds/LICENSE.txt.
//
// SoundManager: Web Audio (AudioContext создаётся ЛЕНИВО после первого жеста
// пользователя — автоплей-политика), предзагрузка/кэш буферов по схемам,
// play(category) с громкостью, mute. Категории: click (кнопки/табы), toggle
// (тумблеры), success/error (тосты), notify (уведомления), soft (открытие
// модалок). Громкость/набор приходят из конфига (sounds.{enabled,volume,scheme});
// активная кастомная тема может ПЕРЕКРЫВАТЬ scheme/volume (theme.sounds).
// Экономный режим уровня 3 глушит всё (setSuppressed) — как анимации.
//
// Защита от двоящихся звуков: глобальный троттл по категории (45 мс) + опц.
// dedup-ключ (например 'toast-<id>' против StrictMode-двойных эффектов).
// Наведение (hover) НЕ озвучивается сознательно.

export const SOUND_CATEGORIES = ['click', 'toggle', 'success', 'error', 'notify', 'soft'];

// Схемы: маппинг категории → файл из public/sounds (оригинальные имена Kenney —
// для провенанса). «Стандарт»/«Мягкий»/«Тихий».
export const SOUND_SCHEMES = {
  // Схемы больше не выбираются в UI — везде мягкий набор (legacy-ключи
  // сохранённых конфигов указывают на те же файлы).
  standard: {
    click: 'click_003.wav',
    toggle: 'toggle_002.wav',
    success: 'confirmation_003.wav',
    error: 'error_008.wav',
    notify: 'glass_002.wav',
    soft: 'back_001.wav',
  },
  soft: {
    click: 'click_003.wav',
    toggle: 'toggle_002.wav',
    success: 'confirmation_003.wav',
    error: 'error_008.wav',
    notify: 'glass_002.wav',
    soft: 'back_001.wav',
  },
  quiet: {
    click: 'click_003.wav',
    toggle: 'toggle_002.wav',
    success: 'confirmation_003.wav',
    error: 'error_008.wav',
    notify: 'glass_002.wav',
    soft: 'back_001.wav',
  },
};

export const SCHEME_KEYS = Object.keys(SOUND_SCHEMES);

const clampVol = (v) => Math.min(100, Math.max(0, Math.round(Number(v) || 0)));
export const normalizeScheme = (s) => (SCHEME_KEYS.includes(s) ? s : 'soft');
export const normalizeVolume = (v) => clampVol(v);

const DEFAULT_CFG = { enabled: false, volume: 60, scheme: 'soft' };

class SoundManager {
  constructor() {
    this.ctx = null;
    this.master = null;
    this.buffers = new Map();   // file → AudioBuffer
    this.pending = new Map();   // file → Promise (дедуп параллельных загрузок)
    this.cfg = { ...DEFAULT_CFG };
    this.theme = null;          // {scheme, volume} активной кастомной темы | null
    this.suppressed = false;    // экономный режим уровня 3
    this.lastAt = new Map();    // category → ts (троттл)
    this.lastKeyAt = new Map(); // dedup key → ts
    this.installed = false;
    this.unlocked = false;
  }

  // Эффективные параметры: конфиг + перекрытие темой; null = звуки выключены.
  // Громкость — ТОЛЬКО пользовательская (config.sounds.volume): тема перекрывает
  // лишь набор (scheme), иначе ползунок в настройках «не работает» при активной
  // кастомной теме со своим volume.
  effective() {
    if (this.suppressed || !this.cfg.enabled) return null;
    return {
      scheme: normalizeScheme((this.theme && this.theme.scheme) || this.cfg.scheme),
      volume: clampVol(this.cfg.volume),
    };
  }

  setConfig(patch) {
    const before = this.cfg.scheme;
    const p = patch && typeof patch === 'object' ? patch : {};
    this.cfg = {
      enabled: p.enabled === true ? true : (p.enabled === false ? false : this.cfg.enabled),
      volume: clampVol(p.volume != null ? p.volume : this.cfg.volume),
      scheme: normalizeScheme(p.scheme || this.cfg.scheme),
    };
    if (this.unlocked && this.cfg.scheme !== before) this.prefetch(this.cfg.scheme);
  }

  setThemeSounds(sounds) {
    const s = sounds && typeof sounds === 'object' ? sounds : null;
    this.theme = s ? { scheme: normalizeScheme(s.scheme), volume: clampVol(s.volume != null ? s.volume : 60) } : null;
    if (this.theme && this.unlocked) this.prefetch(this.theme.scheme);
  }

  clearThemeSounds() {
    this.theme = null;
  }

  setSuppressed(v) {
    this.suppressed = !!v;
  }

  // Глобальные слушатели: разблокировка аудио первым жестом + делегат кликов
    // по кнопкам (кроме disabled, тумблеров Switch и помеченных data-sound="off").
  install() {
    if (this.installed || typeof document === 'undefined') return;
    this.installed = true;
    const gesture = () => this.unlock();
    document.addEventListener('pointerdown', gesture, true);
    document.addEventListener('keydown', gesture, true);
    document.addEventListener('click', (e) => this.onDocClick(e), true);
  }

  // Ленивая инициализация AudioContext — только после жеста пользователя.
  unlock() {
    if (this.unlocked) return true;
    const AC = (typeof window !== 'undefined' && (window.AudioContext || window.webkitAudioContext))
      || (typeof globalThis !== 'undefined' && (globalThis.AudioContext || globalThis.webkitAudioContext));
    if (!AC) return false;
    try {
      this.ctx = new AC();
      this.master = this.ctx.createGain();
      this.master.connect(this.ctx.destination);
      this.unlocked = true;
      const eff = this.effective();
      this.prefetch(eff ? eff.scheme : this.cfg.scheme);
    } catch {
      this.ctx = null;
    }
    return this.unlocked;
  }

  async prefetch(scheme) {
    const map = SOUND_SCHEMES[normalizeScheme(scheme)];
    if (!map) return;
    await Promise.all(Object.values(map).map((f) => this.loadBuffer(f)));
  }

  async loadBuffer(file) {
    if (this.buffers.has(file)) return this.buffers.get(file);
    if (this.pending.has(file)) return this.pending.get(file);
    const p = (async () => {
      try {
        const r = await fetch('/sounds/' + file);
        if (!r.ok) throw new Error('http ' + r.status);
        const ab = await r.arrayBuffer();
        if (!this.ctx) throw new Error('no ctx');
        const buf = await new Promise((res, rej) => {
          const pr = this.ctx.decodeAudioData(ab, res, rej);
          if (pr && typeof pr.then === 'function') pr.then(res, rej);
        });
        this.buffers.set(file, buf);
        return buf;
      } catch {
        return null;
      } finally {
        this.pending.delete(file);
      }
    })();
    this.pending.set(file, p);
    return p;
  }

  // Мгновенный фолбэк на HTMLAudio: пока буфер декодируется (или контекст
  // ещё suspended) — первый клик не должен молчать. Файл уже в HTTP-кэше
  // prefetch'а, поэтому старт почти мгновенный.
  playHtmlFallback(file, volume) {
    try {
      const a = new Audio('/sounds/' + file);
      a.volume = clampVol(volume) / 100;
      const p = a.play();
      if (p && typeof p.catch === 'function') p.catch(() => {});
    } catch {}
  }

  startSource(file, volume) {
    try {
      const buf = this.buffers.get(file);
      if (!buf || !this.ctx || !this.master) return false;
      const src = this.ctx.createBufferSource();
      src.buffer = buf;
      const g = this.ctx.createGain();
      g.gain.value = clampVol(volume) / 100;
      src.connect(g);
      g.connect(this.master);
      src.start();
      return true;
    } catch {
      return false;
    }
  }

  // Воспроизведение категории. opts: {force, scheme, volume, key, keyGap}.
  // force — «прослушать» из редактора: играет даже при выключенных звуках.
  play(category, opts = {}) {
    if (!SOUND_CATEGORIES.includes(category)) return false;
    const eff = this.effective();
    const scheme = normalizeScheme(opts.scheme || (eff ? eff.scheme : this.cfg.scheme));
    const volume = opts.volume != null ? clampVol(opts.volume) : eff ? eff.volume : this.cfg.volume;
    if (!opts.force && !eff) return false;
    // Аудио-контекст лениво создаём при первом воспроизведении: install()-жесты
    // могут отсутствовать, а тосты всегда следуют за действием пользователя.
    if (!this.ctx) this.unlock();
    const now = Date.now();
    if (!opts.force) {
      if (now - (this.lastAt.get(category) || 0) < 45) return false;
      this.lastAt.set(category, now);
      if (opts.key) {
        const gap = opts.keyGap != null ? opts.keyGap : 400;
        if (now - (this.lastKeyAt.get(opts.key) || 0) < gap) return false;
        this.lastKeyAt.set(opts.key, now);
      }
    }
    const file = (SOUND_SCHEMES[scheme] || {})[category];
    if (!file) return false;

    const suspended = !!(this.ctx && this.ctx.state === 'suspended');
    const buf = this.buffers.get(file);
    if (suspended || !buf) {
      // Первый клик: контекст просыпается асинхронно / буфер ещё декодируется —
      // звучим сразу через HTMLAudio, Web Audio подхватит следующие звуки.
      this.playHtmlFallback(file, volume);
      if (this.ctx) {
        try {
          const r = this.ctx.resume();
          if (r && typeof r.catch === 'function') r.catch(() => {});
        } catch {}
      }
      this.loadBuffer(file);
      return true;
    }
    return this.startSource(file, volume);
  }

  // «Прослушать» из редактора тем: конкретная схема/громкость черновика,
  // независимо от enabled/suppressed (явное действие пользователя).
  preview(category, scheme, volume) {
    return this.play(category, { force: true, scheme, volume });
  }

  // Делегат: любой клик по button/.btn → click (или своя категория из
  // data-sound). Тумблеры Switch (switch-track) озвучивают себя сами, чтобы
  // играть toggle вместо click; data-sound="off" — беззвучная кнопка.
  onDocClick(e) {
    const target = e && e.target;
    if (!target || !target.closest) return;
    const btn = target.closest('button, .btn');
    if (!btn) return;
    if (btn.disabled || btn.getAttribute('aria-disabled') === 'true') return;
    if (btn.classList.contains('switch-track') || btn.closest('.switch-track')) return;
    const explicit = btn.getAttribute('data-sound');
    if (explicit === 'off') return;
    this.play(SOUND_CATEGORIES.includes(explicit) ? explicit : 'click');
  }
}

export const sound = new SoundManager();
