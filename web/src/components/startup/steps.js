export const STEP_PROGRESS = {
  connect: 6,
  health: 12,
  envscan: 15,
  'check-service': 18,
  'decide-service': 24,
  'check-local': 30,
  install: 44,
  'install-service': 56,
  'auto-select': 78,
  // Гейт готовности бэка живёт на 90: 100% появляется только когда UI
  // реально открывается — «висящие 100%» вызывают недоверие.
  done: 90,
};

// Фазы умного подбора (autoselect:event phase) — до первого события считаем,
// что идёт определение провайдера.
export const ASM_PHASES = ['operator', 'server', 'local', 'analyze', 'select'];

// Рейка этапов: микрошаги свёрнуты в четыре понятных стадии; пройденные —
// галочка, текущая — пульсирующая точка, будущие — пустые.
export const STAGES = [
  { id: 'prep', steps: ['connect', 'health', 'envscan'] },
  { id: 'check', steps: ['check-service', 'decide-service', 'check-local'] },
  { id: 'install', steps: ['install', 'install-service'] },
  { id: 'select', steps: ['auto-select'] },
];
