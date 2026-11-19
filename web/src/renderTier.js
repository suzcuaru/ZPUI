// Детект программного рендера WebView2: если GPU недоступен (драйвер,
// RemoteFX, старый iGPU), WebGL поднимается на SwiftShader/llvmpipe и живые
// blur-эффекты становятся на порядок дороже. Один проб при старте ставит
// body.render-lite — упрощённая отрисовка (CSS: стекло плоской подложкой,
// декоративные анимации глушатся вместе с power-save).
// Спека: Документы/Задачи/2026-11-03-render-responsiveness.md (R7, A7).

const SOFT_RE = /swiftshader|llvmpipe|software|basic render/i;

export function detectRenderTier(glRenderer) {
  if (!glRenderer || SOFT_RE.test(String(glRenderer))) return 'software';
  return 'hardware';
}

export function applyRenderTier() {
  let renderer = '';
  try {
    const canvas = document.createElement('canvas');
    const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
    if (gl) {
      const ext = gl.getExtension('WEBGL_debug_renderer_info');
      renderer = String(gl.getParameter(ext ? ext.UNMASKED_RENDERER_WEBGL : gl.RENDERER));
    }
  } catch {
    renderer = '';
  }
  const lite = detectRenderTier(renderer) === 'software';
  document.body.classList.toggle('render-lite', lite);
  return lite;
}
